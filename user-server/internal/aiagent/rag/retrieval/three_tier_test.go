package ragretrieval

import (
	"context"
	"errors"
	"testing"
)

// mock vectorizer
type mockVectorizer struct {
	dim int
}

func (m *mockVectorizer) EmbedText(text string) ([]float32, error) {
	if text == "" {
		return make([]float32, m.dim), nil
	}
	vec := make([]float32, m.dim)
	for i := 0; i < m.dim && i < len(text); i++ {
		vec[i] = float32(text[i]) / 255.0
	}
	return vec, nil
}

func (m *mockVectorizer) EmbedBatch(texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		v, _ := m.EmbedText(t)
		out[i] = v
	}
	return out, nil
}

func (m *mockVectorizer) GetDimension() int                  { return m.dim }
func (m *mockVectorizer) ValidateEmbedding(e []float32) bool { return len(e) == m.dim }

// mock index manager
type mockIndex struct {
	chunks map[string][]Chunk
	err    error
}

func (m *mockIndex) BuildIndex(ctx context.Context, kbID string, chunks []Chunk) error {
	if m.chunks == nil {
		m.chunks = make(map[string][]Chunk)
	}
	m.chunks[kbID] = chunks
	return nil
}
func (m *mockIndex) AddToIndex(ctx context.Context, kbID string, chunk Chunk) error {
	m.chunks[kbID] = append(m.chunks[kbID], chunk)
	return nil
}
func (m *mockIndex) RemoveFromIndex(ctx context.Context, kbID, chunkID string) error {
	return nil
}
func (m *mockIndex) SearchIndex(ctx context.Context, kbID string, q []float32, topK int) ([]Chunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.chunks[kbID], nil
}
func (m *mockIndex) DropIndex(ctx context.Context, kbID string) error { return nil }
func (m *mockIndex) GetIndexStats(ctx context.Context, kbID string) (*IndexStats, error) {
	return &IndexStats{KbID: kbID, VectorCount: len(m.chunks[kbID])}, nil
}

// mock keyword searcher
type mockKeyword struct {
	chunks []Chunk
	err    error
}

func (m *mockKeyword) Search(ctx context.Context, kbID, q string, topK int) ([]Chunk, error) {
	if m.err != nil {
		return nil, m.err
	}
	if topK > len(m.chunks) {
		topK = len(m.chunks)
	}
	return m.chunks[:topK], nil
}

// ============== RAGThreeTierService ==============

func newThreeTierService() (*RAGThreeTierService, *mockIndex, *mockIndex, *mockKeyword) {
	v := &mockVectorizer{dim: 16}
	l2 := &mockIndex{}
	l3 := &mockIndex{}
	kw := &mockKeyword{}
	s := NewRAGThreeTierService(nil, v, l2, l3, kw, 100)
	return s, l2, l3, kw
}

func TestRAGThreeTier_EmptyQuery(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	_, err := s.Search(context.Background(), "kb1", "", 5)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestRAGThreeTier_L2Hit(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", DocumentID: "d1", Content: "hello", Score: 0.9}},
	}
	r, err := s.Search(context.Background(), "kb1", "hello", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if r.Source != TierL2WarmIndex {
		t.Errorf("expected L2 source, got %s", r.Source)
	}
	if len(r.Chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(r.Chunks))
	}
}

func TestRAGThreeTier_L3Fallback(t *testing.T) {
	s, l2, l3, _ := newThreeTierService()
	// L2 失败
	l2.err = errors.New("l2 error")
	l3.chunks = map[string][]Chunk{
		"kb1": {{ID: "c3", DocumentID: "d3", Content: "cold", Score: 0.7}},
	}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source != TierL3ColdIndex {
		t.Errorf("expected L3, got %s", r.Source)
	}
}

func TestRAGThreeTier_L4KeywordFallback(t *testing.T) {
	s, l2, l3, kw := newThreeTierService()
	l2.err = errors.New("l2 fail")
	l3.err = errors.New("l3 fail")
	kw.chunks = []Chunk{{ID: "c4", Content: "kw"}}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source != TierL4Keyword {
		t.Errorf("expected L4, got %s", r.Source)
	}
}

func TestRAGThreeTier_NoHit(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source != "" {
		t.Errorf("expected empty source, got %s", r.Source)
	}
	if r.Metadata["reason"] != "no_hit" {
		t.Errorf("expected no_hit metadata, got %v", r.Metadata)
	}
}

