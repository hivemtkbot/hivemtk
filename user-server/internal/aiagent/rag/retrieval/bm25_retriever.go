package ragretrieval

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// BM25Retriever PostgreSQL tsvector + zhparser BM25 召回器
type BM25Retriever struct {
	db *gorm.DB
}

// NewBM25Retriever 创建 BM25 召回器
func NewBM25Retriever(db *gorm.DB) *BM25Retriever {
	return &BM25Retriever{db: db}
}

// Retrieve BM25 召回
//
// 参数:
//   - productID != "" 时按产品过滤；= 0 时全产品检索
//   - query 原始查询文本（zhparser 自动分词，无需手动 tokenize）
//   - topK 返回结果数（<= 0 时使用默认值 50）
//
// 容错策略:
//  1. 优先 contextual_tsv @@ plainto_tsquery('zh_rag', $1)
//  2. 失败（列/配置不存在）→ fallback content_tsv @@ plainto_tsquery('simple', $1)
//  3. 仍失败 → fallback content ILIKE '%query%'（保证召回不阻断主流程）
func (r *BM25Retriever) Retrieve(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("bm25 retriever 未初始化")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 50
	}

	if rows, err := r.tryTSQuery(ctx, productID, query, topK, "contextual_tsv", "zh_rag"); err == nil && len(rows) > 0 {
		return rows, nil
	}

	if rows, err := r.tryTSQuery(ctx, productID, query, topK, "content_tsv", "simple"); err == nil && len(rows) > 0 {
		return rows, nil
	}

	return r.ilikeFallback(ctx, productID, query, topK)
}

// SearchKeyword 实现 KeywordSearcher 接口
func (r *BM25Retriever) SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error) {
	return r.Retrieve(ctx, kbID, query, topK)
}

var allowedTSCols = map[string]string{
	"contextual_tsv": "contextual_tsv",
	"content_tsv":    "content_tsv",
}

var allowedTSConfigs = map[string]string{
	"zh_rag":  "zh_rag",
	"simple":  "simple",
	"english": "english",
}

func (r *BM25Retriever) tryTSQuery(ctx context.Context, productID string, query string, topK int, tsvCol, tsConfig string) ([]Chunk, error) {
	safeCol, ok := allowedTSCols[tsvCol]
	if !ok {
		return nil, fmt.Errorf("invalid tsvector column: %s", tsvCol)
	}
	safeConfig, ok := allowedTSConfigs[tsConfig]
	if !ok {
		return nil, fmt.Errorf("invalid text search config: %s", tsConfig)
	}
	sql := fmt.Sprintf(`
		SELECT id, document_id, content,
		       ts_rank(%s, plainto_tsquery('%s', ?))::float8 AS score
		FROM knowledge_chunks
		WHERE %s @@ plainto_tsquery('%s', ?)
	`, safeCol, safeConfig, safeCol, safeConfig)
	args := []any{query}
	if productID != "" {
		sql += " AND product_id = ? ORDER BY score DESC LIMIT ?"
		args = append(args, query, productID, topK)
	} else {
		sql += " ORDER BY score DESC LIMIT ?"
		args = append(args, query, topK)
	}
	var rows []chunkScanRow
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}

func (r *BM25Retriever) ilikeFallback(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	pattern := "%" + escaped + "%"
	sql := `
		SELECT id, document_id, content, 1.0::float8 AS score
		FROM knowledge_chunks
		WHERE content ILIKE ?
	`
	args := []any{pattern}
	if productID != "" {
		sql += " AND product_id = ? ORDER BY id DESC LIMIT ?"
		args = append(args, productID, topK)
	} else {
		sql += " ORDER BY id DESC LIMIT ?"
		args = append(args, topK)
	}
	var rows []chunkScanRow
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}
