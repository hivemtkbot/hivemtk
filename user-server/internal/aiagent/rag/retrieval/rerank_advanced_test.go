package ragretrieval


import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)


// mockRerankerInstance 可控的 RerankerInterface 实现
type mockRerankerInstance struct {
	mu           sync.Mutex
	callCount    int32
	scores       map[string]float64 
	err          error
	latency      time.Duration
	recordInputs bool
	lastQuery    string
	lastDocs     []RerankDoc
}

func newMockReranker(scores map[string]float64) *mockRerankerInstance {
	return &mockRerankerInstance{scores: scores}
}

func (m *mockRerankerInstance) Rerank(ctx context.Context, query string, docs []RerankDoc) ([]RerankResult, error) {
	atomic.AddInt32(&m.callCount, 1)
	m.mu.Lock()
	if m.recordInputs {
		m.lastQuery = query
		m.lastDocs = append([]RerankDoc(nil), docs...)
	}
	if m.latency > 0 {
		m.mu.Unlock()
		select {
		case <-time.After(m.latency):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		m.mu.Lock()
	}
	defer m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	out := make([]RerankResult, 0, len(docs))
	for _, d := range docs {
		s, ok := m.scores[d.ID]
		if !ok {
			s = 0.5 
		}
		out = append(out, RerankResult{ID: d.ID, Score: s})
	}
	return out, nil
}

func (m *mockRerankerInstance) calls() int32 { return atomic.LoadInt32(&m.callCount) }
func (m *mockRerankerInstance) setErr(err error) {
	m.mu.Lock()
	m.err = err
	m.mu.Unlock()
}


// 1) TestCrossEncoderRerank_Basic 基本重排
func TestCrossEncoderRerank_Basic(t *testing.T) {
	scores := map[string]float64{
		"d1": 0.9,
		"d2": 0.3,
		"d3": 0.7,
	}
	mock := newMockReranker(scores)
	scorer := NewCrossEncoderScorer(mock, nil)
	reranker := NewCrossEncoderReranker(scorer)

	docs := []RetrievedDoc{
		{ID: "d1", Content: "doc1", Score: 0.5, Source: "vector"},
		{ID: "d2", Content: "doc2", Score: 0.8, Source: "vector"},
		{ID: "d3", Content: "doc3", Score: 0.6, Source: "vector"},
	}
	ranked, err := reranker.Rerank(context.Background(), "q", docs, 3)
	if err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("len=%d want=3", len(ranked))
	}
	if ranked[0].ID != "d1" || ranked[1].ID != "d3" || ranked[2].ID != "d2" {
		t.Errorf("order wrong: %s %s %s", ranked[0].ID, ranked[1].ID, ranked[2].ID)
	}
	if ranked[0].Rank != 1 {
		t.Errorf("rank=1 expected, got=%d", ranked[0].Rank)
	}
	if ranked[0].Strategy != string(StrategyCrossEncoder) {
		t.Errorf("strategy=%s want=%s", ranked[0].Strategy, StrategyCrossEncoder)
	}
	if !ranked[0].Recomputed {
		t.Errorf("Recomputed should be true for cross-encoder")
	}
}

