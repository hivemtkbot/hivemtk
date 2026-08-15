package tooluse

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)



func makeHandler(name string, callCount *int32, succeed bool, errMsg string) ToolHandler {
	return func(ctx context.Context, args map[string]any) (ToolResult, error) {
		atomic.AddInt32(callCount, 1)
		if !succeed {
			return ErrorResult(name, errors.New(errMsg)), errors.New(errMsg)
		}
		return SuccessResult(name, map[string]any{"echo": args}), nil
	}
}

func makePanicHandler(name string, callCount *int32) ToolHandler {
	return func(ctx context.Context, args map[string]any) (ToolResult, error) {
		atomic.AddInt32(callCount, 1)
		panic("intentional panic")
	}
}

func makeSlowHandler(name string, callCount *int32, duration time.Duration) ToolHandler {
	return func(ctx context.Context, args map[string]any) (ToolResult, error) {
		atomic.AddInt32(callCount, 1)
		select {
		case <-time.After(duration):
			return SuccessResult(name, "done"), nil
		case <-ctx.Done():
			return ErrorResult(name, ctx.Err()), ctx.Err()
		}
	}
}

// makeCtxWithToolName 构造带工具名的 ctx
func makeCtxWithToolName(name string) context.Context {
	ctx := context.Background()
	return WithToolName(ctx, name)
}


// allowAllChecker 始终放行
type allowAllChecker struct{}

func (allowAllChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	return nil
}

// denyAllChecker 始终拒绝
type denyAllChecker struct{}

func (denyAllChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	return errors.New("access denied for " + toolName)
}

// permissionListChecker 按工具名白名单放行
type permissionListChecker struct {
	allowed map[string]bool
}

func (c *permissionListChecker) Check(ctx context.Context, toolName string, tc *ToolContext) error {
	if !c.allowed[toolName] {
		return fmt.Errorf("tool %s not in permission list", toolName)
	}
	return nil
}

func TestPermissionDecorator_Allow(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := PermissionDecorator(allowAllChecker{})(h)
	ctx := makeCtxWithToolName("test.tool")
	r, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("期望放行，实际错误：%v", err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true，实际 false")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}

func TestPermissionDecorator_Deny(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := PermissionDecorator(denyAllChecker{})(h)
	ctx := makeCtxWithToolName("test.tool")
	r, err := dec(ctx, nil)
	if err == nil {
		t.Fatalf("期望拒绝（err 非 nil），实际 nil")
	}
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("期望 ErrPermissionDenied，实际 %v", err)
	}
	if r.Success {
		t.Fatalf("期望 Success=false")
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("权限拒绝时不应调用 handler，实际调用了 %d 次", calls)
	}
}

func TestPermissionDecorator_NilChecker(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := PermissionDecorator(nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("nil checker 应放行，实际错误：%v", err)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}

func TestPermissionDecorator_Whitelist(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := PermissionDecorator(&permissionListChecker{
		allowed: map[string]bool{"test.allowed": true},
	})(h)
	ctx1 := makeCtxWithToolName("test.allowed")
	if _, err := dec(ctx1, nil); err != nil {
		t.Fatalf("白名单内工具应放行：%v", err)
	}
	ctx2 := makeCtxWithToolName("test.denied")
	_, err := dec(ctx2, nil)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("不在白名单应返回 ErrPermissionDenied，实际 %v", err)
	}
}


func TestRateLimitDecorator_Allow(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := RateLimitDecorator(NoOpRateLimiter{})(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("NoOp 限流器应放行，实际错误：%v", err)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}

func TestRateLimitDecorator_NilLimiter(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := RateLimitDecorator(nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("nil 限流器应放行，实际错误：%v", err)
	}
}

// denyAllLimiter 始终拒绝
type denyAllLimiter struct{}

func (denyAllLimiter) Acquire(ctx context.Context, key string) error {
	return errors.New("rate limit exceeded")
}

func TestRateLimitDecorator_Deny(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := RateLimitDecorator(denyAllLimiter{})(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("期望 ErrRateLimited，实际 %v", err)
	}
	if calls != 0 {
		t.Fatalf("被限流不应调用 handler")
	}
}

func TestTokenBucketLimiter_BasicAllow(t *testing.T) {
	limiter := NewTokenBucketLimiter(100, 5) 
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := limiter.Acquire(ctx, "key1"); err != nil {
			t.Fatalf("第 %d 次应放行，实际错误：%v", i+1, err)
		}
	}
}

func TestTokenBucketLimiter_DenyWhenExhausted(t *testing.T) {
	limiter := NewTokenBucketLimiter(0.01, 2) 
	ctx := context.Background()
	_ = limiter.Acquire(ctx, "key1")
	_ = limiter.Acquire(ctx, "key1")
	err := limiter.Acquire(ctx, "key1")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("令牌耗尽应返回 ErrRateLimited，实际 %v", err)
	}
}

