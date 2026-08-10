package ragretrieval

// cached_embedding_client_test.go Embedding 缓存装饰器单元测试
//
// 覆盖：
//  1. 空输入直接返回
//  2. 无缓存（disableCache=true）直接走 inner
//  3. Redis 命中 → 不调 inner
//  4. Redis 未命中 → 调 inner → 回填 Redis
//  5. Redis 失败 → 不阻断，走 inner
//  6. inner 错误透传
//  7. DefaultConfig 透传
//  8. EmbedOne 调用 Embed
//  9. 维度不匹配视为未命中
// 10. EmbeddingServiceInterface 接口断言

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
	useEmpty bool // 显式返回空切片
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
	// 显式返回空切片
	if m.useEmpty {
		return [][]float32{}, nil
	}
	// 返回预置向量；若未预置，返回固定 1024 维向量
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

	// 预填 Redis 缓存
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
	// 验证向量内容
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

	// 等待异步回填完成（异步 goroutine）
	// 异步回填是 goroutine，给一点时间
	time.Sleep(50 * time.Millisecond)

	// 验证 Redis 已被回填
	key := c.cacheKey(cfg.Model, text)
	if !redisClient.Contains(key) {
		t.Errorf("Redis should be backfilled after miss, key=%s not found", key)
	}
}

func TestCachedEmbedding_RedisFailure_DoesNotBlock(t *testing.T) {
	// Redis Get 返回非 redis.Nil 错误 → 不阻断，走 inner
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
	// inner 返回空切片 → EmbedOne 应报错
	inner := &mockEmbeddingService{useEmpty: true}
	c := NewCachedEmbeddingClient(inner, nil, nil, nil)
	_, err := c.EmbedOne(context.Background(), inner.DefaultConfig(), "hello")
	if err == nil {
		t.Error("EmbedOne should error when inner returns empty")
	}
}

func TestCachedEmbedding_DimensionMismatch_TreatedAsMiss(t *testing.T) {
	// Redis 中缓存了维度不匹配的向量 → 视为未命中，走 inner
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	// 预填维度错误的向量（512 而非 1024）
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
	// 多个 text 混合命中（部分命中 Redis，部分未命中）
	inner := &mockEmbeddingService{}
	redisClient := newMockRedisClient()
	c := NewCachedEmbeddingClient(inner, redisClient, nil, nil)

	// 预填第一个 text 的缓存
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
	// 第一个应该来自缓存（值 0.7）
	if out[0][0] != 0.7 {
		t.Errorf("out[0][0]=%v want=0.7 (cached)", out[0][0])
	}
	// 第二个应该来自 inner（值 1，因为 makeFixedVector(1024, float32(0+1))）
	if out[1][0] != 1.0 {
		t.Errorf("out[1][0]=%v want=1.0 (fresh)", out[1][0])
	}
	// inner 应只被调用一次（只对未命中的 text2）
	if inner.callCount() != 1 {
		t.Errorf("inner call count=%d want=1 (only for fresh text)", inner.callCount())
	}
}

func TestCachedEmbedding_NilRedisAndDB_CallInner(t *testing.T) {
	// 既无 Redis 也无 DB → 直接走 inner
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
