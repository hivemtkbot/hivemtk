package ragretrieval


import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hivemtk-user/internal/aiagent/llm"
)

// mockEmbeddingService mock llm.EmbeddingServiceInterface
type mockEmbeddingService struct {
	mu       sync.Mutex
	called   int
	lastText []string
	vectors  [][]float32
	useEmpty bool 
	err      error
}

func (m *mockEmbeddingService) Embed(_ context.Context, _ *llm.EmbeddingConfig, texts []string) ([][]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called++
	m.lastText = texts
	if m.err != nil {
		return nil, m.err
	}
	if m.useEmpty {
		return [][]float32{}, nil
	}
	if len(m.vectors) > 0 {
		return m.vectors, nil
	}
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = makeFixedVector(1024, float32(i+1))
	}
	return out, nil
}

func (m *mockEmbeddingService) EmbedOne(ctx context.Context, cfg *llm.EmbeddingConfig, text string) ([]float32, error) {
	vectors, err := m.Embed(ctx, cfg, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("empty")
	}
	return vectors[0], nil
}

func (m *mockEmbeddingService) DefaultConfig() *llm.EmbeddingConfig {
	return &llm.EmbeddingConfig{
		Model:     "bge-m3",
		Dimension: 1024,
	}
}

func (m *mockEmbeddingService) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

// makeFixedVector 生成固定值的向量
func makeFixedVector(dim int, val float32) []float32 {
	v := make([]float32, dim)
	for i := range v {
		v[i] = val
	}
	return v
}

func TestCachedEmbedding_EmptyInput(t *testing.T) {
	inner := &mockEmbeddingService{}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	out, err := c.Embed(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("empty input should return empty, got=%d", len(out))
	}
	if inner.callCount() != 0 {
		t.Errorf("inner should not be called for empty input, called=%d", inner.callCount())
	}
}

func TestCachedEmbedding_DisableCache_CallInner(t *testing.T) {
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	cfg := &CachedEmbeddingClientConfig{DisableCache: true}
	c := NewCachedEmbeddingClient(inner, redisClient, nil, cfg)
	_, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if inner.callCount() != 1 {
		t.Errorf("inner should be called once when cache disabled, got=%d", inner.callCount())
	}
	if len(redisClient.store) != 0 {
		t.Errorf("Redis should not be written when cache disabled, store=%d", len(redisClient.store))
	}
}

func TestCachedEmbedding_RedisHit_NoInnerCall(t *testing.T) {
	inner := &mockEmbeddingService{err: errors.New("inner should not be called")}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	model := inner.DefaultConfig().Model
	text := "hello"
	key := c.cacheKey(model, text)
	expectedVec := makeFixedVector(1024, 0.5)
	redisClient.store[key] = encodeVec(expectedVec)

	out, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{text})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out)=%d want=1", len(out))
	}
	if len(out[0]) != 1024 {
		t.Errorf("vector dim=%d want=1024", len(out[0]))
	}
	for i, v := range out[0] {
		if v != 0.5 {
			t.Errorf("vec[%d]=%v want=0.5 (first only)", i, v)
			break
		}
	}
	if inner.callCount() != 0 {
		t.Errorf("inner should NOT be called on Redis hit, called=%d", inner.callCount())
	}
}

func TestCachedEmbedding_RedisMiss_CallInnerAndBackfill(t *testing.T) {
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	cfg := inner.DefaultConfig()
	text := "hello world"
	out, err := c.Embed(context.Background(), cfg, []string{text})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("len(out)=%d want=1", len(out))
	}
	if inner.callCount() != 1 {
		t.Errorf("inner should be called once on miss, called=%d", inner.callCount())
	}

	time.Sleep(50 * time.Millisecond)

	key := c.cacheKey(cfg.Model, text)
	if !redisClient.Contains(key) {
		t.Errorf("Redis should be backfilled after miss, key=%s not found", key)
	}
}