func TestTokenBucketLimiter_Refill(t *testing.T) {
	limiter := NewTokenBucketLimiter(1000, 1) 
	ctx := context.Background()
	_ = limiter.Acquire(ctx, "key1") 
	time.Sleep(20 * time.Millisecond)
	if err := limiter.Acquire(ctx, "key1"); err != nil {
		t.Fatalf("等待后应放行，实际错误：%v", err)
	}
}

func TestTokenBucketLimiter_PerKeyIsolation(t *testing.T) {
	limiter := NewTokenBucketLimiter(0.01, 1)
	ctx := context.Background()
	_ = limiter.Acquire(ctx, "key1")
	if err := limiter.Acquire(ctx, "key2"); err != nil {
		t.Fatalf("不同 key 应独立限流，实际错误：%v", err)
	}
}

func TestRateLimitDecorator_KeyIncludesCallerID(t *testing.T) {
	limiter := NewTokenBucketLimiter(0.01, 1)
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := RateLimitDecorator(limiter)(h)

	ctx1 := WithToolContext(
		WithToolName(context.Background(), "test.tool"),
		&ToolContext{CallerID: "agent-001"},
	)
	if _, err := dec(ctx1, nil); err != nil {
		t.Fatalf("首次应放行：%v", err)
	}
	_, err := dec(ctx1, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("同 caller 再次应被限流，实际 %v", err)
	}
	ctx2 := WithToolContext(
		WithToolName(context.Background(), "test.tool"),
		&ToolContext{CallerID: "agent-002"},
	)
	if _, err := dec(ctx2, nil); err != nil {
		t.Fatalf("不同 caller 应独立放行：%v", err)
	}
}


// countFailThenSucceedN 前 N-1 次失败，第 N 次成功
func countFailThenSucceedN(name string, counter *int32, failUntil int32) ToolHandler {
	return func(ctx context.Context, args map[string]any) (ToolResult, error) {
		cur := atomic.AddInt32(counter, 1)
		if cur < failUntil {
			return ErrorResult(name, fmt.Errorf("fail #%d", cur)), fmt.Errorf("fail #%d", cur)
		}
		return SuccessResult(name, "ok"), nil
	}
}

// zeroBackoffPolicy 立即重试（用于测试）
type zeroBackoffPolicy struct {
	maxAttempts int
}

func (p *zeroBackoffPolicy) MaxAttempts() int { return p.maxAttempts }
func (p *zeroBackoffPolicy) NextBackoff(attempt int, lastErr error) (time.Duration, bool) {
	if attempt >= p.maxAttempts {
		return 0, false
	}
	return 0, true
}

func TestRetryDecorator_FirstSuccess(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := RetryDecorator(&zeroBackoffPolicy{maxAttempts: 3})(h)
	ctx := makeCtxWithToolName("test.tool")
	r, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("首次成功不应有错误：%v", err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true")
	}
	if r.Timing.RetryCount != 0 {
		t.Fatalf("首次成功 RetryCount 应为 0，实际 %d", r.Timing.RetryCount)
	}
	if calls != 1 {
		t.Fatalf("应只调用 1 次，实际 %d", calls)
	}
}

func TestRetryDecorator_SuccessAfterRetries(t *testing.T) {
	var calls int32
	h := countFailThenSucceedN("test.tool", &calls, 3) 
	dec := RetryDecorator(&zeroBackoffPolicy{maxAttempts: 5})(h)
	ctx := makeCtxWithToolName("test.tool")
	r, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("重试后应成功，实际错误：%v", err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true")
	}
	if r.Timing.RetryCount != 2 {
		t.Fatalf("期望 RetryCount=2，实际 %d", r.Timing.RetryCount)
	}
	if calls != 3 {
		t.Fatalf("期望调用 3 次，实际 %d", calls)
	}
}