// 2) TestCrossEncoderRerank_EmptyInput 空输入
func TestCrossEncoderRerank_EmptyInput(t *testing.T) {
	mock := newMockReranker(nil)
	scorer := NewCrossEncoderScorer(mock, nil)
	reranker := NewCrossEncoderReranker(scorer)
	ranked, err := reranker.Rerank(context.Background(), "q", nil, 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ranked != nil {
		t.Errorf("empty input should return nil, got %d", len(ranked))
	}
}

// 3) TestCrossEncoderRerank_DelegateFailure 降级（delegate 报错）
func TestCrossEncoderRerank_DelegateFailure(t *testing.T) {
	mock := newMockReranker(map[string]float64{"d1": 0.9})
	mock.setErr(errors.New("network unreachable"))
	scorer := NewCrossEncoderScorer(mock, nil)
	reranker := NewCrossEncoderReranker(scorer)

	docs := []RetrievedDoc{
		{ID: "d1", Content: "doc1", Score: 0.5},
		{ID: "d2", Content: "doc2", Score: 0.8},
	}
	ranked, err := reranker.Rerank(context.Background(), "q", docs, 2)
	if err != nil {
		t.Fatalf("should not propagate error, got %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("len=%d want=2", len(ranked))
	}
	if ranked[0].ID != "d2" {
		t.Errorf("first should be d2 (original 0.8), got %s", ranked[0].ID)
	}
	if ranked[0].Recomputed {
		t.Errorf("Recomputed should be false on fallback")
	}
}

// 4) TestCrossEncoderRerank_CacheHit 缓存命中
func TestCrossEncoderRerank_CacheHit(t *testing.T) {
	mock := newMockReranker(map[string]float64{"d1": 0.9})
	scorer := NewCrossEncoderScorer(mock, nil)
	reranker := NewCrossEncoderReranker(scorer)

	docs := []RetrievedDoc{{ID: "d1", Content: "doc1", Score: 0.5}}
	_, _ = reranker.Rerank(context.Background(), "q", docs, 1)
	callsAfterFirst := mock.calls()
	if callsAfterFirst != 1 {
		t.Fatalf("delegate should be called once, got=%d", callsAfterFirst)
	}
	_, _ = reranker.Rerank(context.Background(), "q", docs, 1)
	callsAfterSecond := mock.calls()
	if callsAfterSecond != 1 {
		t.Errorf("delegate should NOT be called again (cache hit), got=%d", callsAfterSecond-1)
	}
}


// 5) TestRRFReranker_BasicMultiSource 多路 RRF 融合
func TestRRFReranker_BasicMultiSource(t *testing.T) {
	r := NewRRFReranker(60, nil)
	docs := []RetrievedDoc{
		{ID: "common", Content: "c", Score: 0.9, Source: "vector"},
		{ID: "vec_only", Content: "v", Score: 0.7, Source: "vector"},
		{ID: "common", Content: "c", Score: 5.0, Source: "bm25"},
		{ID: "bm25_only", Content: "b", Score: 4.0, Source: "bm25"},
	}
	ranked, err := r.Rerank(context.Background(), "q", docs, 10)
	if err != nil {
		t.Fatalf("rrf: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("len=%d want=3 (common + vec_only + bm25_only)", len(ranked))
	}
	if ranked[0].ID != "common" {
		t.Errorf("first should be common (double score), got %s", ranked[0].ID)
	}
	expectedCommon := 2.0 / 61.0
	if absFloat(ranked[0].Score-expectedCommon) > 1e-6 {
		t.Errorf("common score=%.6f want=%.6f", ranked[0].Score, expectedCommon)
	}
}

// 6) TestRRFReranker_SingleSource 单源仍能排序
func TestRRFReranker_SingleSource(t *testing.T) {
	r := NewRRFReranker(60, nil)
	docs := []RetrievedDoc{
		{ID: "1", Score: 0.3, Source: "vector"},
		{ID: "2", Score: 0.9, Source: "vector"},
		{ID: "3", Score: 0.6, Source: "vector"},
	}
	ranked, err := r.Rerank(context.Background(), "q", docs, 10)
	if err != nil {
		t.Fatalf("rrf: %v", err)
	}
	if ranked[0].ID != "2" {
		t.Errorf("first should be 2 (highest score), got %s", ranked[0].ID)
	}
}

// 7) TestRRFReranker_EmptySource 空源处理
func TestRRFReranker_EmptySource(t *testing.T) {
	r := NewRRFReranker(60, nil)
	ranked, err := r.Rerank(context.Background(), "q", nil, 10)
	if err != nil || ranked != nil {
		t.Errorf("empty input should return nil,nil; got ranked=%v err=%v", ranked, err)
	}
}

// 8) TestRRFReranker_CustomK 自定义 k
func TestRRFReranker_CustomK(t *testing.T) {
	r := NewRRFReranker(100, nil)
	if r.k != 100 {
		t.Errorf("k=%d want=100", r.k)
	}
}


// 9) TestHybridRerank_WithCrossEncoder 混合策略（含 Cross-Encoder）
func TestHybridRerank_WithCrossEncoder(t *testing.T) {
	mock := newMockReranker(map[string]float64{
		"d1": 0.95,
		"d2": 0.20,
		"d3": 0.80,
		"d4": 0.40,
	})
	reranker := NewHybridReranker(60, mock, nil)
	docs := []RetrievedDoc{
		{ID: "d1", Score: 0.9, Source: "vector"},
		{ID: "d2", Score: 0.85, Source: "vector"},
		{ID: "d3", Score: 0.7, Source: "bm25"},
		{ID: "d4", Score: 0.6, Source: "bm25"},
	}
	ranked, err := reranker.Rerank(context.Background(), "q", docs, 3)
	if err != nil {
		t.Fatalf("hybrid: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("len=%d want=3 (topK)", len(ranked))
	}
	if ranked[0].ID != "d1" {
		t.Errorf("first should be d1 (CE=0.95), got %s", ranked[0].ID)
	}
	if ranked[0].Strategy != string(StrategyHybrid) {
		t.Errorf("strategy=%s want=%s", ranked[0].Strategy, StrategyHybrid)
	}
}

// 10) TestHybridRerank_NoDelegate 无 Cross-Encoder 时降级为纯 RRF
func TestHybridRerank_NoDelegate(t *testing.T) {
	reranker := NewHybridReranker(60, nil, nil)
	docs := []RetrievedDoc{
		{ID: "d1", Score: 0.9, Source: "vector"},
		{ID: "d2", Score: 0.7, Source: "bm25"},
		{ID: "d1", Score: 5.0, Source: "bm25"}, 
	}
	ranked, err := reranker.Rerank(context.Background(), "q", docs, 5)
	if err != nil {
		t.Fatalf("hybrid: %v", err)
	}
	if len(ranked) == 0 {
		t.Fatalf("should have results")
	}
	if ranked[0].ID != "d1" {
		t.Errorf("first should be d1 (multi-path), got %s", ranked[0].ID)
	}
	if ranked[0].Strategy != string(StrategyHybrid) {
		t.Errorf("strategy=%s want=%s", ranked[0].Strategy, StrategyHybrid)
	}
}

// 11) TestHybridRerank_CrossEncoderFails Cross-Encoder 失败回退 RRF
func TestHybridRerank_CrossEncoderFails(t *testing.T) {
	mock := newMockReranker(map[string]float64{"d1": 0.9})
	mock.setErr(errors.New("service unavailable"))
	reranker := NewHybridReranker(60, mock, nil)
	docs := []RetrievedDoc{
		{ID: "d1", Score: 0.9, Source: "vector"},
		{ID: "d2", Score: 0.5, Source: "vector"},
	}
	ranked, err := reranker.Rerank(context.Background(), "q", docs, 2)
	if err != nil {
		t.Fatalf("should fallback gracefully, got %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("len=%d want=2", len(ranked))
	}
	if ranked[0].ID != "d1" {
		t.Errorf("first should be d1, got %s", ranked[0].ID)
	}
}

// 12) TestHybridRerank_TopKRespected topK 限制
func TestHybridRerank_TopKRespected(t *testing.T) {
	mock := newMockReranker(map[string]float64{})
	reranker := NewHybridReranker(60, mock, nil)
	docs := make([]RetrievedDoc, 30)
	for i := range docs {
		docs[i] = RetrievedDoc{ID: fmt.Sprintf("d%d", i), Score: float64(30 - i), Source: "vector"}
	}
	ranked, _ := reranker.Rerank(context.Background(), "q", docs, 5)
	if len(ranked) != 5 {
		t.Errorf("topK=5 should return 5, got %d", len(ranked))
	}
}


// 13) TestClampTopK topK 限制（≤20，≤0 用默认）
func TestClampTopK(t *testing.T) {
	if got := clampTopK(0); got != 20 {
		t.Errorf("clampTopK(0)=%d want=20", got)
	}
	if got := clampTopK(-5); got != 20 {
		t.Errorf("clampTopK(-5)=%d want=20", got)
	}
	if got := clampTopK(15); got != 15 {
		t.Errorf("clampTopK(15)=%d want=15", got)
	}
	if got := clampTopK(100); got != 20 {
		t.Errorf("clampTopK(100)=%d want=20 (max)", got)
	}
}

// 14) TestClampDocs docs 限制（≤100）
func TestClampDocs(t *testing.T) {
	small := make([]RetrievedDoc, 50)
	if got := clampDocs(small); len(got) != 50 {
		t.Errorf("50 docs should stay 50, got=%d", len(got))
	}
	big := make([]RetrievedDoc, 200)
	if got := clampDocs(big); len(got) != 100 {
		t.Errorf("200 docs should clamp to 100, got=%d", len(got))
	}
}

// 15) TestCache_BasicSetGet 缓存基础读写
func TestCache_BasicSetGet(t *testing.T) {
	c := newRerankScoreCache(100, time.Hour)
	c.Set("k1", 0.95)
	if v, ok := c.Get("k1"); !ok || v != 0.95 {
		t.Errorf("Get(k1)=%v,%v want 0.95,true", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Errorf("missing key should not hit")
	}
}

// 16) TestCache_Expiry 缓存过期
func TestCache_Expiry(t *testing.T) {
	c := newRerankScoreCache(100, 50*time.Millisecond)
	c.Set("k1", 0.9)
	if _, ok := c.Get("k1"); !ok {
		t.Errorf("should hit before expiry")
	}
	time.Sleep(80 * time.Millisecond)
	if _, ok := c.Get("k1"); ok {
		t.Errorf("should miss after expiry")
	}
}

// 17) TestCache_Eviction 缓存淘汰
func TestCache_Eviction(t *testing.T) {
	c := newRerankScoreCache(3, time.Hour)
	c.Set("k1", 1.0)
	c.Set("k2", 2.0)
	c.Set("k3", 3.0)
	c.Set("k4", 4.0) 
	if c.Len() != 3 {
		t.Errorf("Len=%d want=3 (cap)", c.Len())
	}
}

// 18) TestCacheKey 不同 query 不同 key
func TestCacheKey(t *testing.T) {
	k1 := cacheKey("query1", "d1")
	k2 := cacheKey("query2", "d1")
	if k1 == k2 {
		t.Errorf("different queries should produce different keys")
	}
	k3 := cacheKey("query1", "d1")
	if k1 != k3 {
		t.Errorf("same query+doc should produce same key")
	}
}


// 19) TestAdapter_Basic 适配器把 Reranker 转 RerankerInterface
func TestAdapter_Basic(t *testing.T) {
	mock := newMockReranker(map[string]float64{"d1": 0.9, "d2": 0.5})
	reranker := NewHybridReranker(60, mock, nil)
	adapter := NewRerankerToInterfaceAdapter(reranker)

	docs := []RerankDoc{
		{ID: "d1", Content: "doc1"},
		{ID: "d2", Content: "doc2"},
	}
	results, err := adapter.Rerank(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len=%d want=2", len(results))
	}
	if results[0].ID != "d1" {
		t.Errorf("first should be d1, got %s", results[0].ID)
	}
}

// 20) TestChunksToRetrievedDocs Chunk ↔ RetrievedDoc 转换
func TestChunksToRetrievedDocs(t *testing.T) {
	chunks := []Chunk{
		{ID: "c1", Content: "hello", Score: 0.9, Metadata: map[string]any{"k": "v"}},
		{ID: "c2", Content: "world", Score: 0.5},
	}
	docs := ChunksToRetrievedDocs(chunks, "vector")
	if len(docs) != 2 {
		t.Fatalf("len=%d want=2", len(docs))
	}
	if docs[0].Source != "vector" {
		t.Errorf("source=%s want=vector", docs[0].Source)
	}
	if docs[0].Score != 0.9 {
		t.Errorf("score=%.2f want=0.9", docs[0].Score)
	}

	original := make(map[string]Chunk, len(chunks))
	for _, c := range chunks {
		original[c.ID] = c
	}
	ranked := []RankedDoc{
		{ID: "c2", Score: 0.7},
		{ID: "c1", Score: 0.3},
	}
	out := RankedDocsToChunks(ranked, original)
	if len(out) != 2 {
		t.Fatalf("len=%d want=2", len(out))
	}
	if out[0].ID != "c2" {
		t.Errorf("first should be c2, got %s", out[0].ID)
	}
	if out[0].Score != 0.7 {
		t.Errorf("score should be 0.7, got %.2f", out[0].Score)
	}
}

// 21) TestDefaultReranker DefaultReranker 可构造
func TestDefaultReranker(t *testing.T) {
	r := DefaultReranker()
	if r == nil {
		t.Fatalf("DefaultReranker returned nil")
	}
	if r.Strategy() != string(StrategyHybrid) {
		t.Errorf("strategy=%s want=%s", r.Strategy(), StrategyHybrid)
	}
}

// 22) TestFallbackByOriginal 降级路径
func TestFallbackByOriginal(t *testing.T) {
	docs := []RetrievedDoc{
		{ID: "low", Score: 0.3},
		{ID: "high", Score: 0.9},
		{ID: "mid", Score: 0.5},
	}
	ranked := fallbackByOriginal(docs, 3, "test_strategy")
	if ranked[0].ID != "high" {
		t.Errorf("first should be high, got %s", ranked[0].ID)
	}
	if ranked[0].Strategy != "test_strategy" {
		t.Errorf("strategy=%s", ranked[0].Strategy)
	}
	if ranked[0].Recomputed {
		t.Errorf("fallback should set Recomputed=false")
	}
}

// 23) TestRankedDocs_StableSort 同分稳定排序
//
// RRF 公式：score = Σ 1/(k + rank_in_source)
//
// 输入顺序：b 和 a 在 vector 源中并列（都是 0.9），b 在 bm25 源中也并列（都是 0.5）。
// 由于稳定排序保持原顺序，b 在两路中都获得 rank 0（更小 RRF 分母 → 更大 RRF 分数）。
//
//	b: 1/(60+0+1) + 1/(60+0+1) = 2/61 ≈ 0.0328
//	a: 1/(60+1+1) + 1/(60+1+1) = 2/62 ≈ 0.0323
//
// 所以 b 总分 > a 总分，b 排在前面。
func TestRankedDocs_StableSort(t *testing.T) {
	r := NewRRFReranker(60, nil)
	docs := []RetrievedDoc{
		{ID: "b", Score: 0.9, Source: "vector"},
		{ID: "a", Score: 0.9, Source: "vector"},
		{ID: "b", Score: 0.5, Source: "bm25"},
		{ID: "a", Score: 0.5, Source: "bm25"},
	}
	ranked, _ := r.Rerank(context.Background(), "q", docs, 10)
	if len(ranked) != 2 {
		t.Fatalf("len=%d want=2", len(ranked))
	}
	if ranked[0].ID != "b" {
		t.Errorf("first should be b (higher RRF score), got %s", ranked[0].ID)
	}
	if ranked[1].ID != "a" {
		t.Errorf("second should be a, got %s", ranked[1].ID)
	}
}

// 24) TestCrossEncoderRerank_NilConstructor nil scorer 构造返回 nil
func TestCrossEncoderRerank_NilConstructor(t *testing.T) {
	if NewCrossEncoderReranker(nil) != nil {
		t.Errorf("nil scorer should produce nil reranker")
	}
	if NewCrossEncoderScorer(nil, nil) != nil {
		t.Errorf("nil delegate should produce nil scorer")
	}
}

// 25) TestCacheKey_QueryHashDeterministic query hash 稳定性
func TestCacheKey_QueryHashDeterministic(t *testing.T) {
	h1 := hashQuery("test_query")
	h2 := hashQuery("test_query")
	if h1 != h2 {
		t.Errorf("hashQuery should be deterministic")
	}
	if len(h1) != 16 {
		t.Errorf("hash length=%d want=16", len(h1))
	}
}

// 26) TestDescribeReranker 描述函数
func TestDescribeReranker(t *testing.T) {
	if DescribeReranker(nil) != "nil" {
		t.Errorf("nil should return 'nil'")
	}
	r := NewRRFReranker(60, nil)
	desc := DescribeReranker(r)
	if desc != "Reranker(strategy=rrf)" {
		t.Errorf("desc=%s", desc)
	}
}

// 27) TestTrimStrategyName 截断策略名
func TestTrimStrategyName(t *testing.T) {
	if got := TrimStrategyName("short"); got != "short" {
		t.Errorf("short should stay, got=%s", got)
	}
	long := "this_is_a_very_long_strategy_name_that_exceeds_32_chars"
	got := TrimStrategyName(long)
	if len(got) > 35 { 
		t.Errorf("should be truncated, got len=%d", len(got))
	}
}

// 28) TestRankedDoc_String String 方法
func TestRankedDoc_String(t *testing.T) {
	r := RankedDoc{ID: "x", Score: 0.5, Rank: 3, Strategy: "rrf"}
	s := r.String()
	if s == "" {
		t.Errorf("String should not be empty")
	}
}

// 29) TestScorer_NilContext 未初始化 scorer 报错
func TestScorer_NilContext(t *testing.T) {
	scorer := &CrossEncoderScorer{} 
	_, err := scorer.Score(context.Background(), "q", []RetrievedDoc{{ID: "d1"}})
	if err == nil {
		t.Errorf("should error on nil delegate")
	}
}

// 30) TestHybridRerank_NilReceiver nil receiver
func TestHybridRerank_NilReceiver(t *testing.T) {
	var r *HybridReranker
	_, err := r.Rerank(context.Background(), "q", []RetrievedDoc{{ID: "d1"}}, 5)
	if err == nil {
		t.Errorf("nil receiver should error")
	}
}

// 31) TestCrossEncoderRerank_TopKRespected topK 截断
func TestCrossEncoderRerank_TopKRespected(t *testing.T) {
	scores := map[string]float64{}
	docs := make([]RetrievedDoc, 30)
	for i := range docs {
		docs[i] = RetrievedDoc{ID: fmt.Sprintf("d%d", i)}
		scores[docs[i].ID] = float64(i) / 100.0
	}
	mock := newMockReranker(scores)
	scorer := NewCrossEncoderScorer(mock, nil)
	reranker := NewCrossEncoderReranker(scorer)
	ranked, _ := reranker.Rerank(context.Background(), "q", docs, 5)
	if len(ranked) != 5 {
		t.Errorf("topK=5 should return 5, got=%d", len(ranked))
	}
}

// 32) TestCache_ConcurrentSafe 并发安全
func TestCache_ConcurrentSafe(t *testing.T) {
	c := newRerankScoreCache(1000, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			c.Set(fmt.Sprintf("k%d", n), float64(n))
			_, _ = c.Get(fmt.Sprintf("k%d", n))
		}(i)
	}
	wg.Wait()
}

// absFloat 浮点绝对值（避免与 rrf_fusion_test.go 的 abs 冲突）
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

