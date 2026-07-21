package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"marketing/internal/aiagent/llm"
	"marketing/internal/aiagent/rag/retrieval"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// RAGChunk 别名(从 dto 包引用)
type RAGChunk = dto.RAGChunk

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
func NewRagSearcher() *RagSearcher {
	s := &RagSearcher{
		db:               dbGetDB(),
		embeddingService: llm.NewEmbeddingService(),
	}
	// 构造独立 Dispatcher 实例（无外部注入时使用本地 LLM 服务）。
	// 此处不强制启用 HyDE/MultiQuery/Rerank（依赖外部 client），仅保证 RRF+Vector+BM25 路径可用。
	dispatcher := llm.NewDispatcher(llm.NewLLMService())
	s.EnableHybridSearcher(nil, nil, nil, s.embeddingService, ragretrieval.DefaultHybridSearcherConfig())
	_ = dispatcher // 留作未来扩展（HyDE/MultiQuery 启用时通过 SetLLMChat 注入）
	return s
}

// NewRagSearcherWithDB 创建带 DB 的 RAG 检索器（供测试注入 DB）
//
// 修复 P0-2 章节检查报告 #1：测试场景同样启用 HybridSearcher。
func NewRagSearcherWithDB(gdb *gorm.DB) *RagSearcher {
	s := &RagSearcher{
		db:               gdb,
		embeddingService: llm.NewEmbeddingService(),
	}
	s.EnableHybridSearcher(nil, nil, nil, s.embeddingService, ragretrieval.DefaultHybridSearcherConfig())
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

// MerchantRAGChunk 召回分段
type MerchantRAGChunk struct {
	ID         uint64         `json:"id"`
	DocumentID uint64         `json:"document_id"`
	Content    string         `json:"content"`
	Score      float64        `json:"score"`
	Metadata   map[string]any `json:"metadata"`
}

// chunkRow 数据库扫描行（向量检索结果）
type chunkRow struct {
	ID         uint64
	DocumentID uint64
	Content    string
	Score      float64
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
		topK = 5
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
		topK = 5
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

// chunksToRAGChunks ragretrieval.Chunk → dto.RAGChunk
//
// 字段映射：
//   - Content: Chunk.Content → RAGChunk.Content（截断到 500 字）
//   - Score:   Chunk.Score → RAGChunk.Score
//   - DocID:   Chunk.DocumentID → RAGChunk.DocID（string 透传）
//   - ChunkID: Chunk.ID → RAGChunk.ChunkID（string 透传）
//   - Source:  固定 "hybrid"（区分 legacy 路径返回的 chunk）
func chunksToRAGChunks(chunks []ragretrieval.Chunk) []RAGChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]RAGChunk, 0, len(chunks))
	for _, c := range chunks {
		result = append(result, RAGChunk{
			Content: truncateText(c.Content, 500),
			Source:  "hybrid",
			Score:   c.Score,
			DocID:   c.DocumentID,
			ChunkID: c.ID,
		})
	}
	return result
}

// chunksToMerchantChunks ragretrieval.Chunk → MerchantRAGChunk
//
// 字段映射：
//   - ID / DocumentID: string → uint64（解析失败回退 0）
//   - Content / Score: 透传（Content 截断 500）
//   - Metadata: 携带 ChunkIndex / Title 用于上游展示
func chunksToMerchantChunks(chunks []ragretrieval.Chunk) []MerchantRAGChunk {
	if len(chunks) == 0 {
		return nil
	}
	result := make([]MerchantRAGChunk, 0, len(chunks))
	for _, c := range chunks {
		id, _ := strconv.ParseUint(c.ID, 10, 64)
		docID, _ := strconv.ParseUint(c.DocumentID, 10, 64)
		meta := c.Metadata
		if meta == nil {
			meta = make(map[string]any)
		}
		if c.Title != "" {
			meta["title"] = c.Title
		}
		meta["chunk_index"] = c.ChunkIndex
		result = append(result, MerchantRAGChunk{
			ID:         id,
			DocumentID: docID,
			Content:    truncateText(c.Content, 500),
			Score:      c.Score,
			Metadata:   meta,
		})
	}
	return result
}