func TestRetryDecorator_Exhausted(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, false, "always fail")
	dec := RetryDecorator(&zeroBackoffPolicy{maxAttempts: 3})(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err == nil {
		t.Fatalf("重试耗尽应返回错误")
	}
	if calls != 3 {
		t.Fatalf("期望调用 3 次，实际 %d", calls)
	}
}

func TestRetryDecorator_PanicRecovery(t *testing.T) {
	var calls int32
	h := makePanicHandler("test.tool", &calls)
	dec := RetryDecorator(&zeroBackoffPolicy{maxAttempts: 2})(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err == nil {
		t.Fatalf("panic 应转为错误")
	}
	if !errors.Is(err, ErrToolPanic) {
		t.Fatalf("期望 ErrToolPanic，实际 %v", err)
	}
	if calls != 2 {
		t.Fatalf("panic 也应触发重试，期望调用 2 次，实际 %d", calls)
	}
}

func TestRetryDecorator_NilPolicy(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, false, "fail")
	dec := RetryDecorator(nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err == nil {
		t.Fatalf("应返回失败错误")
	}
	if calls != 1 {
		t.Fatalf("nil policy 应只调用 1 次，实际 %d", calls)
	}
}

func TestRetryDecorator_ContextCancel(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, false, "fail")
	dec := RetryDecorator(&zeroBackoffPolicy{maxAttempts: 100})(h)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() 
	ctx = WithToolName(ctx, "test.tool")
	_, err := dec(ctx, nil)
	if err == nil {
		t.Fatalf("context 已取消应返回错误")
	}
	if calls != 0 {
		t.Fatalf("context 已取消应不调用 handler，实际 %d", calls)
	}
}

func TestExponentialBackoffPolicy_BasicDelay(t *testing.T) {
	p := &ExponentialBackoffPolicy{
		MaxAttemptsValue: 5,
		BaseDelay:        100 * time.Millisecond,
		MaxDelay:         10 * time.Second,
		Jitter:           false,
	}
	d1, ok := p.NextBackoff(1, nil)
	if !ok || d1 != 100*time.Millisecond {
		t.Fatalf("attempt 1 期望 100ms，实际 %v (ok=%v)", d1, ok)
	}
	d2, _ := p.NextBackoff(2, nil)
	if d2 != 200*time.Millisecond {
		t.Fatalf("attempt 2 期望 200ms，实际 %v", d2)
	}
	d3, _ := p.NextBackoff(3, nil)
	if d3 != 400*time.Millisecond {
		t.Fatalf("attempt 3 期望 400ms，实际 %v", d3)
	}
}

func TestExponentialBackoffPolicy_CapAtMax(t *testing.T) {
	p := &ExponentialBackoffPolicy{
		MaxAttemptsValue: 10,
		BaseDelay:        1 * time.Second,
		MaxDelay:         5 * time.Second,
		Jitter:           false,
	}
	d, _ := p.NextBackoff(10, nil)
	if d != 5*time.Second {
		t.Fatalf("应被 cap 到 5s，实际 %v", d)
	}
}

func TestExponentialBackoffPolicy_OutOfRange(t *testing.T) {
	p := &ExponentialBackoffPolicy{
		MaxAttemptsValue: 3,
		BaseDelay:        100 * time.Millisecond,
		MaxDelay:         1 * time.Second,
	}
	_, ok := p.NextBackoff(0, nil)
	if ok {
		t.Fatalf("attempt 0 应返回 ok=false")
	}
	_, ok = p.NextBackoff(4, nil)
	if ok {
		t.Fatalf("attempt 4 应返回 ok=false")
	}
}


func TestTimeoutDecorator_Normal(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := TimeoutDecorator(1 * time.Second)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("应正常完成，实际错误：%v", err)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}

func TestTimeoutDecorator_TimedOut(t *testing.T) {
	var calls int32
	h := makeSlowHandler("test.tool", &calls, 500*time.Millisecond)
	dec := TimeoutDecorator(50 * time.Millisecond)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if !errors.Is(err, ErrToolTimeout) {
		t.Fatalf("期望 ErrToolTimeout，实际 %v", err)
	}
}

