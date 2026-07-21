package ragretrieval

// reranker_adapter.go 重排适配器
//
// 五层架构归属: L4 能力层
// 设计依据: docs/核心链路优化.md 第十四章 §14.4.5
//
// 复用现有 LocalReranker（rerank.go），仅做类型转换：
//   - toRerankDocs([]Chunk) → []RerankDoc（rerank.go 已有，无需重复实现）
//   - applyRerank([]Chunk, []RerankResult) → []Chunk（rerank.go 已有，无需重复实现）
// 本文件仅提供 Chunk 级别的便捷封装 RerankChunks，避免 HybridSearcher 重复样板代码

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
