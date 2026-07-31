package llm

// fallback_tree_test.go 4 级降级决策树单元测试 (T28)
//
// 设计依据: 2026-07-31 AI 智能体性能优化 (T20)
//
// 测试目标:
//   - TestDecisionTree_Decide_7BTo3B: 7B 失败 -> 3B
//   - TestDecisionTree_Decide_3BToCache: 3B 失败 + 缓存命中 -> Cache
//   - TestDecisionTree_Decide_3BToTemplate: 3B 失败 + 缓存 miss -> Template
//   - TestExecuteWithFallback_FullChain: 完整降级链执行

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"marketing/internal/pkg/featureflag"
)

// withFallbackChainFlag 临时设置 FF_FALLBACK_CHAIN, 测试结束恢复
//
// featureflag.Flag.Bool() 返回的是 cachedValue (后台 poller 每 5s 刷新一次),
// 单纯 Setenv 不会立即生效, 所以这里在设值后立即调用 ReloadAll 强制同步,
// 保证本测试用例中 featureflag.Get("fallback_chain").Bool() 立刻反映新 env。
func withFallbackChainFlag(t *testing.T, val string) {
	t.Helper()
	prev, hadPrev := os.LookupEnv("FF_FALLBACK_CHAIN")
	if val == "" {
		_ = os.Unsetenv("FF_FALLBACK_CHAIN")
	} else {
		_ = os.Setenv("FF_FALLBACK_CHAIN", val)
	}
	// 立即强制刷新所有 flag 的 cachedValue (避免 5s 轮询窗口导致测试假阳性)
	featureflag.DefaultManager().ReloadAll()
	t.Cleanup(func() {
		if hadPrev {
			_ = os.Setenv("FF_FALLBACK_CHAIN", prev)
		} else {
			_ = os.Unsetenv("FF_FALLBACK_CHAIN")
		}
		featureflag.DefaultManager().ReloadAll()
	})
}

func newTestDecisionTree() *DecisionTree {
	return NewDecisionTree(DecisionTreeConfig{
		PrimaryProvider:   "local-llama-7b-q5",
		SecondaryProvider: "local-llama-3b-q4",
		CacheEnabled:      true,
		TemplateEnabled:   true,
		TemplateFallback:  "抱歉, 服务暂时不可用, 请稍后重试。",
	})
}

// TestDecisionTree_Decide_7BTo3B 测试 7B 失败 -> 3B
func TestDecisionTree_Decide_7BTo3B(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	dec := tree.Decide(context.Background(), LevelPrimary, "timeout", "", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelSecondary {
		t.Errorf("expected level=secondary_3b, got %s", dec.Level)
	}
	if dec.FromLevel != LevelPrimary {
		t.Errorf("expected from=primary_7b, got %s", dec.FromLevel)
	}
	if dec.Provider != "local-llama-3b-q4" {
		t.Errorf("expected provider=local-llama-3b-q4, got %s", dec.Provider)
	}
	if dec.Reason != "timeout" {
		t.Errorf("expected reason=timeout, got %s", dec.Reason)
	}
	if !dec.CanRetry {
		t.Error("expected CanRetry=true")
	}
}

// TestDecisionTree_Decide_3BToCache 测试 3B 失败 + 缓存命中 -> Cache
func TestDecisionTree_Decide_3BToCache(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	dec := tree.Decide(context.Background(), LevelSecondary, "error", "llm_fallback:abc123", true)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelCache {
		t.Errorf("expected level=cache, got %s", dec.Level)
	}
	if dec.FromLevel != LevelSecondary {
		t.Errorf("expected from=secondary_3b, got %s", dec.FromLevel)
	}
	if dec.CacheKey != "llm_fallback:abc123" {
		t.Errorf("expected cache key to be passed through, got %q", dec.CacheKey)
	}
	if !dec.CanRetry {
		t.Error("expected CanRetry=true for cache hit")
	}
}

// TestDecisionTree_Decide_3BToTemplate 测试 3B 失败 + 缓存 miss -> Template
func TestDecisionTree_Decide_3BToTemplate(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	// 缓存 miss
	dec := tree.Decide(context.Background(), LevelSecondary, "rate_limit", "llm_fallback:def456", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template, got %s", dec.Level)
	}
	if dec.FromLevel != LevelSecondary {
		t.Errorf("expected from=secondary_3b, got %s", dec.FromLevel)
	}
	if !dec.CanRetry {
		t.Error("expected CanRetry=true")
	}
}

// TestDecisionTree_Decide_CacheMissToTemplate 测试缓存 miss -> Template
func TestDecisionTree_Decide_CacheMissToTemplate(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()
	dec := tree.Decide(context.Background(), LevelCache, "miss", "", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template, got %s", dec.Level)
	}
}

// TestDecisionTree_Decide_EndOfChain 测试最后一级不再降级
func TestDecisionTree_Decide_EndOfChain(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()
	dec := tree.Decide(context.Background(), LevelTemplate, "exhausted", "", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template (end of chain), got %s", dec.Level)
	}
	if dec.CanRetry {
		t.Error("expected CanRetry=false at end of chain")
	}
}

// TestDecisionTree_Decide_DisabledFallbackChain 测试关闭时直接跳到 Template
func TestDecisionTree_Decide_DisabledFallbackChain(t *testing.T) {
	withFallbackChainFlag(t, "0")
	tree := newTestDecisionTree()

	// 7B 失败 -> 关闭时直接 Template
	dec := tree.Decide(context.Background(), LevelPrimary, "any_reason", "", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template when chain disabled, got %s", dec.Level)
	}
	if dec.FromLevel != LevelPrimary {
		t.Errorf("expected from=primary_7b, got %s", dec.FromLevel)
	}
}

// TestDecisionTree_Decide_CacheDisabledButHit 测试缓存关闭时降级到 Template
func TestDecisionTree_Decide_CacheDisabledButHit(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := NewDecisionTree(DecisionTreeConfig{
		PrimaryProvider:   "p1",
		SecondaryProvider: "p2",
		CacheEnabled:      false, // 缓存关闭
		TemplateEnabled:   true,
		TemplateFallback:  "兜底",
	})
	// 3B 失败 + 即使 hasCacheHit=true, 缓存关闭时直接到 Template
	dec := tree.Decide(context.Background(), LevelSecondary, "error", "k", true)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template when cache disabled, got %s", dec.Level)
	}
}