func TestTimeoutDecorator_ZeroDuration(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := TimeoutDecorator(0)(h) 
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("0 时长应不超时，实际错误：%v", err)
	}
}

func TestTimeoutDecorator_PanicInGoroutine(t *testing.T) {
	var calls int32
	h := makePanicHandler("test.tool", &calls)
	dec := TimeoutDecorator(1 * time.Second)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if !errors.Is(err, ErrToolPanic) {
		t.Fatalf("期望 ErrToolPanic，实际 %v", err)
	}
}


func TestAuditDecorator_SuccessLogged(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	tracker := NewMemoryCostTracker()
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := AuditDecorator(logger, tracker)(h)
	ctx := WithToolContext(
		WithToolName(context.Background(), "test.tool"),
		&ToolContext{
			CallerID:   "agent-001",
			AgentID:    "agent-001",
			CustomerID: "cust-100",
			SessionID:  "sess-200",
		},
	)
	_, err := dec(ctx, map[string]any{"foo": "bar"})
	if err != nil {
		t.Fatalf("应成功：%v", err)
	}
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志，实际 %d", len(entries))
	}
	e := entries[0]
	if e.ToolName != "test.tool" {
		t.Fatalf("ToolName 期望 test.tool，实际 %s", e.ToolName)
	}
	if e.CallerID != "agent-001" {
		t.Fatalf("CallerID 期望 agent-001，实际 %s", e.CallerID)
	}
	if !e.Success {
		t.Fatalf("应记录成功")
	}
	if e.ArgsSummary == "" {
		t.Fatalf("ArgsSummary 不应为空")
	}
	if !strings.Contains(e.ArgsSummary, "foo=bar") {
		t.Fatalf("ArgsSummary 应包含参数：实际 %s", e.ArgsSummary)
	}
	stats := tracker.Stats()
	if len(stats) != 1 {
		t.Fatalf("期望 1 条计费统计，实际 %d", len(stats))
	}
	if stats[0].TotalCalls != 1 || stats[0].SuccessCalls != 1 {
		t.Fatalf("计费统计错误：%+v", stats[0])
	}
}

func TestAuditDecorator_FailureLogged(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	tracker := NewMemoryCostTracker()
	var calls int32
	h := makeHandler("test.tool", &calls, false, "boom")
	dec := AuditDecorator(logger, tracker)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, _ = dec(ctx, nil)
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志，实际 %d", len(entries))
	}
	if entries[0].Success {
		t.Fatalf("应记录失败")
	}
	if entries[0].Error == "" {
		t.Fatalf("Error 字段不应为空")
	}
	stats := tracker.Stats()
	if stats[0].FailedCalls != 1 {
		t.Fatalf("期望 FailedCalls=1，实际 %d", stats[0].FailedCalls)
	}
}

func TestAuditDecorator_SensitiveArgsMasked(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	h := makeHandler("test.tool", new(int32), true, "")
	dec := AuditDecorator(logger, nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	args := map[string]any{
		"phone":     "13800000000",
		"token":     "secret-xyz",
		"api_key":   "key-123",
		"normal":    "visible",
		"password":  "p@ssw0rd",
		"id_card":   "110101199001011234",
		"bank_card": "6228480000000000000",
	}
	_, _ = dec(ctx, args)
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志")
	}
	summary := entries[0].ArgsSummary
	for _, k := range []string{"phone", "token", "api_key", "password", "id_card", "bank_card"} {
		if strings.Contains(summary, k+"=") && !strings.Contains(summary, k+"=***") {
			t.Fatalf("敏感字段 %s 应被脱敏，实际 summary=%s", k, summary)
		}
	}
	if !strings.Contains(summary, "normal=visible") {
		t.Fatalf("普通字段应保留原值，实际 summary=%s", summary)
	}
}

