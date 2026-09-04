package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// vectorSearch 通过 pgvector 余弦相似度检索
//
// SQL 关键：
//   - WHERE embedding IS NOT NULL  → 只查已向量化的 chunk
//   - 1 - (embedding <=> $1::vector) AS score  → 把 pgvector 余弦距离(0=完全相同,2=完全相反)
//     转成余弦相似度(0~1)，与历史接口 Score 字段语义一致
//   - ORDER BY embedding <=> $1::vector  → HNSW 索引命中
//   - LIMIT $2  → 取 topK
func (s *RagSearcher) vectorSearch(ctx context.Context, productID string, query string, topK int) ([]scored, error) {
	if s.embeddingService == nil {
		return nil, fmt.Errorf("embedding service 未初始化")
	}
	cfg := s.embeddingService.DefaultConfig()

	queryVec, err := s.embeddingService.EmbedOne(ctx, cfg, query)
	if err != nil {
		return nil, fmt.Errorf("TEI 编码失败: %w", err)
	}

	vecLiteral := vecToPGString(queryVec)

	// 用 GORM Raw + 参数绑定（防止 SQL 注入）
	var rows []chunkRow
	if productID != "" {
		sql := `
			SELECT id, document_id, content,
			       (1 - (embedding <=> ?::vector))::float8 AS score
			FROM knowledge_chunks
			WHERE embedding IS NOT NULL
			  AND embedding_source = 'tei'
			  AND product_id = ?
			ORDER BY embedding <=> ?::vector
			LIMIT ?
		`
		if err := s.db.WithContext(ctx).Raw(sql, vecLiteral, productID, vecLiteral, topK).Scan(&rows).Error; err != nil {
			return nil, err
		}
	} else {
		sql := `
			SELECT id, document_id, content,
			       (1 - (embedding <=> ?::vector))::float8 AS score
			FROM knowledge_chunks
			WHERE embedding IS NOT NULL
			  AND embedding_source = 'tei'
			ORDER BY embedding <=> ?::vector
			LIMIT ?
		`
		if err := s.db.WithContext(ctx).Raw(sql, vecLiteral, vecLiteral, topK).Scan(&rows).Error; err != nil {
			return nil, err
		}
	}
	pairs := make([]scored, 0, len(rows))
	for _, r := range rows {
		pairs = append(pairs, scored{row: r, score: r.Score})
	}
	return pairs, nil
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
		b.WriteString(strconv.FormatFloat(float64(f), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

