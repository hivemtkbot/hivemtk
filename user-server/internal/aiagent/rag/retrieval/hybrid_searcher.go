package ragretrieval

// hybrid_searcher.go 混合检索器主入口
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.1
// 调用方: L3 业务层 RagSearcher.Search / SearchIndex（rag_searcher.go）
//
// 设计原则:
//   - 向量召回与 BM25 召回并行执行（不是兜底关系）
//   - RRF 融合后送 Reranker 重排
//   - 查询改写（HyDE + Multi-Query）前置，命中缓存优先
//   - 所有 TEI/Redis 调用失败均报错，禁止静默降级到 BM25-lite
//   - 两路召回（向量+BM25）均失败才报错；单路失败仅记录日志
//   - 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

// HybridSearcher 混合检索器主入口
type HybridSearcher struct {
	db              *gorm.DB
	embeddingClient llm.EmbeddingServiceInterface // 走 CachedEmbeddingClient 装饰
	vectorRetriever *VectorRetriever
	bm25Retriever   *BM25Retriever
	rrfFusion       *RRFFusion
	reranker        RerankerInterface // 复用现有 LocalReranker
	queryRewriter   *QueryRewriter
	contextualEnh   *ContextualRetrievalEnhancer

	// 检索参数
	defaultTopK      int // 最终返回 topK（默认 5）
	candidatePool    int // 每路召回候选池大小（默认 50）
	fusedTopN        int // RRF 融合后送重排的 topN（默认 20）
	rrfK             int // RRF 平滑常数（默认 60）
	efSearch         int // HNSW ef_search（默认 80）
	enableHyDE       bool
	enableMultiQuery bool
	enableRerank     bool

	// 检索日志表存在性缓存：避免每条消息都查 information_schema（性能审计 P1-4）。
	logMu        sync.Mutex
	logReady     bool
	logCheckedAt time.Time
}

// HybridSearcherConfig 配置
type HybridSearcherConfig struct {
	DefaultTopK      int
	CandidatePool    int
	FusedTopN        int
	RRFK             int
	EfSearch         int
	EnableHyDE       bool
	EnableMultiQuery bool
	EnableRerank     bool
}

// DefaultHybridSearcherConfig 默认配置
//
// 性能审计 P1-5：HyDE / MultiQuery 会为每条消息额外发起 1~N 次 LLM 调用，
// 在 1000 万/日被动回复下把 LLM 负载放大 2-3 倍。默认关闭，仅在建库调试或低吞吐场景
// 通过环境变量 RAG_ENABLE_HYDE / RAG_ENABLE_MULTIQUERY 开启。
func DefaultHybridSearcherConfig() *HybridSearcherConfig {
	return &HybridSearcherConfig{
		DefaultTopK:      5,
		CandidatePool:    50,
		FusedTopN:        20,
		RRFK:             60,
		EfSearch:         80,
		EnableHyDE:       envBool("RAG_ENABLE_HYDE", false),
		EnableMultiQuery: envBool("RAG_ENABLE_MULTIQUERY", false),
		EnableRerank:     true,
	}
}