func TestAuditDecorator_NilLogger(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	dec := AuditDecorator(nil, nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	_, err := dec(ctx, nil)
	if err != nil {
		t.Fatalf("nil logger 不应影响执行，实际错误：%v", err)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}

func TestAuditDecorator_TruncatesLongArgs(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	h := makeHandler("test.tool", new(int32), true, "")
	dec := AuditDecorator(logger, nil)(h)
	ctx := makeCtxWithToolName("test.tool")
	longStr := strings.Repeat("x", 500)
	args := map[string]any{"long_field": longStr}
	_, _ = dec(ctx, args)
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志")
	}
	if len(entries[0].ArgsSummary) > 250 {
		t.Fatalf("长参数应被截断，实际长度 %d", len(entries[0].ArgsSummary))
	}
}


func TestChainDecorators_Order(t *testing.T) {
	// 用一个共享 slice 记录调用顺序
	var order []string
	var mu sync.Mutex
	addLog := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	dec1 := func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			addLog("dec1-pre")
			r, e := next(ctx, args)
			addLog("dec1-post")
			return r, e
		}
	}
	dec2 := func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			addLog("dec2-pre")
			r, e := next(ctx, args)
			addLog("dec2-post")
			return r, e
		}
	}
	dec3 := func(next ToolHandler) ToolHandler {
		return func(ctx context.Context, args map[string]any) (ToolResult, error) {
			addLog("dec3-pre")
			r, e := next(ctx, args)
			addLog("dec3-post")
			return r, e
		}
	}
	handler := func(ctx context.Context, args map[string]any) (ToolResult, error) {
		addLog("handler")
		return SuccessResult("test", nil), nil
	}
	chain := ChainDecorators(handler, dec1, dec2, dec3)
	_, _ = chain(context.Background(), nil)

	expected := []string{"dec1-pre", "dec2-pre", "dec3-pre", "handler", "dec3-post", "dec2-post", "dec1-post"}
	if len(order) != len(expected) {
		t.Fatalf("顺序长度错误：期望 %d，实际 %d (%v)", len(expected), len(order), order)
	}
	for i, s := range expected {
		if order[i] != s {
			t.Fatalf("第 %d 步顺序错误：期望 %s，实际 %s (完整: %v)", i, s, order[i], order)
		}
	}
}

func TestChainDecorators_SkipNilDecorators(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	chain := ChainDecorators(h, nil, nil, nil)
	ctx := makeCtxWithToolName("test.tool")
	_, err := chain(ctx, nil)
	if err != nil {
		t.Fatalf("nil 装饰器应被跳过，实际错误：%v", err)
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
}


func TestBuildDefaultChain_PermissionDenyStopsChain(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	chain := BuildDefaultChain(h,
		denyAllChecker{},                   
		NoOpRateLimiter{},                  
		&zeroBackoffPolicy{maxAttempts: 3}, 
		1*time.Second,                      
		NewMemoryAuditLogger(100),          
		NewMemoryCostTracker(),             
	)
	ctx := makeCtxWithToolName("test.tool")
	_, err := chain(ctx, nil)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("期望 ErrPermissionDenied，实际 %v", err)
	}
	if calls != 0 {
		t.Fatalf("权限拒绝时不应调用 handler，实际 %d", calls)
	}
}

func TestBuildDefaultChain_RateLimitedStopsChain(t *testing.T) {
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	chain := BuildDefaultChain(h,
		allowAllChecker{},                  
		denyAllLimiter{},                   
		&zeroBackoffPolicy{maxAttempts: 3}, 
		1*time.Second,
		NewMemoryAuditLogger(100),
		NewMemoryCostTracker(),
	)
	ctx := makeCtxWithToolName("test.tool")
	_, err := chain(ctx, nil)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("期望 ErrRateLimited，实际 %v", err)
	}
	if calls != 0 {
		t.Fatalf("限流拒绝时不应调用 handler，实际 %d", calls)
	}
}