// filterMerchantChunksByMetadata 按附加字段过滤分片。
// 当且仅当分片的 Metadata 包含 filters 中所有键值对（字符串相等）时才保留。
// 用于把检索收敛到特定业务上下文，例如 {"customer_id":"123","order_id":"A01"}。
func filterMerchantChunksByMetadata(chunks []MerchantRAGChunk, filters map[string]string) []MerchantRAGChunk {
	if len(filters) == 0 {
		return chunks
	}
	out := make([]MerchantRAGChunk, 0, len(chunks))
	for _, c := range chunks {
		if chunkMatchesMetadata(c.Metadata, filters) {
			out = append(out, c)
		}
	}
	return out
}

// chunkMatchesMetadata 判断分片元信息是否满足全部过滤条件。
func chunkMatchesMetadata(meta map[string]any, filters map[string]string) bool {
	for k, want := range filters {
		got, ok := meta[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", got) != want {
			return false
		}
	}
	return true
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

// vectorSearch 通过 pgvector 余弦相似度检索
//
// SQL 关键：
//   - WHERE embedding IS NOT NULL  → 只查已向量化的 chunk
//   - 1 - (embedding <=> $1::vector) AS score  → 把 pgvector 余弦距离(0=完全相同,2=完全相反)
//     转成余弦相似度(0~1)，与历史接口 Score 字段语义一致
//   - ORDER BY embedding <=> $1::vector  → HNSW 索引命中
//   - LIMIT $2  → 取 topK
func (s *RagSearcher) vectorSearch(ctx context.Context, productID int64, query string, topK int) ([]scored, error) {
	if s.embeddingService == nil {
		return nil, fmt.Errorf("embedding service 未初始化")
	}
	cfg := s.embeddingService.DefaultConfig()

	queryVec, err := s.embeddingService.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, fmt.Errorf("TEI 编码失败: %w", err)
	}

	// 关键:把 []float32 序列化为 pgvector 字面量字符串 '[0.1,0.2,...]'
	vecLiteral := vecToPGString(queryVec)

	// 用 GORM Raw + 参数绑定（防止 SQL 注入）
	var rows []chunkRow
	// 注意：HNSW 索引建在 embedding 列上
	// 余弦距离 <=> 范围 [0,2]，转换为相似度 = 1 - distance
	if productID > 0 {
		sql := `
			SELECT id, document_id, content,
			       (1 - (embedding <=> $1::vector))::float8 AS score
			FROM knowledge_chunks
			WHERE embedding IS NOT NULL
			  AND product_id = $2
			ORDER BY embedding <=> $1::vector
			LIMIT $3
		`
		if err := s.db.WithContext(ctx).Raw(sql, vecLiteral, productID, topK).Scan(&rows).Error; err != nil {
			return nil, err
		}
	} else {
		sql := `
			SELECT id, document_id, content,
			       (1 - (embedding <=> $1::vector))::float8 AS score
			FROM knowledge_chunks
			WHERE embedding IS NOT NULL
			ORDER BY embedding <=> $1::vector
			LIMIT $2
		`
		if err := s.db.WithContext(ctx).Raw(sql, vecLiteral, topK).Scan(&rows).Error; err != nil {
			return nil, err
		}
	}
	// 转 scored
	pairs := make([]scored, 0, len(rows))
	for _, r := range rows {
		pairs = append(pairs, scored{row: r, score: r.Score})
	}
	return pairs, nil
}

