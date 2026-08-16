package ragretrieval

import (
	"context"
	"strings"
)

// PgKeywordSearcher PostgreSQL tsvector 关键词检索（USR-AI-02）
// 实际使用时：knowledge_embeddings 表加 tsvector 列 + GIN 索引
type PgKeywordSearcher struct {
	// executor PG 执行器（注入）
}

func NewPgKeywordSearcher() *PgKeywordSearcher {
	return &PgKeywordSearcher{}
}

// SearchKeyword tsvector 全文检索
// 真实实现：SELECT id, document_id, content, ts_rank(tsv, query) AS score
//   FROM knowledge_embeddings, plainto_tsquery('simple', ?) query
//   WHERE kb_id = ? AND tsv @@ query
//   ORDER BY score DESC LIMIT ?
func (s *PgKeywordSearcher) SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error) {
	// 占位实现：返回空数组
	// 真实环境应注入 *sql.DB 或 *gorm.DB
	_ = ctx
	_ = kbID
	_ = query
	_ = topK
	return []Chunk{}, nil
}

// HighlightQuery 高亮关键词（前端展示用）
func (s *PgKeywordSearcher) HighlightQuery(content, query string) string {
	tokens := strings.Fields(query)
	highlighted := content
	for _, t := range tokens {
		if len(t) < 2 {
			continue
		}
		highlighted = strings.ReplaceAll(highlighted, t, "<mark>"+t+"</mark>")
	}
	return highlighted
}