// envBool 读取布尔型环境变量，支持 "1"/"true"/"yes"/"on"（大小写不敏感），否则用默认值。
func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// NewHybridSearcher 构造混合检索器
//
// 依赖注入:
//   - db: GORM DB（指向 knowledge_chunks 表）
//   - embeddingClient: 已被 CachedEmbeddingClient 装饰的 EmbeddingService
//   - reranker: 复用现有 LocalReranker（rerank.go）；可为 nil 禁用重排
//   - llmChatClient: 用于 HyDE / Multi-Query / Contextual Retrieval；可为 nil 禁用查询改写
//   - redisClient: 用于查询改写缓存；可为 nil 跳过 L1 缓存
//   - cfg: 检索参数配置；nil 用默认值
//
// 注意:
//   - 本构造函数不创建 ContextualRetrievalEnhancer（索引期工具，运行时检索不需要）
//     如需索引期增强，用 NewContextualRetrievalEnhancer 单独构造
//   - llmChatClient 为 nil 时 EnableHyDE / EnableMultiQuery 自动置 false
func NewHybridSearcher(
	db *gorm.DB,
	embeddingClient llm.EmbeddingServiceInterface,
	reranker RerankerInterface,
	llmChatClient LLMChatClient,
	redisClient RedisClient,
	cfg *HybridSearcherConfig,
) *HybridSearcher {
	if cfg == nil {
		cfg = DefaultHybridSearcherConfig()
	}
	enableHyDE := cfg.EnableHyDE && llmChatClient != nil
	enableMultiQuery := cfg.EnableMultiQuery && llmChatClient != nil

	s := &HybridSearcher{
		db:               db,
		embeddingClient:  embeddingClient,
		rrfFusion:        NewRRFFusion(cfg.RRFK),
		reranker:         reranker,
		defaultTopK:      cfg.DefaultTopK,
		candidatePool:    cfg.CandidatePool,
		fusedTopN:        cfg.FusedTopN,
		rrfK:             cfg.RRFK,
		efSearch:         cfg.EfSearch,
		enableHyDE:       enableHyDE,
		enableMultiQuery: enableMultiQuery,
		enableRerank:     cfg.EnableRerank,
	}
	s.vectorRetriever = NewVectorRetriever(db, embeddingClient, cfg.EfSearch)
	s.bm25Retriever = NewBM25Retriever(db)

	// HyDE / Multi-Query 生成器（仅在 llmChatClient 非 nil 时启用）
	hydeGen := NewHyDEGenerator(llmChatClient, &HyDEGeneratorConfig{
		Enabled: enableHyDE,
	})
	multiGen := NewMultiQueryGenerator(llmChatClient, &MultiQueryGeneratorConfig{
		Enabled: enableMultiQuery,
	})
	s.queryRewriter = NewQueryRewriter(hydeGen, multiGen, redisClient, db, nil)

	return s
}

// SetContextualEnhancer 注入上下文增强器（可选，仅索引期使用）
func (s *HybridSearcher) SetContextualEnhancer(enh *ContextualRetrievalEnhancer) {
	s.contextualEnh = enh
}

// ContextualEnhancer 返回上下文增强器（可能为 nil）
func (s *HybridSearcher) ContextualEnhancer() *ContextualRetrievalEnhancer {
	return s.contextualEnh
}

// Search 全产品混合检索
//
// 流程:
//  1. QueryRewriter 改写（HyDE + Multi-Query）
//  2. 并行：VectorRetriever + BM25Retriever
//  3. RRFFusion 融合 top fusedTopN
//  4. Reranker 重排 top defaultTopK（失败回退融合顺序）
//  5. 异步记录检索日志（不阻塞返回）
//
// 容错:
//   - QueryRewriter 失败：降级为原 query
//   - 向量召回 / BM25 召回单路失败：仅记录日志，用另一路结果
//   - 两路均失败：返回 error
//   - Reranker 失败：回退到 RRF 融合顺序
func (s *HybridSearcher) Search(ctx context.Context, query string, topK int) ([]Chunk, error) {
	return s.searchWithProductID(ctx, 0, query, topK)
}

// SearchIndex 指定 product_id 检索（与现有 RagSearcher.SearchIndex 对齐）
func (s *HybridSearcher) SearchIndex(ctx context.Context, productID int64, query string, topK int) ([]Chunk, error) {
	return s.searchWithProductID(ctx, productID, query, topK)
}

