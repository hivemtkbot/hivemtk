package ragretrieval

import (
	"context"

	rag_core "hivemtk-user/internal/aiagent/rag/core"
	rag_service "hivemtk-user/internal/aiagent/rag/service"
)

// RAGThreeTierAdapter 将 RAGThreeTierService 适配为 rag_service.ThreeTierSearcher 接口
// 解决 rag_service 不直接依赖 rag_retrieval 包的问题
type RAGThreeTierAdapter struct {
	svc *RAGThreeTierService
}

// NewRAGThreeTierAdapter 构造适配器
func NewRAGThreeTierAdapter(svc *RAGThreeTierService) *RAGThreeTierAdapter {
	return &RAGThreeTierAdapter{svc: svc}
}

// Search 适配 Search 接口
func (a *RAGThreeTierAdapter) Search(ctx context.Context, kbID, query string, topK int) (*rag_service.ThreeTierResult, error) {
	if a.svc == nil {
		return nil, nil
	}
	res, err := a.svc.Search(ctx, kbID, query, topK)
	if err != nil {
		return nil, err
	}
	chunks := make([]rag_core.Chunk, 0, len(res.Chunks))
	for _, c := range res.Chunks {
		chunks = append(chunks, rag_core.Chunk{
			ID:         c.ID,
			DocumentID: c.DocumentID,
			Content:    c.Content,
			Metadata:   c.Metadata,
			Embedding:  c.Embedding,
			Score:      c.Score,
			TokenCount: c.TokenCount,
		})
	}
	return &rag_service.ThreeTierResult{
		Query:     res.Query,
		Chunks:    chunks,
		Source:    string(res.Source),
		Score:     res.Score,
		LatencyMs: res.LatencyMs,
		FromCache: res.FromCache,
	}, nil
}

// Stats 适配 Stats 接口
func (a *RAGThreeTierAdapter) Stats() rag_service.ThreeTierStats {
	if a.svc == nil {
		return rag_service.ThreeTierStats{}
	}
	stats := a.svc.Stats()
	return rag_service.ThreeTierStats{
		L1Hits: stats.L1Hits,
		L2Hits: stats.L2Hits,
		L3Hits: stats.L3Hits,
		L4Hits: stats.L4Hits,
		Misses: stats.Misses,
		Total:  stats.Total,
		AvgMs:  stats.AvgMs,
	}
}