func TestBuildDefaultChain_RetryOnFailure(t *testing.T) {
	var calls int32
	h := countFailThenSucceedN("test.tool", &calls, 3)
	chain := BuildDefaultChain(h,
		allowAllChecker{},
		NoOpRateLimiter{},
		&zeroBackoffPolicy{maxAttempts: 5}, 
		1*time.Second,
		NewMemoryAuditLogger(100),
		NewMemoryCostTracker(),
	)
	ctx := makeCtxWithToolName("test.tool")
	r, err := chain(ctx, nil)
	if err != nil {
		t.Fatalf("重试后应成功，实际错误：%v", err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true")
	}
	if calls != 3 {
		t.Fatalf("期望调用 3 次，实际 %d", calls)
	}
}

func TestBuildDefaultChain_FullSuccess(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	tracker := NewMemoryCostTracker()
	var calls int32
	h := makeHandler("test.tool", &calls, true, "")
	chain := BuildDefaultChain(h,
		allowAllChecker{},
		NoOpRateLimiter{},
		&zeroBackoffPolicy{maxAttempts: 1}, 
		1*time.Second,
		logger,
		tracker,
	)
	ctx := WithToolContext(
		WithToolName(context.Background(), "test.tool"),
		&ToolContext{CallerID: "agent-001", CustomerID: "cust-100"},
	)
	r, err := chain(ctx, map[string]any{"action": "search", "q": "hello"})
	if err != nil {
		t.Fatalf("应成功：%v", err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true")
	}
	if calls != 1 {
		t.Fatalf("期望调用 1 次，实际 %d", calls)
	}
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志，实际 %d", len(entries))
	}
	if entries[0].CallerID != "agent-001" {
		t.Fatalf("审计日志应包含 CallerID")
	}
	if entries[0].CustomerID != "cust-100" {
		t.Fatalf("审计日志应包含 CustomerID")
	}
	stats := tracker.Stats()
	if len(stats) != 1 || stats[0].SuccessCalls != 1 {
		t.Fatalf("计费统计错误：%+v", stats)
	}
}


func TestToolContext_PassThroughChain(t *testing.T) {
	logger := NewMemoryAuditLogger(100)
	var capturedCtx *ToolContext

	h := func(ctx context.Context, args map[string]any) (ToolResult, error) {
		capturedCtx = GetToolContext(ctx)
		return SuccessResult("test.tool", "ok"), nil
	}
	chain := BuildDefaultChain(h,
		allowAllChecker{},
		NoOpRateLimiter{},
		nil, 
		1*time.Second,
		logger,
		nil,
	)
	tc := &ToolContext{
		CallerID:   "agent-xyz",
		AgentID:    "agent-xyz",
		CustomerID: "cust-999",
		SessionID:  "sess-111",
		Source:     "sop",
	}
	ctx := WithToolContext(WithToolName(context.Background(), "test.tool"), tc)
	_, _ = chain(ctx, nil)
	if capturedCtx == nil {
		t.Fatalf("ToolContext 应通过链路传递到 handler")
	}
	if capturedCtx.CallerID != "agent-xyz" {
		t.Fatalf("CallerID 期望 agent-xyz，实际 %s", capturedCtx.CallerID)
	}
	if capturedCtx.CustomerID != "cust-999" {
		t.Fatalf("CustomerID 期望 cust-999，实际 %s", capturedCtx.CustomerID)
	}
	if capturedCtx.Source != "sop" {
		t.Fatalf("Source 期望 sop，实际 %s", capturedCtx.Source)
	}
}

func TestGetToolContext_NilWhenNotSet(t *testing.T) {
	ctx := context.Background()
	if GetToolContext(ctx) != nil {
		t.Fatalf("未注入时应返回 nil")
	}
}


func TestMemoryAuditLogger_RollOver(t *testing.T) {
	logger := NewMemoryAuditLogger(3)
	logger.Log(context.Background(), AuditEntry{ToolName: "a"})
	logger.Log(context.Background(), AuditEntry{ToolName: "b"})
	logger.Log(context.Background(), AuditEntry{ToolName: "c"})
	logger.Log(context.Background(), AuditEntry{ToolName: "d"})
	entries := logger.Entries()
	if len(entries) != 3 {
		t.Fatalf("maxSize=3 应只保留 3 条，实际 %d", len(entries))
	}
	if entries[0].ToolName != "b" {
		t.Fatalf("应滚动覆盖最旧的 a，实际 entries[0]=%s", entries[0].ToolName)
	}
	if entries[2].ToolName != "d" {
		t.Fatalf("最新应为 d，实际 %s", entries[2].ToolName)
	}
}

func TestMemoryAuditLogger_ConcurrentSafe(t *testing.T) {
	logger := NewMemoryAuditLogger(10000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger.Log(context.Background(), AuditEntry{
				ToolName: fmt.Sprintf("tool-%d", i%5),
			})
		}(i)
	}
	wg.Wait()
	if logger.Count() != 100 {
		t.Fatalf("并发写后期望 100 条，实际 %d", logger.Count())
	}
}