func TestCachedEmbedding_RedisFailure_DoesNotBlock(t *testing.T) {
	inner := &mockEmbeddingService{}
	redisClient := &mockRedisClient{getErr: errors.New("redis down")}
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	_, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{"hello"})
	if err != nil {
		t.Fatalf("Redis failure should not block: %v", err)
	}
	if inner.callCount() != 1 {
		t.Errorf("inner should be called when Redis fails, called=%d", inner.callCount())
	}
}

func TestCachedEmbedding_InnerError_Propagates(t *testing.T) {
	innerErr := errors.New("TEI down")
	inner := &mockEmbeddingService{err: innerErr}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)

	_, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{"hello"})
	if !errors.Is(err, innerErr) {
		t.Errorf("err=%v want=%v", err, innerErr)
	}
}

func TestCachedEmbedding_DefaultConfig_Passthrough(t *testing.T) {
	inner := &mockEmbeddingService{}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	cfg := c.DefaultConfig()
	if cfg.Model != "bge-m3" {
		t.Errorf("DefaultConfig Model=%q want=bge-m3", cfg.Model)
	}
	if cfg.Dimension != 1024 {
		t.Errorf("DefaultConfig Dimension=%d want=1024", cfg.Dimension)
	}
}

func TestCachedEmbedding_EmbedOne_ReturnsVector(t *testing.T) {
	inner := &mockEmbeddingService{}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	vec, err := c.EmbedOne(context.Background(), inner.DefaultConfig(), "hello")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(vec) != 1024 {
		t.Errorf("vec dim=%d want=1024", len(vec))
	}
}

func TestCachedEmbedding_EmbedOne_EmptyInner(t *testing.T) {
	inner := &mockEmbeddingService{useEmpty: true}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	_, err := c.EmbedOne(context.Background(), inner.DefaultConfig(), "hello")
	if err == nil {
		t.Error("EmbedOne should error when inner returns empty")
	}
}

func TestCachedEmbedding_DimensionMismatch_TreatedAsMiss(t *testing.T) {
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	model := inner.DefaultConfig().Model
	text := "hello"
	key := c.cacheKey(model, text)
	wrongVec := makeFixedVector(512, 0.5)
	redisClient.store[key] = encodeVec(wrongVec)

	out, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{text})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 1 || len(out[0]) != 1024 {
		t.Errorf("should fallback to inner and return 1024-dim, got len=%d", len(out))
	}
	if inner.callCount() != 1 {
		t.Errorf("inner should be called on dim mismatch, called=%d", inner.callCount())
	}
}

func TestCachedEmbedding_MultipleTexts(t *testing.T) {
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	cfg := inner.DefaultConfig()
	text1 := "cached"
	text2 := "fresh"
	key1 := c.cacheKey(cfg.Model, text1)
	redisClient.store[key1] = encodeVec(makeFixedVector(1024, 0.7))

	out, err := c.Embed(context.Background(), cfg, []string{text1, text2})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("len(out)=%d want=2", len(out))
	}
	if out[0][0] != 0.7 {
		t.Errorf("out[0][0]=%v want=0.7 (cached)", out[0][0])
	}
	if out[1][0] != 1.0 {
		t.Errorf("out[1][0]=%v want=1.0 (fresh)", out[1][0])
	}
	if inner.callCount() != 1 {
		t.Errorf("inner call count=%d want=1 (only for fresh text)", inner.callCount())
	}
}

func TestCachedEmbedding_NilRedisAndDB_CallInner(t *testing.T) {
	inner := &mockEmbeddingService{}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	_, err := c.Embed(context.Background(), inner.DefaultConfig(), []string{"hello"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if inner.callCount() != 1 {
		t.Errorf("inner should be called once, got=%d", inner.callCount())
	}
}

// 接口编译期断言
var _ llm.EmbeddingServiceInterface = (*mockEmbeddingService)(nil)
var _ llm.EmbeddingServiceInterface = (*CachedEmbeddingClient)(nil)

