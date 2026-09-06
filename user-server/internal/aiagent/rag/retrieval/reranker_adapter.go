package ragretrieval

import (
	"context"
)

// RerankChunks 对 chunks 重排，返回按相关性降序的 chunks
//
// 复用 rerank.go 中已有的 toRerankDocs / applyRerank / RerankerInterface.Rerank
// 失败时返回 error；调用方决定是否回退到融合顺序
func RerankChunks(ctx context.Context, reranker RerankerInterface, query string, chunks []Chunk) ([]Chunk, error) {
	if reranker == nil || len(chunks) == 0 {
		return chunks, nil
	}
	docs := toRerankDocs(chunks)
	results, err := reranker.Rerank(ctx, query, docs)
	if err != nil {
		return nil, err
	}
	return applyRerank(chunks, results), nil
}
