package ragretrieval

import "testing"

// TestApplyRerankWritesBackScore 回归：applyRerank 必须把 reranker 的 relevance_score 写回 chunk.Score。
//
// 历史 bug：applyRerank 仅重排序、丢弃 relevance_score，导致 chunk.Score 停留在 RRF 量级(~0.03)。
// 下游 rag.search 工具用 threshold=0.3 过滤时，会把全部候选过滤，造成"永远空召回"(total=0)。
func TestApplyRerankWritesBackScore(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Content: "alpha", Score: 0.01},
		{ID: "c2", Content: "beta", Score: 0.02},
		{ID: "c3", Content: "gamma", Score: 0.03},
	}
	results := []RerankResult{
		{ID: "c2", Score: 0.91},
		{ID: "c1", Score: 0.62},
		{ID: "c3", Score: 0.20},
	}
	out := applyRerank(chunks, results)

	if len(out) != 3 {
		t.Fatalf("want 3 chunks, got %d", len(out))
	}
	want := []struct {
		id    string
		score float64
	}{
		{"c2", 0.91},
		{"c1", 0.62},
		{"c3", 0.20},
	}
	for i, w := range want {
		if out[i].ID != w.id {
			t.Errorf("out[%d].ID = %q, want %q", i, out[i].ID, w.id)
		}
		if out[i].Score != w.score {
			t.Errorf("out[%d].Score = %v, want %v (rerank relevance_score 必须写回 chunk.Score)", i, out[i].Score, w.score)
		}
	}
}

// TestApplyRerankKeepsUncoveredChunks 未出现在 rerank 结果中的分片应保留在末尾，
// 且其 Score 必须保持原值（不得被清零），以便降级路径仍可使用 RRF 顺序。
func TestApplyRerankKeepsUncoveredChunks(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Content: "a", Score: 0.01},
		{ID: "c2", Content: "b", Score: 0.02},
	}
	results := []RerankResult{{ID: "c1", Score: 0.8}}
	out := applyRerank(chunks, results)

	if len(out) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(out))
	}
	if out[0].ID != "c1" || out[0].Score != 0.8 {
		t.Errorf("out[0]=%s/%v, want c1/0.8", out[0].ID, out[0].Score)
	}
	if out[1].ID != "c2" || out[1].Score != 0.02 {
		t.Errorf("out[1]=%s/%v, want c2/0.02 (保留原 RRF score, 不得清零)", out[1].ID, out[1].Score)
	}
}

