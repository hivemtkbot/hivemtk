package ragretrieval

import (
	"testing"
)

func TestRRFFusion_DefaultK(t *testing.T) {
	f := NewRRFFusion(0)
	if f.K() != 60 {
		t.Errorf("default K=%d want=60", f.K())
	}
}

func TestRRFFusion_CustomK(t *testing.T) {
	f := NewRRFFusion(100)
	if f.K() != 100 {
		t.Errorf("custom K=%d want=100", f.K())
	}
}

func TestRRFFusion_EmptyInputs(t *testing.T) {
	f := NewRRFFusion(60)
	out := f.Fuse(nil, nil, 20)
	if len(out) != 0 {
		t.Errorf("empty input should return empty, got %d", len(out))
	}
}

func TestRRFFusion_SingleVecOnly(t *testing.T) {
	f := NewRRFFusion(60)
	vec := []Chunk{
		{ID: "1", Score: 0.9},
		{ID: "2", Score: 0.8},
		{ID: "3", Score: 0.7},
	}
	out := f.Fuse(vec, nil, 20)
	if len(out) != 3 {
		t.Fatalf("len=%d want=3", len(out))
	}
	expectedFirst := 1.0 / 61.0
	if abs(out[0].Score-expectedFirst) > 1e-6 {
		t.Errorf("first RRF score=%.6f want=%.6f", out[0].Score, expectedFirst)
	}
	if out[0].ID != "1" || out[1].ID != "2" || out[2].ID != "3" {
		t.Errorf("order wrong: %s %s %s", out[0].ID, out[1].ID, out[2].ID)
	}
}

func TestRRFFusion_SingleBM25Only(t *testing.T) {
	f := NewRRFFusion(60)
	bm25 := []Chunk{
		{ID: "10", Score: 5.0},
		{ID: "20", Score: 4.0},
	}
	out := f.Fuse(nil, bm25, 20)
	if len(out) != 2 {
		t.Fatalf("len=%d want=2", len(out))
	}
	if out[0].ID != "10" {
		t.Errorf("first ID=%s want=10", out[0].ID)
	}
}

func TestRRFFusion_BothPathsSameChunkGetsDoubleScore(t *testing.T) {
	f := NewRRFFusion(60)
	vec := []Chunk{
		{ID: "common", Score: 0.9},
		{ID: "vec_only", Score: 0.8},
	}
	bm25 := []Chunk{
		{ID: "common", Score: 5.0},
		{ID: "bm25_only", Score: 4.0},
	}
	out := f.Fuse(vec, bm25, 20)
	if len(out) != 3 {
		t.Fatalf("len=%d want=3 (common + vec_only + bm25_only)", len(out))
	}
	if out[0].ID != "common" {
		t.Errorf("first ID=%s want=common (double score)", out[0].ID)
	}
	expectedCommon := 2.0 / 61.0
	if abs(out[0].Score-expectedCommon) > 1e-6 {
		t.Errorf("common RRF score=%.6f want=%.6f", out[0].Score, expectedCommon)
	}
}

func TestRRFFusion_TopNTruncation(t *testing.T) {
	f := NewRRFFusion(60)
	vec := make([]Chunk, 10)
	for i := range vec {
		vec[i] = Chunk{ID: string(rune('a' + i))}
	}
	out := f.Fuse(vec, nil, 3)
	if len(out) != 3 {
		t.Errorf("topN=3 should truncate, got %d", len(out))
	}
}

func TestRRFFusion_DefaultTopNWhenZero(t *testing.T) {
	f := NewRRFFusion(60)
	vec := make([]Chunk, 30)
	for i := range vec {
		vec[i] = Chunk{ID: string(rune('a' + i))}
	}
	out := f.Fuse(vec, nil, 0)
	if len(out) != 20 {
		t.Errorf("topN=0 should default to 20, got %d", len(out))
	}
}

func TestRRFFusion_ScoreOverwritten(t *testing.T) {
	f := NewRRFFusion(60)
	vec := []Chunk{{ID: "1", Score: 0.999}}
	out := f.Fuse(vec, nil, 20)
	expected := 1.0 / 61.0
	if abs(out[0].Score-expected) > 1e-6 {
		t.Errorf("score should be RRF value=%.6f, got=%.6f", expected, out[0].Score)
	}
}

func TestRRFFusion_TieBreakByID(t *testing.T) {
	f := NewRRFFusion(60)
	vec := []Chunk{
		{ID: "z_first", Score: 0.9},
		{ID: "a_tie", Score: 0.8},
	}
	bm25 := []Chunk{
		{ID: "z_first", Score: 5.0},
		{ID: "a_tie", Score: 4.0},
	}
	out := f.Fuse(vec, bm25, 20)
	if out[0].ID != "z_first" {
		t.Errorf("first should be z_first (rank 1 in both paths), got %s", out[0].ID)
	}
	if out[1].ID != "a_tie" {
		t.Errorf("second should be a_tie, got %s", out[1].ID)
	}
}

func TestTrimChunks(t *testing.T) {
	chunks := []Chunk{{ID: "1"}, {ID: "2"}, {ID: "3"}, {ID: "4"}}
	if out := trimChunks(chunks, 2); len(out) != 2 {
		t.Errorf("trimChunks(2) len=%d want=2", len(out))
	}
	if out := trimChunks(chunks, 0); len(out) != 4 {
		t.Errorf("trimChunks(0) should return all, len=%d want=4", len(out))
	}
	if out := trimChunks(chunks, 10); len(out) != 4 {
		t.Errorf("trimChunks(10) should return all, len=%d want=4", len(out))
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