func TestMemoryCostTracker_StatsComputation(t *testing.T) {
	tracker := NewMemoryCostTracker()
	ctx := context.Background()
	_ = tracker.Record(ctx, "tool.a", true, 100*time.Millisecond)
	_ = tracker.Record(ctx, "tool.a", true, 200*time.Millisecond)
	_ = tracker.Record(ctx, "tool.a", false, 50*time.Millisecond)
	_ = tracker.Record(ctx, "tool.b", true, 10*time.Millisecond)

	stats := tracker.Stats()
	if len(stats) != 2 {
		t.Fatalf("期望 2 个工具统计，实际 %d", len(stats))
	}
	// 找到 tool.a 的统计
	var aStats *CostStats
	for i := range stats {
		if stats[i].ToolName == "tool.a" {
			aStats = &stats[i]
			break
		}
	}
	if aStats == nil {
		t.Fatalf("未找到 tool.a 的统计")
	}
	if aStats.TotalCalls != 3 {
		t.Fatalf("TotalCalls 期望 3，实际 %d", aStats.TotalCalls)
	}
	if aStats.SuccessCalls != 2 {
		t.Fatalf("SuccessCalls 期望 2，实际 %d", aStats.SuccessCalls)
	}
	if aStats.FailedCalls != 1 {
		t.Fatalf("FailedCalls 期望 1，实际 %d", aStats.FailedCalls)
	}
	if aStats.TotalDurationMs != 350 {
		t.Fatalf("TotalDurationMs 期望 350，实际 %d", aStats.TotalDurationMs)
	}
	expected := 2.0 / 3.0
	if aStats.SuccessRate < expected-0.001 || aStats.SuccessRate > expected+0.001 {
		t.Fatalf("SuccessRate 期望 %f，实际 %f", expected, aStats.SuccessRate)
	}
	if aStats.AvgDurationMs < 116.0 || aStats.AvgDurationMs > 117.0 {
		t.Fatalf("AvgDurationMs 期望 ~116.67，实际 %f", aStats.AvgDurationMs)
	}
}


func TestSummarizeArgs_Empty(t *testing.T) {
	if summarizeArgs(nil) != "" {
		t.Fatalf("空参数应返回空字符串")
	}
}

func TestSummarizeArgs_BasicSerialization(t *testing.T) {
	args := map[string]any{
		"name":  "alice",
		"age":   30,
		"admin": true,
	}
	s := summarizeArgs(args)
	for k, v := range args {
		expected := fmt.Sprintf("%s=%v", k, v)
		if !strings.Contains(s, expected) {
			t.Fatalf("summary 应包含 %s，实际 %s", expected, s)
		}
	}
}

func TestSummarizeArgs_LongValueTruncated(t *testing.T) {
	args := map[string]any{
		"long": strings.Repeat("a", 100),
	}
	s := summarizeArgs(args)
	if !strings.Contains(s, "...") {
		t.Fatalf("长值应被截断加省略号，实际 %s", s)
	}
	idx := strings.Index(s, "long=")
	if idx >= 0 {
		val := s[idx+5:]
		if comma := strings.Index(val, ","); comma >= 0 {
			val = val[:comma]
		}
		if len(val) > 53 {
			t.Fatalf("截断后单值应 ≤ 53 字符，实际 %d (%s)", len(val), val)
		}
	}
}


func TestNoOpImplementations(t *testing.T) {
	ctx := context.Background()
	tc := &ToolContext{CallerID: "x"}

	if err := (NoOpPermissionChecker{}).Check(ctx, "any.tool", tc); err != nil {
		t.Fatalf("NoOpPermissionChecker 应返回 nil")
	}
	if err := (NoOpRateLimiter{}).Acquire(ctx, "any.key"); err != nil {
		t.Fatalf("NoOpRateLimiter 应返回 nil")
	}
	(NoOpAuditLogger{}).Log(ctx, AuditEntry{}) 
	if err := (NoOpCostTracker{}).Record(ctx, "any.tool", true, 1*time.Second); err != nil {
		t.Fatalf("NoOpCostTracker 应返回 nil")
	}
}

