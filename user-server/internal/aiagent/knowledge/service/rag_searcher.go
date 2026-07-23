package service

import (
	"context"
	"fmt"
	"strings"

	"marketing/internal/aiagent/llm"
	"marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// RagSearcher 知识库检索器
// 2026-07-18 重构：真正走 pgvector 余弦相似度 + TEI bge-m3（1024 维）
//
//   - 优先路径：TEI 把 query 编码成 1024 维向量，SQL 用 embedding <=> $1 走 HNSW 索引
//   - 兜底路径：当 TEI 不可用或 query 为空时，回退到 BM25-lite 文本匹配
//   - 降级日志：ERROR 级别（与私域基线一致：禁止静默降级到伪向量）
//
// 2026-07-19 升级（P0-2 RAG 精确召回）：
//   - 新增 hybridSearcher 字段（委托模式），非 nil 时优先走 HybridSearcher
//   - HybridSearcher 提供：pgvector HNSW + tsvector BM25 + RRF 融合 + bge-reranker-v2-m3 重排
//   - HyDE/Multi-Query 改写 + Contextual Retrieval + CachedEmbeddingClient
//   - 旧 vectorSearch / bm25SearchAll 路径保留为 legacy fallback（hybridSearcher 为 nil 时启用）
//   - 私域独立部署：无 merchant_id 字段
type RagSearcher struct {
	db               *gorm.DB
	embeddingService llm.EmbeddingServiceInterface
	hybridSearcher   *ragretrieval.HybridSearcher // 委托的混合检索器（可选，nil 时走 legacy）
}

// NewRagSearcher 创建全局 RAG 检索器（自动初始化 TEI 客户端 + HybridSearcher）
//
// 修复 P0-2 章节检查报告 #1：自动启用 HybridSearcher，无需外部手动调用 EnableHybridSearcher。
// 注意：reranker / llmChatClient / redisClient 在此便捷路径中均为 nil，
// 检索将以 RRF 融合 + 上下文相关 BM25 + 向量召回三路并行（无重排、无 HyDE、无 L1 缓存）。
// 需要完整能力的生产环境请用 EnableHybridSearcher 显式注入依赖。
//
// 2026-07-23 五层架构治理（二轮）：构造函数内允许调 `db.GetDB()`（service 工厂
// 合规），但不再走 `dbGetDB()` 本地别名（已删除）。
func NewRagSearcher() *RagSearcher {
	return newRagSearcherWithDB(db.GetDB())
}

// NewRagSearcherWithDB 创建带 DB 的 RAG 检索器（供测试注入 DB）
//
// 修复 P0-2 章节检查报告 #1：测试场景同样启用 HybridSearcher。
func NewRagSearcherWithDB(gdb *gorm.DB) *RagSearcher {
	return newRagSearcherWithDB(gdb)
}

func newRagSearcherWithDB(gdb *gorm.DB) *RagSearcher {
	s := &RagSearcher{
		db:               gdb,
		embeddingService: llm.NewEmbeddingService(),
	}
	// 构造独立 Dispatcher 实例（无外部注入时使用本地 LLM 服务）。
	// 此处不强制启用 HyDE/MultiQuery/Rerank（依赖外部 client），仅保证 RRF+Vector+BM25 路径可用。
	dispatcher := llm.NewDispatcher(llm.NewLLMService())
	s.EnableHybridSearcher(nil, nil, nil, s.embeddingService, ragretrieval.DefaultHybridSearcherConfig())
	_ = dispatcher // 留作未来扩展（HyDE/MultiQuery 启用时通过 SetLLMChat 注入）
	return s
}

// SetHybridSearcher 注入混合检索器（DI，用于运行时切换检索策略）
//
// 传入 nil 等价于禁用混合检索（回退到 legacy vector + BM25-lite）
func (s *RagSearcher) SetHybridSearcher(hs *ragretrieval.HybridSearcher) {
	s.hybridSearcher = hs
}

// HybridSearcher 返回当前委托的混合检索器（可能为 nil）
func (s *RagSearcher) HybridSearcher() *ragretrieval.HybridSearcher {
	return s.hybridSearcher
}

// EnableHybridSearcher 构造依赖并启用混合检索（一站式便捷方法）
//
// 参数：
//   - reranker: 重排器（bge-reranker-v2-m3）；nil 时禁用重排
//   - llmChatClient: LLM 对话客户端（用于 HyDE / Multi-Query）；nil 时禁用查询改写
//   - redisClient: Redis 客户端（用于查询改写缓存）；nil 时跳过 L1 缓存
//   - embeddingClient: 已被 CachedEmbeddingClient 装饰的 Embedding 服务；nil 时使用内部默认
//   - cfg: 检索参数；nil 用默认配置
//
// 调用此方法后，Search / SearchIndex 将优先委托给 HybridSearcher
func (s *RagSearcher) EnableHybridSearcher(
	reranker ragretrieval.RerankerInterface,
	llmChatClient ragretrieval.LLMChatClient,
	redisClient ragretrieval.RedisClient,
	embeddingClient llm.EmbeddingServiceInterface,
	cfg *ragretrieval.HybridSearcherConfig,
) {
	if embeddingClient == nil {
		embeddingClient = s.embeddingService
	}
	s.hybridSearcher = ragretrieval.NewHybridSearcher(
		s.db, embeddingClient, reranker, llmChatClient, redisClient, cfg,
	)
}

// Search 全产品检索（实现 SalesEngine 的 RAGSearcher 接口）
//
// 流程：
//  1. 若 hybridSearcher 已注入 → 委托给 HybridSearcher（混合检索 + RRF + 重排）
//  2. 否则走 legacy：TEI 编码 query → 1024 维向量 → pgvector <=> HNSW → 失败回退 BM25-lite
func (s *RagSearcher) Search(ctx context.Context, query string, topK int) ([]RAGChunk, error) {
	if s.db == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = DefaultTopK
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	// 1) 优先委托给 HybridSearcher
	if s.hybridSearcher != nil {
		chunks, err := s.hybridSearcher.Search(ctx, query, topK)
		if err == nil {
			return chunksToRAGChunks(chunks), nil
		}
		// 混合检索失败：降级到 legacy 路径（保证可用性）
		logger.Errorf("[RagSearcher] hybrid search failed, fallback to legacy: %v", err)
	} else {
		logger.Debugf("[RagSearcher] hybrid searcher not enabled, use legacy vector+BM25-lite")
	}

	// 2) legacy 向量检索
	rows, vecErr := s.vectorSearch(ctx, 0, query, topK)
	if vecErr == nil && len(rows) > 0 {
		return s.toRAGChunks(rows), nil
	}
	if vecErr != nil {
		// 私域基线：禁止静默降级；但当 TEI 不可达/无 embedding 列时，必须有兜底
		logger.Errorf("[RagSearcher] vector search failed, fallback to BM25-lite: %v", vecErr)
	}

	// 3) 兜底 BM25-lite
	return s.bm25SearchAll(ctx, query, topK)
}

// SearchIndex 在指定产品下检索 query
//
// 流程与 Search 相同，但带 product_id 过滤。
// metadata 为可选附加字段过滤条件（如 {"customer_id":"123","order_id":"A01"}），
// 用于把检索范围收敛到特定业务上下文（如某客户的订单知识）。
func (s *RagSearcher) SearchIndex(ctx context.Context, productID int64, query string, topK int, metadata map[string]string) ([]MerchantRAGChunk, error) {
	if s.db == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = DefaultTopK
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	// 1) 优先委托给 HybridSearcher
	if s.hybridSearcher != nil {
		chunks, err := s.hybridSearcher.SearchIndex(ctx, productID, query, topK)
		if err == nil {
			return filterMerchantChunksByMetadata(chunksToMerchantChunks(chunks), metadata), nil
		}
		logger.Errorf("[RagSearcher] hybrid search (product=%d) failed, fallback to legacy: %v", productID, err)
	}

	// 2) legacy 向量检索
	rows, vecErr := s.vectorSearch(ctx, productID, query, topK)
	if vecErr == nil && len(rows) > 0 {
		return filterMerchantChunksByMetadata(s.toMerchantChunks(rows), metadata), nil
	}
	if vecErr != nil {
		logger.Errorf("[RagSearcher] vector search failed (product=%d), fallback to BM25-lite: %v", productID, vecErr)
	}

	// 3) 兜底 BM25-lite
	bm25, _ := s.bm25SearchIndex(ctx, productID, query, topK)
	filtered := filterMerchantChunksByMetadata(bm25, metadata)
	return filtered, nil
}

// SearchIndexWithConfig 使用指定的 embedding/rerank 配置做 per 知识库检索（覆盖全局默认）
//
// 实现：每次请求用传入的 embedding client + reranker 构造临时 HybridSearcher，
// 不修改共享的 s.hybridSearcher 单例（避免并发竞态）。结果经 chunksToMerchantChunks 转换。
func (s *RagSearcher) SearchIndexWithConfig(ctx context.Context, productID int64, query string, topK int, embClient llm.EmbeddingServiceInterface, reranker ragretrieval.RerankerInterface) ([]MerchantRAGChunk, error) {
	if s.db == nil {
		return nil, fmt.Errorf("db 未初始化")
	}
	cfg := ragretrieval.DefaultHybridSearcherConfig()
	hs := ragretrieval.NewHybridSearcher(s.db, embClient, reranker, nil, nil, cfg)
	chunks, err := hs.SearchIndex(ctx, productID, query, topK)
	if err != nil {
		return nil, err
	}
	return chunksToMerchantChunks(chunks), nil
}
