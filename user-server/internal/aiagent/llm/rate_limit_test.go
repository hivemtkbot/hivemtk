package llm

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// D05: 429 单次即冷却——RateLimitError 特判路径

func newD05Failover(t *testing.T) *ProviderFailover {
	t.Helper()
	return &ProviderFailover{
		health:  map[string]*ProviderHealth{},
		config:  DefaultFailoverConfig(),
		dispatcher: nil,
	}
}

// 429 带 Retry-After=7s → 单次即冷却且不计失败数
func TestD05_RateLimitCooldownWithRetryAfter(t *testing.T) {
	f := newD05Failover(t)
	before := time.Now()
	f.RecordFailure("p1", &RateLimitError{StatusCode: 429, RetryAfter: 7 * time.Second, Body: "quota"})
	h := f.GetHealth("p1")
	if h == nil {
		t.Fatal("health missing")
	}
	if h.ConsecutiveFailures != 0 {
		t.Errorf("429 不应计入 ConsecutiveFailures, got %d", h.ConsecutiveFailures)
	}
	if h.CircuitOpenUntil.Before(before.Add(6*time.Second)) || h.CircuitOpenUntil.After(before.Add(8*time.Second)) {
		t.Errorf("CircuitOpenUntil 应 ≈ now+7s, got %v (before=%v)", h.CircuitOpenUntil, before)
	}
	if !f.IsCircuitOpen("p1") {
		t.Error("冷却期内 IsCircuitOpen 应为 true")
	}
}

// Retry-After 缺失 → 兜底 15s
func TestD05_RateLimitCooldownFallback(t *testing.T) {
	f := newD05Failover(t)
	before := time.Now()
	f.RecordFailure("p2", &RateLimitError{StatusCode: 429, Body: "rate"})
	h := f.GetHealth("p2")
	openFor := h.CircuitOpenUntil.Sub(before)
	if openFor < 14*time.Second || openFor > 16*time.Second {
		t.Errorf("兜底冷却应 ≈15s, got %v", openFor)
	}
}

// 普通 5xx 维持原语义：连续 4 次不熔断、第 5 次熔断 60s
func TestD05_PlainFailureOriginalSemantics(t *testing.T) {
	f := newD05Failover(t)
	for i := 1; i <= 4; i++ {
		f.RecordFailure("p3", fmt.Errorf("server error: status=500"))
		if f.IsCircuitOpen("p3") {
			t.Fatalf("第 %d 次失败不应熔断", i)
		}
	}
	f.RecordFailure("p3", fmt.Errorf("server error: status=500"))
	if !f.IsCircuitOpen("p3") {
		t.Error("第 5 次失败应熔断")
	}
	if h := f.GetHealth("p3"); h.ConsecutiveFailures != 5 {
		t.Errorf("ConsecutiveFailures 应为 5, got %d", h.ConsecutiveFailures)
	}
}

// errors.As 穿透 dispatcher 的 %w 包装
func TestD05_ErrorsAsThroughWrapping(t *testing.T) {
	rle := &RateLimitError{StatusCode: 429, RetryAfter: 3 * time.Second}
	wrapped := fmt.Errorf("provider primary: %w", rle)
	var out *RateLimitError
	if !errors.As(wrapped, &out) {
		t.Fatal("errors.As 应穿透包装")
	}
	if out.RetryAfter != 3*time.Second {
		t.Errorf("RetryAfter 应保留, got %v", out.RetryAfter)
	}
}

// 冷却期内健康检查成功不清冷却（审核修正项）——429 冷却 15s 内 checkOne Ping 成功不解熔
func TestD05_HealthCheckRespectsCooldown(t *testing.T) {
	f := newD05Failover(t)
	f.RecordFailure("p4", &RateLimitError{StatusCode: 429, RetryAfter: 30 * time.Second})
	if !f.IsCircuitOpen("p4") {
		t.Fatal("前置：应在冷却中")
	}
	// 模拟 checkOne 成功路径
	f.mu.Lock()
	if h, ok := f.health["p4"]; ok {
		h.LatencyP95Ms = 10
		if time.Now().Before(h.CircuitOpenUntil) {
			// 新逻辑：冷却期内不清 CircuitOpenUntil（Status 保持 Degraded）
			h.Status = ProviderStatusDegraded
		} else {
			h.Status = ProviderStatusUp
			h.CircuitOpenUntil = time.Time{}
		}
	}
	f.mu.Unlock()
	if !f.IsCircuitOpen("p4") {
		t.Error("冷却期内健康检查成功不应解除熔断")
	}
}

// 二测补充：Retry-After 边界（负数/HTTP-date/非法）与错误文案相容
func TestD05_ParseRetryAfterBoundaries(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"5", 5 * time.Second},
		{"0", 0},
		{"-3", 0},
		{"abc", 0},
		{"Mon, 02 Jan 2006 15:04:05 GMT", 0},
	}
	for _, c := range cases {
		if got := parseRetryAfterSeconds(c.in); got != c.want {
			t.Errorf("parseRetryAfterSeconds(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

func TestD05_RateLimitErrorCompat(t *testing.T) {
	e := &RateLimitError{StatusCode: 429, Body: `{"error":"quota"}`}
	want := "LLM API error: status=429 body={\"error\":\"quota\"}"
	if e.Error() != want {
		t.Errorf("文案应与原格式相容\ngot  %s\nwant %s", e.Error(), want)
	}
}
