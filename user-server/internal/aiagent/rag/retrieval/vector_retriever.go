package ragretrieval

// vector_retriever.go pgvector HNSW 向量召回
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.2
//
// SQL 关键:
//   - SET LOCAL hnsw.ef_search = $efSearch  → 查询期动态调整 HNSW 候选
//   - WHERE embedding IS NOT NULL          → 跳过未向量化的 chunk
//   - ORDER BY embedding <=> $1::vector     → 命中 HNSW 索引（vector_cosine_ops）
//   - 1 - (embedding <=> $1::vector) AS score → 转相似度（0~1，pgvector 余弦距离 0=完全相同 2=完全相反）
//
// 设计原则:
//   - embedding 通过 CachedEmbeddingClient 装饰，命中 Redis/DB 缓存直接返回
//   - 维度强约束 1024（与 TEI bge-m3 一致），不匹配直接报错
//   - SET LOCAL 包在事务内，不污染连接池
//   - 私域独立部署: 无 merchant_id 字段

import (
	"context"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"marketing/internal/aiagent/llm"
	"marketing/internal/pkg/utils/logger"
)

// VectorRetriever pgvector HNSW 向量召回器
type VectorRetriever struct {
	db              *gorm.DB
	embeddingClient llm.EmbeddingServiceInterface // 通常为 *CachedEmbeddingClient
	efSearch        int
}

// NewVectorRetriever 创建向量召回器
//
// efSearch <= 0 时使用默认值 80（设计文档推荐）
// embeddingClient 通常应传入 *CachedEmbeddingClient 装饰后的实例；
// 若直接传入 *llm.EmbeddingService 也能工作，只是无缓存。
func NewVectorRetriever(db *gorm.DB, embeddingClient llm.EmbeddingServiceInterface, efSearch int) *VectorRetriever {
	if efSearch <= 0 {
		efSearch = 80
	}
	return &VectorRetriever{
		db:              db,
		embeddingClient: embeddingClient,
		efSearch:        efSearch,
	}
}

// chunkScanRow 数据库扫描行
type chunkScanRow struct {
	ID         uint64  `gorm:"column:id"`
	DocumentID uint64  `gorm:"column:document_id"`
	Content    string  `gorm:"column:content"`
	Score      float64 `gorm:"column:score"`
}

// Retrieve 向量召回
//
// 参数:
//   - productID != "" 时按产品过滤；= 0 时全产品检索
//   - query 原始查询文本（将被 embeddingClient 编码为向量）
//   - topK 返回结果数（<= 0 时使用默认值 50）
//
// 返回:
//   - []Chunk 已按相似度降序排序
//   - 维度非法 / embedding 失败 / DB 错误均返回 error
func (r *VectorRetriever) Retrieve(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("vector retriever 未初始化")
	}
	if r.embeddingClient == nil {
		return nil, fmt.Errorf("embedding client 未初始化")
	}
	if topK <= 0 {
		topK = 50
	}
	cfg := r.embeddingClient.DefaultConfig()

	// 走 CachedEmbeddingClient 装饰器（命中 Redis 直接返回）
	queryVec, err := r.embeddingClient.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, fmt.Errorf("embedding 失败: %w", err)
	}
	// 维度强约束：与 TEI bge-m3 一致（1024 维）
	expectDim := 1024
	if cfg != nil && cfg.Dimension > 0 {
		expectDim = cfg.Dimension
	}
	if len(queryVec) != expectDim {
		return nil, fmt.Errorf("embedding 维度非法: expect %d, got %d", expectDim, len(queryVec))
	}
	vecLiteral := vecToPGString(queryVec)

	// 在事务内 SET LOCAL hnsw.ef_search，仅影响本次查询
	// 用 GORM Transaction 包裹，确保 SET LOCAL 不会污染连接池
	var rows []chunkScanRow
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("SET LOCAL hnsw.ef_search = %d", r.efSearch)).Error; err != nil {
			return fmt.Errorf("set hnsw.ef_search: %w", err)
		}
		sql := `
			SELECT id, document_id, content,
			       (1 - (embedding <=> ?::vector))::float8 AS score
			FROM knowledge_chunks
			WHERE embedding IS NOT NULL
		`
		args := []any{vecLiteral}
		if productID != "" {
			sql += " AND product_id = ? ORDER BY embedding <=> ?::vector LIMIT ?"
			args = append(args, productID, vecLiteral, topK)
		} else {
			sql += " ORDER BY embedding <=> ?::vector LIMIT ?"
			args = append(args, vecLiteral, topK)
		}
		return tx.Raw(sql, args...).Scan(&rows).Error
	})
	if err != nil {
		return nil, err
	}

	// 空召回告警：向量路返回 0 行，但库里仍存在未向量化（embed_status='pending' 或
	// embedding IS NULL）的 chunk 时，说明存在回填缺失，打印 Warn 便于发现，不改变正常返回。
	if len(rows) == 0 {
		if n := r.countUnembeddedChunks(ctx, productID); n > 0 {
			logger.Warnf("[VectorRetriever] 向量召回为空，但存在 %d 个未向量化 chunk (embed_status='pending' 或 embedding IS NULL)，疑似回填缺失；query=%q product_id=%q",
				n, query, productID)
		}
	}
	return rowsToChunks(rows), nil
}

// countUnembeddedChunks 统计指定 product 下仍存在未向量化 chunk 的数量。
//
// 判定：embed_status='pending' OR embedding IS NULL。仅用于空召回告警，不影响主流程；
// 任何错误（列不存在/无 product 过滤）返回 0（不打扰主链路）。
func (r *VectorRetriever) countUnembeddedChunks(ctx context.Context, productID string) int64 {
	sql := `
		SELECT COUNT(*) FROM knowledge_chunks
		WHERE (embed_status = 'pending' OR embedding IS NULL)
	`
	args := []any{}
	if productID != "" {
		sql += " AND product_id = ?"
		args = append(args, productID)
	}
	var n int64
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&n).Error; err != nil {
		return 0
	}
	return n
}

// rowsToChunks chunkScanRow → Chunk
func rowsToChunks(rows []chunkScanRow) []Chunk {
	out := make([]Chunk, 0, len(rows))
	for _, r := range rows {
		out = append(out, Chunk{
			ID:         strconv.FormatUint(r.ID, 10),
			DocumentID: strconv.FormatUint(r.DocumentID, 10),
			Content:    truncateContent(r.Content, 500),
			Score:      r.Score,
		})
	}
	return out
}
