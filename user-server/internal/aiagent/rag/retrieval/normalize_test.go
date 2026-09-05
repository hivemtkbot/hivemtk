package ragretrieval

import "testing"

// D17b: RRF 量纲归一化
func TestD17b_NormalizeRRFScores(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", Score: 0.033},
		{ID: "b", Score: 0.025},
		{ID: "c", Score: 0.016},
	}
	normalizeRRFScores(chunks)
	if chunks[0].Score != 1.0 {
		t.Errorf("最高分应=1, got %v", chunks[0].Score)
	}
	if chunks[2].Score != 0 {
		t.Errorf("最低分应=0, got %v", chunks[2].Score)
	}
	// 单调性保持
	if !(chunks[0].Score > chunks[1].Score && chunks[1].Score > chunks[2].Score) {
		t.Error("归一化后应保持单调")
	}
}

// 全同分 → 全 1
func TestD17b_NormalizeSameScores(t *testing.T) {
	chunks := []Chunk{{ID: "a", Score: 0.033}, {ID: "b", Score: 0.033}}
	normalizeRRFScores(chunks)
	for _, c := range chunks {
		if c.Score != 1.0 {
			t.Errorf("全同分应全 1, got %v", c.Score)
		}
	}
}