// TestDecisionTree_Decide_3BNoTemplateConfigured 测试 3B 失败 + 无模板
func TestDecisionTree_Decide_3BNoTemplateConfigured(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := NewDecisionTree(DecisionTreeConfig{
		PrimaryProvider:   "p1",
		SecondaryProvider: "p2",
		CacheEnabled:      true,
		TemplateEnabled:   false, // 模板关闭
		TemplateFallback:  "",
	})
	dec := tree.Decide(context.Background(), LevelSecondary, "error", "k", false)
	if dec == nil {
		t.Fatal("expected non-nil decision")
	}
	if dec.Level != LevelTemplate {
		t.Errorf("expected level=template (no template configured), got %s", dec.Level)
	}
	if dec.Reason != "no_template_configured" {
		t.Errorf("expected reason=no_template_configured, got %s", dec.Reason)
	}
}

// TestFallbackLevel_String 测试降级级别名称
func TestFallbackLevel_String(t *testing.T) {
	tests := []struct {
		level FallbackLevel
		want  string
	}{
		{LevelPrimary, "primary_7b"},
		{LevelSecondary, "secondary_3b"},
		{LevelCache, "cache"},
		{LevelTemplate, "template"},
		{FallbackLevel(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("level %d: expected %q, got %q", tt.level, tt.want, got)
		}
	}
}

// TestExecuteWithFallback_PrimarySuccess 测试 7B 成功就直接返回
func TestExecuteWithFallback_PrimarySuccess(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	callCount := 0
	callProvider := func(ctx context.Context, provider string) (string, error) {
		callCount++
		return "7B 回复: 你好", nil
	}
	queryCache := func(ctx context.Context, key string) (string, bool) {
		t.Error("cache should not be queried when primary succeeds")
		return "", false
	}

	content, level, err := tree.ExecuteWithFallback(context.Background(), "你好世界", callProvider, queryCache, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "7B 回复: 你好" {
		t.Errorf("expected 7B content, got %q", content)
	}
	if level != LevelPrimary {
		t.Errorf("expected level=primary, got %s", level)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (primary), got %d", callCount)
	}
}

// TestExecuteWithFallback_SecondarySuccess 测试 7B 失败 -> 3B 成功
func TestExecuteWithFallback_SecondarySuccess(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	callCount := 0
	callProvider := func(ctx context.Context, provider string) (string, error) {
		callCount++
		if provider == "local-llama-7b-q5" {
			return "", errors.New("7B timeout")
		}
		return "3B 回复: 你好", nil
	}
	queryCache := func(ctx context.Context, key string) (string, bool) {
		return "", false
	}

	content, level, err := tree.ExecuteWithFallback(context.Background(), "你好世界", callProvider, queryCache, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "3B 回复: 你好" {
		t.Errorf("expected 3B content, got %q", content)
	}
	if level != LevelSecondary {
		t.Errorf("expected level=secondary, got %s", level)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls (primary failed + secondary), got %d", callCount)
	}
}

// TestExecuteWithFallback_CacheHit 测试 7B/3B 都失败 + 缓存命中
func TestExecuteWithFallback_CacheHit(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	callCount := 0
	callProvider := func(ctx context.Context, provider string) (string, error) {
		callCount++
		return "", errors.New("all fail")
	}
	cacheCalled := false
	queryCache := func(ctx context.Context, key string) (string, bool) {
		cacheCalled = true
		return "缓存回复: 你好", true
	}

	content, level, err := tree.ExecuteWithFallback(context.Background(), "你好世界", callProvider, queryCache, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "缓存回复: 你好" {
		t.Errorf("expected cache content, got %q", content)
	}
	if level != LevelCache {
		t.Errorf("expected level=cache, got %s", level)
	}
	if !cacheCalled {
		t.Error("expected cache to be queried")
	}
}

// TestExecuteWithFallback_TemplateFallback 测试全部失败 -> 模板兜底
func TestExecuteWithFallback_TemplateFallback(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	callProvider := func(ctx context.Context, provider string) (string, error) {
		return "", errors.New("all fail")
	}
	queryCache := func(ctx context.Context, key string) (string, bool) {
		return "", false
	}

	content, level, err := tree.ExecuteWithFallback(context.Background(), "你好世界", callProvider, queryCache, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "抱歉, 服务暂时不可用, 请稍后重试。" {
		t.Errorf("expected template content, got %q", content)
	}
	if level != LevelTemplate {
		t.Errorf("expected level=template, got %s", level)
	}
}

// TestExecuteWithFallback_AllExhausted 测试全部失败且无模板
func TestExecuteWithFallback_AllExhausted(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := NewDecisionTree(DecisionTreeConfig{
		PrimaryProvider:   "p1",
		SecondaryProvider: "p2",
		CacheEnabled:      true,
		TemplateEnabled:   false, // 模板关闭
		TemplateFallback:  "",
	})

	callProvider := func(ctx context.Context, provider string) (string, error) {
		return "", errors.New("fail")
	}
	queryCache := func(ctx context.Context, key string) (string, bool) {
		return "", false
	}

	_, _, err := tree.ExecuteWithFallback(context.Background(), "x", callProvider, queryCache, 5000)
	if err == nil {
		t.Error("expected error when all fallback levels exhausted")
	}
}

// TestExecuteWithFallback_DefaultTimeout 测试默认 timeout
func TestExecuteWithFallback_DefaultTimeout(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	callProvider := func(ctx context.Context, provider string) (string, error) {
		return "OK", nil
	}
	// timeoutMs=0 触发默认 60s
	_, _, err := tree.ExecuteWithFallback(context.Background(), "x", callProvider, nil, 0)
	if err != nil {
		t.Errorf("expected success, got %v", err)
	}
}

// TestExecuteWithFallback_FullChain 端到端测试完整 4 级降级链
func TestExecuteWithFallback_FullChain(t *testing.T) {
	withFallbackChainFlag(t, "1")
	tree := newTestDecisionTree()

	// 7B 失败 -> 3B 失败 -> 缓存 miss -> 模板
	callProviders := map[string]string{
		"local-llama-7b-q5": "",
		"local-llama-3b-q4": "",
	}
	callOrder := []string{}
	callProvider := func(ctx context.Context, provider string) (string, error) {
		callOrder = append(callOrder, provider)
		if content, ok := callProviders[provider]; ok && content != "" {
			return content, nil
		}
		return "", fmt.Errorf("provider %s failed", provider)
	}
	queryCache := func(ctx context.Context, key string) (string, bool) {
		return "", false
	}

	content, level, err := tree.ExecuteWithFallback(context.Background(), "test_prompt", callProvider, queryCache, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 验证调用顺序
	if len(callOrder) != 2 {
		t.Errorf("expected 2 provider calls (7B+3B), got %d: %v", len(callOrder), callOrder)
	}
	if callOrder[0] != "local-llama-7b-q5" {
		t.Errorf("expected first call to 7B, got %s", callOrder[0])
	}
	if callOrder[1] != "local-llama-3b-q4" {
		t.Errorf("expected second call to 3B, got %s", callOrder[1])
	}
	// 验证最终结果是模板
	if content != "抱歉, 服务暂时不可用, 请稍后重试。" {
		t.Errorf("expected template content, got %q", content)
	}
	if level != LevelTemplate {
		t.Errorf("expected level=template (end of chain), got %s", level)
	}
}
