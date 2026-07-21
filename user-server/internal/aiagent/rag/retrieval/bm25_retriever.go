package ragretrieval

// bm25_retriever.go PostgreSQL tsvector + zhparser BM25 召回
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.3
//
// SQL 关键:
//   - WHERE contextual_tsv @@ plainto_tsquery('zh_rag', $1) → 命中 GIN 索引
//   - ts_rank(contextual_tsv, plainto_tsquery('zh_rag', $1)) AS score → BM25 风格打分
//   - 优先用 contextual_tsv（含 Anthropic Contextual Retrieval 上下文）
//   - fallback 到 content_tsv（未做上下文增强时）
//
// 兼容性:
//   - 若 contextual_tsv 列不存在（迁移未执行），自动 fallback 到 content ILIKE 模糊匹配
//   - 若 zh_rag 配置不存在，fallback 到 simple 配置
//   - 私域独立部署: 无 merchant_id 字段

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
//   - productID > 0 时按产品过滤；= 0 时全产品检索
//   - query 原始查询文本（zhparser 自动分词，无需手动 tokenize）
//   - topK 返回结果数（<= 0 时使用默认值 50）
//
// 容错策略:
//  1. 优先 contextual_tsv @@ plainto_tsquery('zh_rag', $1)
//  2. 失败（列/配置不存在）→ fallback content_tsv @@ plainto_tsquery('simple', $1)
//  3. 仍失败 → fallback content ILIKE '%query%'（保证召回不阻断主流程）
func (r *BM25Retriever) Retrieve(ctx context.Context, productID int64, query string, topK int) ([]Chunk, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("bm25 retriever 未初始化")
	}
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}
	if topK <= 0 {
		topK = 50
	}

	// 路径 1: contextual_tsv + zh_rag（迁移后优先路径）
	if rows, err := r.tryTSQuery(ctx, productID, query, topK, "contextual_tsv", "zh_rag"); err == nil {
		return rows, nil
	}

	// 路径 2: content_tsv + simple（zhparser 未安装时）
	if rows, err := r.tryTSQuery(ctx, productID, query, topK, "content_tsv", "simple"); err == nil {
		return rows, nil
	}

	// 路径 3: ILIKE 兜底（保证至少有召回，不阻断主流程）
	return r.ilikeFallback(ctx, productID, query, topK)
}

// tryTSQuery 尝试指定 tsvector 列 + 文本搜索配置
func (r *BM25Retriever) tryTSQuery(ctx context.Context, productID int64, query string, topK int, tsvCol, tsConfig string) ([]Chunk, error) {
	sql := fmt.Sprintf(`
		SELECT id, document_id, content,
		       ts_rank(%s, plainto_tsquery('%s', $1))::float8 AS score
		FROM knowledge_chunks
		WHERE %s @@ plainto_tsquery('%s', $1)
	`, tsvCol, tsConfig, tsvCol, tsConfig)
	args := []any{query}
	if productID > 0 {
		sql += " AND product_id = $2 ORDER BY score DESC LIMIT $3"
		args = append(args, productID, topK)
	} else {
		sql += " ORDER BY score DESC LIMIT $2"
		args = append(args, topK)
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
func (r *BM25Retriever) ilikeFallback(ctx context.Context, productID int64, query string, topK int) ([]Chunk, error) {
	// 对 query 做最小转义（避免 % _ 通配符干扰）
	escaped := strings.NewReplacer("%", "\\%", "_", "\\_").Replace(query)
	pattern := "%" + escaped + "%"
	sql := `
		SELECT id, document_id, content, 1.0::float8 AS score
		FROM knowledge_chunks
		WHERE content ILIKE $1
	`
	args := []any{pattern}
	if productID > 0 {
		sql += " AND product_id = $2 ORDER BY id DESC LIMIT $3"
		args = append(args, productID, topK)
	} else {
		sql += " ORDER BY id DESC LIMIT $2"
		args = append(args, topK)
	}
	var rows []chunkScanRow
	if err := r.db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rowsToChunks(rows), nil
}