// searchWithProductID 内部统一检索实现
func (s *HybridSearcher) searchWithProductID(ctx context.Context, productID int64, query string, topK int) ([]Chunk, error) {
	if s == nil {
		return nil, fmt.Errorf("hybrid searcher 未初始化")
	}
	if topK <= 0 {
		topK = s.defaultTopK
	}
	startTime := time.Now()

	// 1) 查询改写
	rewriteStart := time.Now()
	rewritten, err := s.queryRewriter.Rewrite(ctx, query)
	if err != nil {
		// 改写失败不阻塞检索，降级为原始 query
		logger.Ctx(ctx).Error().Err(err).Msg("[HybridSearcher] query rewrite failed, use raw query")
		rewritten = &RewrittenQuery{Original: query, Rewritten: query, UsedStrategy: StrategyNone}
	}
	rewriteLatency := time.Since(rewriteStart).Milliseconds()

	// 2) 并行召回
	var (
		vecResults, bm25Results []Chunk
		vecErr, bm25Err         error
	)
	vecStart := time.Now()
	bm25Start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		vecResults, vecErr = s.vectorRetriever.Retrieve(ctx, productID, rewritten.Rewritten, s.candidatePool)
	}()
	go func() {
		defer wg.Done()
		bm25Results, bm25Err = s.bm25Retriever.Retrieve(ctx, productID, query, s.candidatePool)
	}()
	wg.Wait()
	vecLatency := time.Since(vecStart).Milliseconds()
	bm25Latency := time.Since(bm25Start).Milliseconds()

	// 两路都失败才报错（私域基线：禁止静默降级到 BM25-lite）
	if vecErr != nil && bm25Err != nil {
		return nil, fmt.Errorf("vector and bm25 both failed: vec=%v bm25=%v", vecErr, bm25Err)
	}
	if vecErr != nil {
		logger.Ctx(ctx).Error().Err(vecErr).Int64("product_id", productID).
			Msg("[HybridSearcher] vector retriever failed, use bm25 only")
	}
	if bm25Err != nil {
		logger.Ctx(ctx).Error().Err(bm25Err).Int64("product_id", productID).
			Msg("[HybridSearcher] bm25 retriever failed, use vector only")
	}

	// 3) RRF 融合
	fused := s.rrfFusion.Fuse(vecResults, bm25Results, s.fusedTopN)

	// 4) 重排
	var final []Chunk
	rerankStart := time.Now()
	if s.enableRerank && s.reranker != nil && len(fused) > 0 {
		reranked, err := RerankChunks(ctx, s.reranker, query, fused)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Msg("[HybridSearcher] rerank failed, use fused order")
			final = trimChunks(fused, topK)
		} else {
			final = trimChunks(reranked, topK)
		}
	} else {
		final = trimChunks(fused, topK)
	}
	rerankLatency := time.Since(rerankStart).Milliseconds()

	// 5) 异步记录检索日志（不阻塞返回）
	go s.logSearch(productID, query, topK, len(vecResults), len(bm25Results), len(fused), len(final),
		vecLatency, bm25Latency, rewriteLatency, rerankLatency, string(rewritten.UsedStrategy), rewritten.CacheHit)

	logger.Ctx(ctx).Info().
		Str("query", query).
		Int("vec_n", len(vecResults)).
		Int("bm25_n", len(bm25Results)).
		Int("fused_n", len(fused)).
		Int("final_n", len(final)).
		Int64("total_ms", time.Since(startTime).Milliseconds()).
		Msg("[HybridSearcher] search done")
	return final, nil
}

// logSearch 异步记录检索日志
//
// 写入 knowledge_search_logs 表（若表存在）；
// best-effort: 失败仅记录日志，不影响主流程
func (s *HybridSearcher) logSearch(productID int64, query string, topK, vecN, bm25N, fusedN, finalN int,
	vecLatency, bm25Latency, rewriteLatency, rerankLatency int64,
	rewriteStrategy string, cacheHit bool) {
	if s.db == nil {
		return
	}
	ctx := context.Background()
	// 性能审计 P1-4：表存在性仅探测一次（5 分钟 TTL 复探），避免 1000 万/日每条消息都查 information_schema。
	if !s.searchLogReady(ctx) {
		return
	}
	err := s.db.WithContext(ctx).Exec(`
		INSERT INTO knowledge_search_logs
			(query, product_id, top_k, vector_count, bm25_count, fused_count, rerank_count,
			 vector_latency_ms, bm25_latency_ms, rewrite_latency_ms, rerank_latency_ms,
			 rewrite_used, cache_hit, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
	`,
		query, productID, topK, vecN, bm25N, fusedN, finalN,
		vecLatency, bm25Latency, rewriteLatency, rerankLatency,
		rewriteStrategy, cacheHit,
	).Error
	if err != nil {
		// best-effort：不阻断主流程，但记录错误（R5 修复：原 _ = err 静默吞噬）
		logger.Errorf("hybrid_searcher: persist search log failed: %v", err)
	}
}

// searchLogReady 探测 knowledge_search_logs 表是否存在，结果缓存 5 分钟（性能审计 P1-4）。
func (s *HybridSearcher) searchLogReady(ctx context.Context) bool {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if time.Since(s.logCheckedAt) < 5*time.Minute {
		return s.logReady
	}
	var exists bool
	if err := s.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_search_logs')`,
	).Scan(&exists).Error; err == nil {
		s.logReady = exists
	}
	s.logCheckedAt = time.Now()
	return s.logReady
}
