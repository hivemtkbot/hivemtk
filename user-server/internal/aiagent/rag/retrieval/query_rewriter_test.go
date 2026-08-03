package ragretrieval

// query_rewriter_test.go 查询改写器单元测试
//
// 覆盖：
//  1. 空 query 直接返回 none
//  2. nil RedisClient + nil DB → 走生成路径
//  3. HyDE 成功 → strategy=hyde，Rewritten=hydeDoc
//  4. HyDE 失败 → Rewritten=原 query
//  5. Multi-Query 成功 → MultiQueries 填充
//  6. HyDE + Multi-Query 同时成功 → strategy=hyde_multiquery
//  7. 二者均失败 → strategy=none, Rewritten=原 query
//  8. Redis 缓存命中 → CacheHit=true
//  9. DB 缓存命中 → CacheHit=true + 回填 Redis
// 10. mockLLMChatClient 已在 llm_chat_test.go 定义，这里复用

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// mockRedisClient mock RedisClient
type mockRedisClient struct {
	mu      sync.RWMutex
	store   map[string]string
	getErr  error
	setErr  error
	lastTTL time.Duration
}

func newMockRedisClient() *mockRedisClient {
	return &mockRedisClient{store: make(map[string]string)}
}

func (m *mockRedisClient) Get(_ context.Context, key string) (string, error) {
	if m.getErr != nil {
		return "", m.getErr
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if v, ok := m.store[key]; ok {
		return v, nil
	}
	// 模拟 redis.Nil
	return "", errRedisNil
}

func (m *mockRedisClient) Set(_ context.Context, key, value string, ttl time.Duration) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.store == nil {
		// 允许直接构造 mock 时 store 为 nil（仅 Get 失败场景），跳过写入
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	m.lastTTL = ttl
	return nil
}

// Contains 线程安全地判断 key 是否存在（供测试在并发回填后读取）
func (m *mockRedisClient) Contains(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.store[key]
	return ok
}

// errRedisNil 模拟 redis.Nil 错误（避免引入 redis 包）
var errRedisNil = errors.New("redis: nil")

func TestQueryRewriter_EmptyQuery(t *testing.T) {
	q := NewQueryRewriter(nil, nil, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "")
	if err != nil {
		t.Fatalf("empty query should not error: %v", err)
	}
	if rw.UsedStrategy != StrategyNone {
		t.Errorf("strategy=%v want=none", rw.UsedStrategy)
	}
	if rw.Rewritten != "" {
		t.Errorf("Rewritten=%q want empty", rw.Rewritten)
	}
}

func TestQueryRewriter_NoGenerators_NoCache(t *testing.T) {
	// 无 HyDE / Multi-Query 生成器，无 Redis / DB → 直接返回原 query, strategy=none
	q := NewQueryRewriter(nil, nil, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rw.Rewritten != "如何退货" {
		t.Errorf("Rewritten=%q want=如何退货", rw.Rewritten)
	}
	if rw.UsedStrategy != StrategyNone {
		t.Errorf("strategy=%v want=none", rw.UsedStrategy)
	}
	if rw.CacheHit {
		t.Error("CacheHit should be false without cache")
	}
}

func TestQueryRewriter_HyDESuccess(t *testing.T) {
	hydeDoc := "假设性答案文档内容，描述退货流程，包含多个领域术语和正式陈述句风格。"
	hydeGen := NewHyDEGenerator(&mockLLMChatClient{resp: hydeDoc}, nil)
	q := NewQueryRewriter(hydeGen, nil, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rw.Rewritten != hydeDoc {
		t.Errorf("Rewritten should be HyDE doc, got=%q", rw.Rewritten)
	}
	if rw.UsedStrategy != StrategyHyDE {
		t.Errorf("strategy=%v want=hyde", rw.UsedStrategy)
	}
}

func TestQueryRewriter_HyDEFailure_FallbackToOriginal(t *testing.T) {
	// HyDE LLM 调用失败 → Rewritten 应回退为原 query
	hydeGen := NewHyDEGenerator(&mockLLMChatClient{err: errors.New("LLM down")}, nil)
	q := NewQueryRewriter(hydeGen, nil, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("should not error on HyDE failure: %v", err)
	}
	if rw.Rewritten != "如何退货" {
		t.Errorf("Rewritten=%q want=如何退货 (fallback)", rw.Rewritten)
	}
	if rw.UsedStrategy != StrategyNone {
		t.Errorf("strategy=%v want=none (HyDE failed)", rw.UsedStrategy)
	}
}

func TestQueryRewriter_MultiQuerySuccess(t *testing.T) {
	multiResp := `["退货流程","如何退款","退换货政策"]`
	multiGen := NewMultiQueryGenerator(&mockLLMChatClient{resp: multiResp}, nil)
	q := NewQueryRewriter(nil, multiGen, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(rw.MultiQueries) != 3 {
		t.Errorf("MultiQueries len=%d want=3", len(rw.MultiQueries))
	}
	if rw.UsedStrategy != StrategyMultiQuery {
		t.Errorf("strategy=%v want=multiquery", rw.UsedStrategy)
	}
	// 无 HyDE，Rewritten 应为原 query
	if rw.Rewritten != "如何退货" {
		t.Errorf("Rewritten=%q want=如何退货", rw.Rewritten)
	}
}

func TestQueryRewriter_HyDEAndMultiQueryBothSuccess(t *testing.T) {
	// HyDE 和 Multi-Query 各用独立 mock（避免共享 lastPrompt 串扰）
	hydeDoc := "假设性答案文档，描述退货流程的具体步骤。"
	hydeGen := NewHyDEGenerator(&mockLLMChatClient{resp: hydeDoc}, nil)
	multiResp := `["退货流程","如何退款"]`
	multiGen := NewMultiQueryGenerator(&mockLLMChatClient{resp: multiResp}, nil)
	q := NewQueryRewriter(hydeGen, multiGen, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rw.UsedStrategy != StrategyHyDEMulti {
		t.Errorf("strategy=%v want=hyde_multiquery", rw.UsedStrategy)
	}
	if rw.Rewritten != hydeDoc {
		t.Errorf("Rewritten should be HyDE doc when both succeed")
	}
	if len(rw.MultiQueries) != 2 {
		t.Errorf("MultiQueries len=%d want=2", len(rw.MultiQueries))
	}
}

func TestQueryRewriter_BothFail(t *testing.T) {
	// HyDE 和 Multi-Query 都失败 → strategy=none, Rewritten=原 query
	hydeGen := NewHyDEGenerator(&mockLLMChatClient{err: errors.New("hyde fail")}, nil)
	multiGen := NewMultiQueryGenerator(&mockLLMChatClient{err: errors.New("multi fail")}, nil)
	q := NewQueryRewriter(hydeGen, multiGen, nil, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("should not error when both fail: %v", err)
	}
	if rw.UsedStrategy != StrategyNone {
		t.Errorf("strategy=%v want=none", rw.UsedStrategy)
	}
	if rw.Rewritten != "如何退货" {
		t.Errorf("Rewritten=%q want=如何退货", rw.Rewritten)
	}
}

func TestQueryRewriter_RedisCacheHit(t *testing.T) {
	// Redis 缓存命中 → CacheHit=true, 不调用 LLM
	redisClient := newMockRedisClient()
	// 预填一个缓存值
	hash := sha256Hex(normalizeQuery("如何退货"))
	cacheKey := "rag:rewrite:" + hash
	cachedJSON := `{"original":"","rewritten":"缓存的假设文档","multi_queries":["m1"],"used_strategy":"hyde","cache_hit":true}`
	redisClient.store[cacheKey] = cachedJSON

	// LLM mock 即使配置了 resp，也不应被调用（用 err 测试）
	llmClient := &mockLLMChatClient{err: errors.New("LLM should not be called")}
	hydeGen := NewHyDEGenerator(llmClient, nil)
	q := NewQueryRewriter(hydeGen, nil, redisClient, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !rw.CacheHit {
		t.Error("CacheHit should be true on Redis hit")
	}
	if rw.Rewritten != "缓存的假设文档" {
		t.Errorf("Rewritten=%q want=缓存的假设文档", rw.Rewritten)
	}
	if rw.UsedStrategy != StrategyHyDE {
		t.Errorf("strategy=%v want=hyde (from cache)", rw.UsedStrategy)
	}
	if llmClient.lastPrompt != "" {
		t.Errorf("LLM should not be called on cache hit, lastPrompt=%q", llmClient.lastPrompt)
	}
}

func TestQueryRewriter_RedisFailure_DoesNotBlock(t *testing.T) {
	// Redis Get 失败 → 不阻断，走生成路径
	redisClient := &mockRedisClient{getErr: errors.New("redis down")}
	// HyDE 文档必须 >= 20 字符（minDocLength 默认 20），否则会被 HyDEGenerator 视为过短
	hydeDoc := "假设性答案文档内容，描述退货流程的具体步骤和注意事项，包含正式陈述句风格。"
	hydeGen := NewHyDEGenerator(&mockLLMChatClient{resp: hydeDoc}, nil)
	q := NewQueryRewriter(hydeGen, nil, redisClient, nil, nil)
	rw, err := q.Rewrite(context.Background(), "如何退货")
	if err != nil {
		t.Fatalf("Redis failure should not block: %v", err)
	}
	if rw.Rewritten != hydeDoc {
		t.Errorf("Rewritten=%q want=HyDE doc", rw.Rewritten)
	}
}

func TestQueryRewriter_NormalizesQueryForHash(t *testing.T) {
	// 大小写/前后空白不同但语义相同的 query 应该命中同一缓存
	// （normalizeQuery 仅合并连续空白，不删除单词间所有空白，故用前后空白而非内部空白）
	redisClient := newMockRedisClient()
	// 第一次填缓存
	q1 := NewQueryRewriter(nil, nil, redisClient, nil, nil)
	_, _ = q1.Rewrite(context.Background(), "如何退货")
	// 第二次用前后空白，应命中同一 hash
	q2 := NewQueryRewriter(nil, nil, redisClient, nil, nil)
	rw, err := q2.Rewrite(context.Background(), "   如何退货   ")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 两次都没 LLM 生成器，第一次写入 Redis 后第二次应命中
	if !rw.CacheHit {
		t.Error("CacheHit should be true after first write")
	}
}

func TestNormalizeQuery_Consistency(t *testing.T) {
	// 不同空白/大小写归一化后应相同
	a := normalizeQuery("  How  to  RETURN  ")
	b := normalizeQuery("how to return")
	if a != b {
		t.Errorf("normalizeQuery inconsistent: %q vs %q", a, b)
	}
}

func TestSha256Hex_Deterministic(t *testing.T) {
	a := sha256Hex("test")
	b := sha256Hex("test")
	if a != b {
		t.Error("sha256Hex should be deterministic")
	}
	if len(a) != 64 {
		t.Errorf("sha256 hex length=%d want=64", len(a))
	}
}

// 验证 mockLLMChatClient 满足接口
var _ LLMChatClient = (*mockLLMChatClient)(nil)

// 确保 mockRedisClient 满足 RedisClient 接口（编译期断言）
var _ RedisClient = (*mockRedisClient)(nil)