func TestRAGThreeTier_L1Cache(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", Content: "hi", Score: 0.9}},
	}
	// 第一次
	r1, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r1.Source != TierL2WarmIndex {
		t.Errorf("first expected L2, got %s", r1.Source)
	}
	// 第二次应命中 L1
	r2, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r2.Source != TierL2WarmIndex {
		t.Errorf("second expected L2 (from cache), got %s", r2.Source)
	}
	if !r2.FromCache {
		t.Error("expected from cache")
	}
	stats := s.Stats()
	if stats.L1Hits < 1 {
		t.Errorf("expected L1 hit, got %d", stats.L1Hits)
	}
}

func TestRAGThreeTier_EnableDisableTier(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	s.EnableTier(TierL2WarmIndex, false)
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source == TierL2WarmIndex {
		t.Error("expected L2 disabled")
	}
	s.EnableTier(TierL2WarmIndex, true)
	if !s.IsEnabled(TierL2WarmIndex) {
		t.Error("expected enabled")
	}
}

func TestRAGThreeTier_ClearCache(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	s.Search(context.Background(), "kb1", "hi", 5)
	s.ClearCache()
	// 清空后应再次走 L2
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.FromCache {
		t.Error("expected not from cache after clear")
	}
}

func TestRAGThreeTier_Stats(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	s.Search(context.Background(), "kb1", "hi", 5)
	s.Search(context.Background(), "kb1", "hi", 5)
	stats := s.Stats()
	if stats.Total != 2 {
		t.Errorf("expected 2 total, got %d", stats.Total)
	}
	if stats.L2Hits+stats.L1Hits < 1 {
		t.Errorf("expected hit, got %+v", stats)
	}
}

func TestRAGThreeTier_MergeResults(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	r1 := []Chunk{{ID: "a", Score: 0.9}, {ID: "b", Score: 0.7}}
	r2 := []Chunk{{ID: "b", Score: 0.8}, {ID: "c", Score: 0.6}}
	merged := s.MergeResults(r1, r2)
	if len(merged) != 3 {
		t.Errorf("expected 3 unique, got %d", len(merged))
	}
	// 第一名应是 score 0.9 (a)
	if merged[0].ID != "a" {
		t.Errorf("expected a first, got %s", merged[0].ID)
	}
}

func TestRAGThreeTier_MergeResults_Empty(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	merged := s.MergeResults()
	if len(merged) != 0 {
		t.Errorf("expected empty, got %d", len(merged))
	}
}

