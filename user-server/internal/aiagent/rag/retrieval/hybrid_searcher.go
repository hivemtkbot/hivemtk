package ragretrieval

import (
	"context"
	"sort"

	"hivemtk-user/internal/pkg/utils/logger"
)

// HybridSearcher Hybrid 检索器（USR-AI-02）
// 借鉴：Qdrant Hybrid Search + 2026 RAG 最佳实践
// 流水线：
//   1. 向量检索 (topK=50)
//   2. BM25 / tsvector 全文检索 (topK=30)
//   3. 融合（RRF / 加权）
//   4. Rerank (topK=5)
type HybridSearcher struct {
	vectorSearcher  VectorSearcher
	keywordSearcher KeywordSearcher
	reranker        RerankerInterface
	vectorWeight    float64
	keywordWeight   float64
}

// VectorSearcher 向量检索接口
type VectorSearcher interface {
	SearchVector(ctx context.Context, kbID string, queryVec []float32, topK int) ([]Chunk, error)
}

// KeywordSearcher 关键词检索接口（BM25 / tsvector）
type KeywordSearcher interface {
	SearchKeyword(ctx context.Context, kbID string, query string, topK int) ([]Chunk, error)
}

func NewHybridSearcher(vs VectorSearcher, ks KeywordSearcher, r RerankerInterface, vectorW, keywordW float64) *HybridSearcher {
	if vectorW == 0 {
		vectorW = 0.7
	}
	if keywordW == 0 {
		keywordW = 0.3
	}
	return &HybridSearcher{
		vectorSearcher:  vs,
		keywordSearcher: ks,
		reranker:        r,
		vectorWeight:    vectorW,
		keywordWeight:   keywordW,
	}
}

// HybridSearch 混合检索
func (h *HybridSearcher) HybridSearch(ctx context.Context, kbID string, query string, queryVec []float32, topK int) ([]Chunk, error) {
	// 1. 向量检索（更多候选）
	vecTopK := topK * 10
	if vecTopK < 50 {
		vecTopK = 50
	}
	vecResults, err := h.vectorSearcher.SearchVector(ctx, kbID, queryVec, vecTopK)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] 向量检索失败，仅用关键词")
		vecResults = nil
	}

	// 2. 关键词检索
	kwTopK := topK * 6
	if kwTopK < 30 {
		kwTopK = 30
	}
	kwResults, err := h.keywordSearcher.SearchKeyword(ctx, kbID, query, kwTopK)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[Hybrid] 关键词检索失败，仅用向量")
		kwResults = nil
	}

	// 3. 融合（RRF）
	fused := h.reciprocalRankFusion(vecResults, kwResults)

	// 4. Rerank
	if h.reranker != nil && len(fused) > topK {
		reranked, rerr := h.reranker.Rerank(ctx, query, toRerankDocs(fused))
		if rerr == nil {
			fused = applyRerank(fused, reranked)
		}
	}

	if len(fused) > topK {
		fused = fused[:topK]
	}
	return fused, nil
}

// reciprocalRankFusion 倒数排名融合（RRF）
// score = sum(weight / (k + rank))
// RRF 公式来源：https://plg.uwaterloo.ca/~gvcormac/cormacksal.j.pdf
func (h *HybridSearcher) reciprocalRankFusion(vecResults, kwResults []Chunk) []Chunk {
	const k = 60 // RRF 标准 k 值
	scores := make(map[string]float64)
	chunkMap := make(map[string]Chunk)

	// 向量结果排名
	for rank, c := range vecResults {
		key := c.DocumentID + "_" + c.ID
		scores[key] += h.vectorWeight / float64(k+rank+1)
		chunkMap[key] = c
	}

	// 关键词结果排名
	for rank, c := range kwResults {
		key := c.DocumentID + "_" + c.ID
		scores[key] += h.keywordWeight / float64(k+rank+1)
		if _, exists := chunkMap[key]; !exists {
			chunkMap[key] = c
		}
	}

	// 排序
	type kv struct {
		key   string
		score float64
	}
	pairs := make([]kv, 0, len(scores))
	for k, v := range scores {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].score > pairs[j].score
	})

	// 输出
	result := make([]Chunk, 0, len(pairs))
	for _, p := range pairs {
		if c, ok := chunkMap[p.key]; ok {
			c.Score = p.score
			result = append(result, c)
		}
	}
	return result
}
