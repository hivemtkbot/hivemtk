package llm

import (
	"context"
	"errors"
	"testing"
)

// A6+F3：永久性错误跳过 Secondary 升级，但免费缓存层仍应尝试（可能有此前有效答案）
func TestFallbackTree_PermanentErrorSkipsSecondaryButTriesCache(t *testing.T) {
	withFallbackChainFlag(t, "true")
	tree := newTestDecisionTree()
	secondaryCalled := false
	cacheQueried := false
	content, level, err := tree.ExecuteWithFallback(
		context.Background(),
		"prompt",
		func(ctx context.Context, provider string) (string, error) {
			if provider == tree.secondaryProvider {
				secondaryCalled = true
			}
			return "", errors.New("request rejected: content_policy_violation")
		},
		func(ctx context.Context, key string) (string, bool) {
			cacheQueried = true
			return "", false
		},
		1000,
	)
	if err != nil {
		t.Fatalf("模板兜底后不应返回错误: %v", err)
	}
	if level != LevelTemplate {
		t.Fatalf("期望落模板级，实际 %s", level)
	}
	if secondaryCalled {
		t.Fatalf("永久性错误不得升级 Secondary")
	}
	if !cacheQueried {
		t.Fatalf("缓存层免费且安全，应仍被尝试")
	}
	if content != "抱歉, 服务暂时不可用, 请稍后重试。" {
		t.Fatalf("应返回模板文案，实际 %q", content)
	}
}

// 永久性错误 + 缓存命中：直接用缓存答案
func TestFallbackTree_PermanentErrorCacheHit(t *testing.T) {
	withFallbackChainFlag(t, "true")
	tree := newTestDecisionTree()
	content, level, err := tree.ExecuteWithFallback(
		context.Background(),
		"prompt",
		func(ctx context.Context, provider string) (string, error) {
			return "", errors.New("content_policy_violation")
		},
		func(ctx context.Context, key string) (string, bool) {
			return "cached-good-answer", true
		},
		1000,
	)
	if err != nil || level != LevelCache || content != "cached-good-answer" {
		t.Fatalf("永久错误+缓存命中应返回缓存: level=%s content=%q err=%v", level, content, err)
	}
}

// 瞬时错误照常走完整降级链
func TestFallbackTree_TransientErrorStillEscalates(t *testing.T) {
	withFallbackChainFlag(t, "true")
	tree := newTestDecisionTree()
	secondaryCalled := false
	content, level, err := tree.ExecuteWithFallback(
		context.Background(),
		"prompt",
		func(ctx context.Context, provider string) (string, error) {
			if provider == tree.primaryProvider {
				return "", errors.New("connection refused")
			}
			secondaryCalled = true
			return "ok-from-3b", nil
		},
		nil,
		1000,
	)
	if err != nil || level != LevelSecondary || content != "ok-from-3b" {
		t.Fatalf("瞬时错误应升级 Secondary 成功: level=%s err=%v", level, err)
	}
	if !secondaryCalled {
		t.Fatalf("Secondary 未被调用")
	}
}

func TestIsPermanentLLMError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("connection refused"), false},
		{errors.New("429 rate limited"), false},
		{errors.New("Content policy violation detected"), true},
		{errors.New("CONTEXT_LENGTH_EXCEEDED: max 8192 tokens"), true},
		{errors.New("moderation blocked"), true},
	}
	for i, c := range cases {
		if got := isPermanentLLMError(c.err); got != c.want {
			t.Fatalf("case %d: %v → want %v got %v", i, c.err, c.want, got)
		}
	}
}