func TestRAGThreeTier_EncodeDecode(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	orig := &TierSearchResult{
		Query:     "hi",
		Chunks:    []Chunk{{ID: "c1", Content: "data"}},
		Source:    TierL2WarmIndex,
		Score:     0.95,
		LatencyMs: 10,
	}
	b, err := s.EncodeResult(orig)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	r, err := s.DecodeResult(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r.Query != "hi" || r.Source != TierL2WarmIndex {
		t.Errorf("round trip mismatch: %+v", r)
	}
}

func TestRAGThreeTier_Warmup(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", Score: 0.9}},
	}
	queries := []string{"q1", "q2", "q3"}
	count, err := s.WarmupCache(context.Background(), "kb1", queries, 5)
	if err != nil {
		t.Fatalf("warmup: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestRAGThreeTier_CacheKey_DifferentQueries(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	k1 := s.cacheKey("kb", "hello", 5)
	k2 := s.cacheKey("kb", "world", 5)
	if k1 == k2 {
		t.Error("expected different keys")
	}
}

func TestRAGThreeTier_CacheKey_CaseInsensitive(t *testing.T) {
	s, _, _, _ := newThreeTierService()
	k1 := s.cacheKey("kb", "Hello", 5)
	k2 := s.cacheKey("kb", "hello", 5)
	if k1 != k2 {
		t.Error("expected case-insensitive key")
	}
}

func TestRAGThreeTier_DefaultTopK(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1"}}}
	r, _ := s.Search(context.Background(), "kb1", "hi", 0)
	if r == nil {
		t.Error("expected result")
	}
}

func TestRAGThreeTier_NoIndexers(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	s := NewRAGThreeTierService(nil, v, nil, nil, nil, 10)
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r == nil {
		t.Error("expected result")
	}
}

func TestRAGThreeTier_VectorizerError(t *testing.T) {
	// 模拟 vectorizer 错误：当前实现为降级（log 后继续下一层），不返回 error
	bad := &badVectorizer{}
	l2 := &mockIndex{}
	s := NewRAGThreeTierService(nil, bad, l2, nil, nil, 10)
	r, err := s.Search(context.Background(), "kb1", "hi", 5)
	if err != nil {
		t.Fatalf("unexpected error (graceful degrade expected): %v", err)
	}
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	// 所有层都不可用时，应返回 no_hit
	if r.Source != "" {
		t.Errorf("expected empty source (degrade), got %s", r.Source)
	}
	if r.Metadata["reason"] != "no_hit" {
		t.Errorf("expected no_hit reason, got %v", r.Metadata)
	}
}

type badVectorizer struct{}

func (b *badVectorizer) EmbedText(s string) ([]float32, error)      { return nil, errors.New("vector err") }
func (b *badVectorizer) EmbedBatch(s []string) ([][]float32, error) { return nil, nil }
func (b *badVectorizer) GetDimension() int                          { return 0 }
func (b *badVectorizer) ValidateEmbedding(e []float32) bool         { return false }

func TestRAGThreeTier_NilService(t *testing.T) {
	var s *RAGThreeTierService
	_, err := s.Search(context.Background(), "kb", "hi", 5)
	if err == nil {
		t.Error("expected nil service error")
	}
}

func TestRAGThreeTier_OnlyL1(t *testing.T) {
	s := NewRAGThreeTierService(nil, nil, nil, nil, nil, 10)
	// 仅 L1 缓存可用
	s.EnableTier(TierL1HotCache, true)
	r, _ := s.Search(context.Background(), "kb", "hi", 5)
	if r == nil {
		t.Error("expected result")
	}
}

func TestRAGThreeTier_L2WithScore(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", Score: 0.5}, {ID: "c2", Score: 0.95}},
	}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Score != 0.95 {
		t.Errorf("expected max score 0.95, got %f", r.Score)
	}
}

func TestRAGThreeTier_L2NoResults_FallbackL3(t *testing.T) {
	s, l2, l3, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{} // 空
	l3.chunks = map[string][]Chunk{
		"kb1": {{ID: "c1", Score: 0.8}},
	}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source != TierL3ColdIndex {
		t.Errorf("expected L3, got %s", r.Source)
	}
}

func TestRAGThreeTier_Latency(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", r.LatencyMs)
	}
}

func TestRAGThreeTier_AllTiers(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	l2 := &mockIndex{}
	l3 := &mockIndex{}
	kw := &mockKeyword{}
	s := NewRAGThreeTierService(nil, v, l2, l3, kw, 10)
	if !s.IsEnabled(TierL1HotCache) {
		t.Error("L1 should be enabled by default")
	}
	if !s.IsEnabled(TierL2WarmIndex) {
		t.Error("L2 should be enabled when indexer != nil")
	}
	if !s.IsEnabled(TierL3ColdIndex) {
		t.Error("L3 should be enabled when coldIndex != nil")
	}
	if !s.IsEnabled(TierL4Keyword) {
		t.Error("L4 should be enabled when keyword != nil")
	}
}

func TestRAGThreeTier_NoIndexerButKeyword(t *testing.T) {
	v := &mockVectorizer{dim: 8}
	kw := &mockKeyword{chunks: []Chunk{{ID: "c1", Score: 0.5}}}
	s := NewRAGThreeTierService(nil, v, nil, nil, kw, 10)
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.Source != TierL4Keyword {
		t.Errorf("expected L4, got %s", r.Source)
	}
}

func TestRAGThreeTier_PutCache_Disabled(t *testing.T) {
	s, l2, _, _ := newThreeTierService()
	s.EnableTier(TierL1HotCache, false)
	l2.chunks = map[string][]Chunk{"kb1": {{ID: "c1", Score: 0.9}}}
	r, _ := s.Search(context.Background(), "kb1", "hi", 5)
	if r.FromCache {
		t.Error("expected no cache")
	}
	stats := s.Stats()
	if stats.L1Hits > 0 {
		t.Errorf("expected 0 L1 hit, got %d", stats.L1Hits)
	}
}