// bm25SearchAll 兜底:全产品 BM25-lite 检索
func (s *RagSearcher) bm25SearchAll(ctx context.Context, query string, topK int) ([]RAGChunk, error) {
	if s.db == nil {
		return nil, nil
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	var rows []chunkRow
	if err := s.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id, document_id, content").
		Where("embedding IS NULL OR embedding IS NOT NULL"). // 兜底时包含全部
		Limit(10000).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	return s.bm25RankAndReturn(rows, terms, topK), nil
}

// bm25SearchIndex 兜底:单产品 BM25-lite 检索
func (s *RagSearcher) bm25SearchIndex(ctx context.Context, productID int64, query string, topK int) ([]MerchantRAGChunk, error) {
	if s.db == nil {
		return nil, nil
	}
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil, nil
	}
	var rows []chunkRow
	if err := s.db.WithContext(ctx).
		Table("knowledge_chunks").
		Select("id, document_id, content").
		Where("product_id = ?", productID).
		Limit(10000).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	pairs := s.bm25Rank(rows, terms)
	if len(pairs) > topK {
		pairs = pairs[:topK]
	}
	result := make([]MerchantRAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, MerchantRAGChunk{
			ID:         p.row.ID,
			DocumentID: p.row.DocumentID,
			Content:    truncateText(p.row.Content, 500),
			Score:      p.score,
		})
	}
	return result, nil
}

// bm25RankAndReturn 排序并转 RAGChunk
func (s *RagSearcher) bm25RankAndReturn(rows []chunkRow, terms []string, topK int) []RAGChunk {
	pairs := s.bm25Rank(rows, terms)
	if len(pairs) > topK {
		pairs = pairs[:topK]
	}
	return s.toRAGChunks(pairs)
}

// bm25Rank BM25-lite 排序
type scored struct {
	row   chunkRow
	score float64
}

func (s *RagSearcher) bm25Rank(rows []chunkRow, terms []string) []scored {
	pairs := make([]scored, 0, len(rows))
	for _, r := range rows {
		sc := scoreText(r.Content, terms)
		if sc > 0 {
			pairs = append(pairs, scored{row: r, score: sc})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})
	return pairs
}

// toRAGChunks scored → dto.RAGChunk
func (s *RagSearcher) toRAGChunks(pairs []scored) []RAGChunk {
	result := make([]RAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, RAGChunk{
			Content: truncateText(p.row.Content, 500),
			Score:   p.score,
			DocID:   strconv.FormatUint(p.row.DocumentID, 10),
			ChunkID: strconv.FormatUint(p.row.ID, 10),
		})
	}
	return result
}

// toMerchantChunks []scored → []MerchantRAGChunk
func (s *RagSearcher) toMerchantChunks(pairs []scored) []MerchantRAGChunk {
	result := make([]MerchantRAGChunk, 0, len(pairs))
	for _, p := range pairs {
		result = append(result, MerchantRAGChunk{
			ID:         p.row.ID,
			DocumentID: p.row.DocumentID,
			Content:    truncateText(p.row.Content, 500),
			Score:      p.score,
		})
	}
	return result
}

// vecToPGString 把 []float32 序列化为 pgvector 字面量字符串
//
// pgvector 支持的格式: '[1.0,2.0,3.0,...]'
// 必须用科学计数或保留小数位，否则 PG 会报 dimension mismatch
func vecToPGString(v []float32) string {
	var b strings.Builder
	b.Grow(len(v) * 12)
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		// 'g' 格式：根据数值大小自动选择定点或科学计数，无尾随零
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

// tokenize 分词（中文按字符 + 英文按词）
func tokenize(text string) []string {
	text = strings.ToLower(text)
	terms := make([]string, 0)
	word := strings.Builder{}
	for _, r := range text {
		if r >= 0x4e00 && r <= 0x9fff {
			if word.Len() > 0 {
				terms = append(terms, word.String())
				word.Reset()
			}
			terms = append(terms, string(r))
		} else if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			word.WriteRune(r)
		} else {
			if word.Len() > 0 {
				terms = append(terms, word.String())
				word.Reset()
			}
		}
	}
	if word.Len() > 0 {
		terms = append(terms, word.String())
	}
	return terms
}

// scoreText BM25-lite 文本打分
func scoreText(text string, terms []string) float64 {
	lower := strings.ToLower(text)
	hits := 0
	for _, term := range terms {
		if strings.Contains(lower, term) {
			hits++
		}
	}
	if len(terms) == 0 {
		return 0
	}
	return float64(hits) / float64(len(terms))
}

// ScoreText BM25-lite 文本打分(公开版,供跨包调用)
func ScoreText(text string, terms []string) float64 {
	return scoreText(text, terms)
}

// truncateText 截断
func truncateText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
