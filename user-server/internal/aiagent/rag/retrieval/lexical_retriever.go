package ragretrieval


import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// LexicalRetriever 词法召回器（PG tsvector + zhparser ts_rank）
// 注意：ts_rank 非真 BM25（无 k1/b、无 IDF 饱和）——D17 命名纠正；真 BM25 见决策 D21 候选 pg_search
type LexicalRetriever struct {
	db *gorm.DB
}

// NewLexicalRetriever 创建词法召回器
func NewLexicalRetriever(db *gorm.DB) *LexicalRetriever {
	return &LexicalRetriever{db: db}
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
func (r *LexicalRetriever) Retrieve(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
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
func (r *LexicalRetriever) SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error) {
	return r.Retrieve(ctx, kbID, query, topK)
}

// allowedTSCols 允许的 tsvector 列白名单
var allowedTSCols = map[string]string{
	"contextual_tsv": "contextual_tsv",
	"content_tsv":    "content_tsv",
}

// allowedTSConfigs 允许的文本搜索配置白名单
var allowedTSConfigs = map[string]string{
	"zh_rag":  "zh_rag",
	"simple":  "simple",
	"english": "english",
}

// tryTSQuery 尝试指定 tsvector 列 + 文本搜索配置
func (r *LexicalRetriever) tryTSQuery(ctx context.Context, productID string, query string, topK int, tsvCol, tsConfig string) ([]Chunk, error) {
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

// ilikeFallback ILIKE 兜底（tsvector 列/配置不存在时）
//
// 简单按 query 子串匹配 content，score = 1.0（无 BM25 排序能力，仅保证有召回）
func (r *LexicalRetriever) ilikeFallback(ctx context.Context, productID string, query string, topK int) ([]Chunk, error) {
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

