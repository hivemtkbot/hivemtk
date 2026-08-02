package tooluse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// executor_test.go 工具执行引擎测试（PRD §5.2 G3）

// ===== 辅助：构造一个 mock tool =====

// mockTool 通用 mock 工具
type mockTool struct {
	BaseTool
	executeFn func(ctx context.Context, args map[string]any) (ToolResult, error)
}

func (t *mockTool) Execute(ctx context.Context, args map[string]any) (ToolResult, error) {
	return t.executeFn(ctx, args)
}

func newMockTool(name string, category ToolCategory, fn func(ctx context.Context, args map[string]any) (ToolResult, error)) *mockTool {
	return &mockTool{
		BaseTool: BaseTool{
			NameVal:     name,
			CategoryVal: category,
			ParamsVal: ToolParameters{
				Type: "object",
				Properties: map[string]ToolParam{
					"foo": {Type: "string", Description: "测试参数"},
				},
				Required: []string{"foo"},
			},
		},
		executeFn: fn,
	}
}

// echoTool 返回入参的简单工具
func echoTool(name string) *mockTool {
	return newMockTool(name, CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		return SuccessResult(name, args), nil
	})
}

// failTool 始终失败的工具
func failTool(name string) *mockTool {
	return newMockTool(name, CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		return ErrorResult(name, errors.New("intentional failure")), errors.New("intentional failure")
	})
}

// panicTool panic 工具
func panicTool(name string) *mockTool {
	return newMockTool(name, CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		panic("intentional panic")
	})
}

// slowTool 慢工具
func slowTool(name string, delay time.Duration) *mockTool {
	return newMockTool(name, CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		select {
		case <-time.After(delay):
			return SuccessResult(name, "done"), nil
		case <-ctx.Done():
			return ErrorResult(name, ctx.Err()), ctx.Err()
		}
	})
}

// countTool 记录调用次数的工具
func countTool(name string, counter *int32) *mockTool {
	return newMockTool(name, CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		atomic.AddInt32(counter, 1)
		return SuccessResult(name, atomic.LoadInt32(counter)), nil
	})
}

// ===== 1. 基本执行测试 =====

func TestToolExecutor_ExecuteSuccess(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})

	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
		Args:     map[string]any{"foo": "bar"},
	})
	if r.Err != nil {
		t.Fatalf("应成功，实际错误：%v", r.Err)
	}
	if !r.Success {
		t.Fatalf("期望 Success=true")
	}
	if r.ToolName != "test.echo" {
		t.Fatalf("ToolName 期望 test.echo，实际 %s", r.ToolName)
	}
	data, ok := r.Data.(map[string]any)
	if !ok {
		t.Fatalf("Data 应为 map")
	}
	if data["foo"] != "bar" {
		t.Fatalf("foo 期望 bar，实际 %v", data["foo"])
	}
}

func TestToolExecutor_ToolNotFound(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "nonexistent.tool",
		Args:     nil,
	})
	if r.Err == nil {
		t.Fatalf("应返回错误")
	}
	if !errors.Is(r.Err, ErrToolNotFound) {
		t.Fatalf("期望 ErrToolNotFound，实际 %v", r.Err)
	}
}

func TestToolExecutor_EmptyToolName(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "",
	})
	if r.Err == nil {
		t.Fatalf("空 tool_name 应返回错误")
	}
}

func TestToolExecutor_ExecuteByName(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	result, err := executor.ExecuteByName(context.Background(), "test.echo", map[string]any{"foo": "baz"})
	if err != nil {
		t.Fatalf("应成功：%v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}
}

// ===== 2. 装饰器链挂载测试 =====

func TestToolExecutor_DecoratorChainApplied(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(failTool("test.fail"))
	logger := NewMemoryAuditLogger(100)
	tracker := NewMemoryCostTracker()
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    1 * time.Second,
		AuditLogger:       logger,
		CostTracker:       tracker,
		PermissionChecker: allowAllChecker{},
		RateLimiter:       NoOpRateLimiter{},
		// 不重试（MaxAttempts=1）
	})

	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.fail",
		Args:     nil,
	})
	if r.Err == nil {
		t.Fatalf("应失败")
	}
	// 验证审计日志被写入
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志，实际 %d", len(entries))
	}
	if entries[0].Success {
		t.Fatalf("应记录失败")
	}
	// 验证计费统计被写入
	stats := tracker.Stats()
	if len(stats) != 1 || stats[0].FailedCalls != 1 {
		t.Fatalf("计费统计错误：%+v", stats)
	}
}

func TestToolExecutor_PermissionCheckerApplied(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	var calls int32
	countT := countTool("test.count", &calls)
	registry.MustRegister(countT)

	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    1 * time.Second,
		PermissionChecker: denyAllChecker{},
	})
	// 权限拒绝，不应调用 tool
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.count",
	})
	if !errors.Is(r.Err, ErrPermissionDenied) {
		t.Fatalf("期望 ErrPermissionDenied，实际 %v", r.Err)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("权限拒绝不应调用 tool，实际 %d", calls)
	}
}

func TestToolExecutor_RateLimiterApplied(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    1 * time.Second,
		PermissionChecker: allowAllChecker{},
		RateLimiter:       denyAllLimiter{},
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
	})
	if !errors.Is(r.Err, ErrRateLimited) {
		t.Fatalf("期望 ErrRateLimited，实际 %v", r.Err)
	}
}

func TestToolExecutor_TimeoutApplied(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(slowTool("test.slow", 500*time.Millisecond))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 50 * time.Millisecond,
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.slow",
	})
	if !errors.Is(r.Err, ErrToolTimeout) {
		t.Fatalf("期望 ErrToolTimeout，实际 %v", r.Err)
	}
}

func TestToolExecutor_PanicRecovery(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(panicTool("test.panic"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.panic",
	})
	if r.Err == nil {
		t.Fatalf("panic 应转为错误")
	}
	if !errors.Is(r.Err, ErrToolPanic) {
		t.Fatalf("期望 ErrToolPanic，实际 %v", r.Err)
	}
}

// ===== 3. 装饰器 handler 缓存测试 =====

func TestToolExecutor_HandlerCached(t *testing.T) {
	registry := NewToolRegistry()
	var calls int32
	countT := countTool("test.count", &calls)
	registry.MustRegister(countT)
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	// 执行 5 次，每次都应使用缓存的 handler
	for i := 0; i < 5; i++ {
		_ = executor.Execute(context.Background(), ExecuteRequest{
			ToolName: "test.count",
		})
	}
	// 验证 handler 缓存中有 1 条
	executor.mu.RLock()
	cacheLen := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen != 1 {
		t.Fatalf("期望 1 条 handler 缓存，实际 %d", cacheLen)
	}
	if atomic.LoadInt32(&calls) != 5 {
		t.Fatalf("期望 tool 被调用 5 次，实际 %d", calls)
	}
}

// ===== 4. 工具级别 override 测试 =====

func TestToolExecutor_SetOverrideDisabled(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	// 设置 disabled
	executor.SetOverride(ToolOverride{
		ToolName: "test.echo",
		Disabled: true,
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
	})
	if r.Err == nil {
		t.Fatalf("disabled 工具应返回错误")
	}
}

func TestToolExecutor_SetOverrideTimeout(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(slowTool("test.slow", 500*time.Millisecond))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second, // 默认 1s 足够慢工具完成
	})
	// 不 override：1s 超时，500ms 完成，应成功
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.slow",
	})
	if r.Err != nil {
		t.Fatalf("应成功：%v", r.Err)
	}

	// override 为 50ms 超时
	executor.SetOverride(ToolOverride{
		ToolName: "test.slow",
		Timeout:  50 * time.Millisecond,
	})
	r2 := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.slow",
	})
	if !errors.Is(r2.Err, ErrToolTimeout) {
		t.Fatalf("override 后应超时，实际 %v", r2.Err)
	}
}

func TestToolExecutor_SetOverrideClearsCache(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	// 首次执行，填充缓存
	_ = executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
	})
	executor.mu.RLock()
	cacheLen1 := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen1 != 1 {
		t.Fatalf("期望 1 条缓存，实际 %d", cacheLen1)
	}
	// 设置 override 应清除该缓存
	executor.SetOverride(ToolOverride{
		ToolName: "test.echo",
		Timeout:  5 * time.Second,
	})
	executor.mu.RLock()
	cacheLen2 := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen2 != 0 {
		t.Fatalf("override 后应清空缓存，实际 %d", cacheLen2)
	}
}

func TestToolExecutor_ClearOverride(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	executor.SetOverride(ToolOverride{
		ToolName: "test.echo",
		Disabled: true,
	})
	// disabled 状态应失败
	r1 := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
	})
	if r1.Err == nil {
		t.Fatalf("disabled 状态应失败")
	}
	// 清除 override
	executor.ClearOverride("test.echo")
	r2 := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
	})
	if r2.Err != nil {
		t.Fatalf("清除 override 后应成功，实际 %v", r2.Err)
	}
}

func TestToolExecutor_GetOverride(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	// 未设置时返回 false
	_, ok := executor.GetOverride("any.tool")
	if ok {
		t.Fatalf("未设置时应返回 false")
	}
	executor.SetOverride(ToolOverride{
		ToolName: "test.echo",
		Timeout:  5 * time.Second,
	})
	o, ok := executor.GetOverride("test.echo")
	if !ok {
		t.Fatalf("应返回 true")
	}
	if o.Timeout != 5*time.Second {
		t.Fatalf("Timeout 期望 5s，实际 %v", o.Timeout)
	}
}

// ===== 5. 批量执行测试 =====

func TestToolExecutor_BatchExecuteSequential(t *testing.T) {
	registry := NewToolRegistry()
	var calls int32
	registry.MustRegister(countTool("test.a", &calls))
	registry.MustRegister(countTool("test.b", &calls))
	registry.MustRegister(countTool("test.c", &calls))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	resp := executor.BatchExecute(context.Background(), BatchExecuteRequest{
		Requests: []ExecuteRequest{
			{ToolName: "test.a"},
			{ToolName: "test.b"},
			{ToolName: "test.c"},
		},
		Parallel:    false,
		StopOnError: false,
	})
	if resp.SuccessCount != 3 {
		t.Fatalf("期望 3 个成功，实际 %d", resp.SuccessCount)
	}
	if resp.FailedCount != 0 {
		t.Fatalf("期望 0 个失败，实际 %d", resp.FailedCount)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("期望 3 个结果，实际 %d", len(resp.Results))
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("期望 3 次调用，实际 %d", calls)
	}
}

func TestToolExecutor_BatchExecuteParallel(t *testing.T) {
	registry := NewToolRegistry()
	var calls int32
	registry.MustRegister(countTool("test.a", &calls))
	registry.MustRegister(countTool("test.b", &calls))
	registry.MustRegister(countTool("test.c", &calls))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	resp := executor.BatchExecute(context.Background(), BatchExecuteRequest{
		Requests: []ExecuteRequest{
			{ToolName: "test.a"},
			{ToolName: "test.b"},
			{ToolName: "test.c"},
		},
		Parallel: true,
	})
	if resp.SuccessCount != 3 {
		t.Fatalf("期望 3 个成功，实际 %d", resp.SuccessCount)
	}
	if atomic.LoadInt32(&calls) != 3 {
		t.Fatalf("期望 3 次调用，实际 %d", calls)
	}
}

func TestToolExecutor_BatchExecuteStopOnError(t *testing.T) {
	registry := NewToolRegistry()
	var calls int32
	registry.MustRegister(countTool("test.a", &calls))
	registry.MustRegister(failTool("test.fail"))
	registry.MustRegister(countTool("test.b", &calls))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	resp := executor.BatchExecute(context.Background(), BatchExecuteRequest{
		Requests: []ExecuteRequest{
			{ToolName: "test.a"},
			{ToolName: "test.fail"},
			{ToolName: "test.b"},
		},
		Parallel:    false,
		StopOnError: true,
	})
	// test.a 成功，test.fail 失败，test.b 被跳过
	if resp.SuccessCount != 1 {
		t.Fatalf("期望 1 个成功，实际 %d", resp.SuccessCount)
	}
	if resp.FailedCount != 2 { // test.fail + test.b 跳过算作失败
		t.Fatalf("期望 2 个失败，实际 %d", resp.FailedCount)
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("期望 1 次成功调用（test.b 被跳过），实际 %d", calls)
	}
}

func TestToolExecutor_BatchExecuteMaxConcurrency(t *testing.T) {
	registry := NewToolRegistry()
	// 用 3 个慢工具测试并发限制
	registry.MustRegister(slowTool("test.slow1", 100*time.Millisecond))
	registry.MustRegister(slowTool("test.slow2", 100*time.Millisecond))
	registry.MustRegister(slowTool("test.slow3", 100*time.Millisecond))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	start := time.Now()
	resp := executor.BatchExecute(context.Background(), BatchExecuteRequest{
		Requests: []ExecuteRequest{
			{ToolName: "test.slow1"},
			{ToolName: "test.slow2"},
			{ToolName: "test.slow3"},
		},
		Parallel:       true,
		MaxConcurrency: 1, // 强制顺序（实际并发=1）
	})
	elapsed := time.Since(start)
	if resp.SuccessCount != 3 {
		t.Fatalf("期望 3 个成功，实际 %d", resp.SuccessCount)
	}
	// MaxConcurrency=1 时 3 个 100ms 任务总耗时 ≥ 300ms
	if elapsed < 250*time.Millisecond {
		t.Fatalf("MaxConcurrency=1 应顺序执行，总耗时 ≥ 300ms，实际 %v", elapsed)
	}
}

func TestToolExecutor_BatchExecuteEmpty(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	resp := executor.BatchExecute(context.Background(), BatchExecuteRequest{
		Requests: []ExecuteRequest{},
	})
	if resp.SuccessCount != 0 || resp.FailedCount != 0 {
		t.Fatalf("空请求应返回空响应")
	}
	if resp.TotalDurationMs < 0 {
		t.Fatalf("总耗时不应为负")
	}
}

// ===== 6. LLM Function Calling 集成测试 =====

func TestToolExecutor_DispatchByLLMToolCall(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("customer.search"))
	registry.MustRegister(echoTool("customer.get"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	toolCalls := []LLMToolCall{
		{
			ID: "call-1",
			Function: LLMToolFunction{
				Name:      "customer.search",
				Arguments: `{"foo":"search-value"}`,
			},
		},
		{
			ID: "call-2",
			Function: LLMToolFunction{
				Name:      "customer.get",
				Arguments: `{"foo":"get-value"}`,
			},
		},
	}
	results := executor.DispatchByLLMToolCall(context.Background(), toolCalls, nil)
	if len(results) != 2 {
		t.Fatalf("期望 2 个结果，实际 %d", len(results))
	}
	// 验证 tool_call_id 对应
	resultMap := map[string]LLMToolResult{}
	for _, r := range results {
		resultMap[r.ToolCallID] = r
	}
	if _, ok := resultMap["call-1"]; !ok {
		t.Fatalf("应包含 call-1 结果")
	}
	if _, ok := resultMap["call-2"]; !ok {
		t.Fatalf("应包含 call-2 结果")
	}
	// 验证 content 是合法 JSON
	for _, r := range results {
		var tr ToolResult
		if err := json.Unmarshal([]byte(r.Content), &tr); err != nil {
			t.Fatalf("content 应为 ToolResult JSON：%v", err)
		}
		if !r.Success {
			t.Fatalf("应成功")
		}
	}
}

func TestToolExecutor_DispatchByLLMToolCall_Empty(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	results := executor.DispatchByLLMToolCall(context.Background(), nil, nil)
	if results != nil {
		t.Fatalf("空输入应返回 nil")
	}
}

func TestToolExecutor_DispatchByLLMToolCall_InvalidJSON(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	toolCalls := []LLMToolCall{
		{
			ID: "call-1",
			Function: LLMToolFunction{
				Name:      "test.echo",
				Arguments: `not-json`,
			},
		},
	}
	results := executor.DispatchByLLMToolCall(context.Background(), toolCalls, nil)
	if len(results) != 1 {
		t.Fatalf("期望 1 个结果")
	}
	if results[0].Success {
		t.Fatalf("非法 JSON 应失败")
	}
	if !contains(results[0].Content, "arguments JSON 解析失败") {
		t.Fatalf("应包含错误信息，实际 %s", results[0].Content)
	}
}

func TestToolExecutor_DispatchByLLMToolCall_ToolNotFound(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	toolCalls := []LLMToolCall{
		{
			ID: "call-1",
			Function: LLMToolFunction{
				Name:      "nonexistent.tool",
				Arguments: `{}`,
			},
		},
	}
	results := executor.DispatchByLLMToolCall(context.Background(), toolCalls, nil)
	if results[0].Success {
		t.Fatalf("不存在的工具应失败")
	}
}

func TestToolExecutor_DispatchByLLMToolCall_WithToolCtx(t *testing.T) {
	registry := NewToolRegistry()
	logger := NewMemoryAuditLogger(100)
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    1 * time.Second,
		AuditLogger:       logger,
		PermissionChecker: allowAllChecker{},
	})
	tc := &ToolContext{
		CallerID:   "agent-001",
		CustomerID: "cust-100",
		AuditTrace: "trace-xyz",
	}
	toolCalls := []LLMToolCall{
		{
			ID: "call-1",
			Function: LLMToolFunction{
				Name:      "test.echo",
				Arguments: `{"foo":"bar"}`,
			},
		},
	}
	results := executor.DispatchByLLMToolCall(context.Background(), toolCalls, tc)
	if !results[0].Success {
		t.Fatalf("应成功")
	}
	// 验证 ToolContext 被传递到审计日志
	entries := logger.Entries()
	if len(entries) != 1 {
		t.Fatalf("期望 1 条审计日志，实际 %d", len(entries))
	}
	if entries[0].CallerID != "agent-001" {
		t.Fatalf("审计日志应包含 CallerID=agent-001，实际 %s", entries[0].CallerID)
	}
	if entries[0].CustomerID != "cust-100" {
		t.Fatalf("审计日志应包含 CustomerID=cust-100，实际 %s", entries[0].CustomerID)
	}
	if entries[0].AuditTrace != "trace-xyz" {
		t.Fatalf("审计日志应包含 AuditTrace=trace-xyz，实际 %s", entries[0].AuditTrace)
	}
}

// ===== 7. 便捷方法测试 =====

func TestToolExecutor_ExecuteByNameWithCtx(t *testing.T) {
	registry := NewToolRegistry()
	logger := NewMemoryAuditLogger(100)
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
		AuditLogger:    logger,
	})
	tc := &ToolContext{CallerID: "agent-007"}
	result, err := executor.ExecuteByNameWithCtx(
		context.Background(),
		"test.echo",
		map[string]any{"foo": "bar"},
		tc,
	)
	if err != nil {
		t.Fatalf("应成功：%v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}
	entries := logger.Entries()
	if entries[0].CallerID != "agent-007" {
		t.Fatalf("CallerID 应通过 ToolCtx 传递")
	}
}

func TestToolExecutor_ListAvailableTools(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.a"))
	registry.MustRegister(echoTool("test.b"))
	registry.MustRegister(echoTool("test.c"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	// 设置 test.b 为 disabled
	executor.SetOverride(ToolOverride{
		ToolName: "test.b",
		Disabled: true,
	})
	tools := executor.ListAvailableTools()
	if len(tools) != 2 {
		t.Fatalf("期望 2 个可用工具（disabled 排除），实际 %d", len(tools))
	}
	for _, tool := range tools {
		if tool.Name() == "test.b" {
			t.Fatalf("disabled 工具不应出现在可用列表中")
		}
	}
}

func TestToolExecutor_ListAvailableLLMFunctions(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.a"))
	registry.MustRegister(echoTool("test.b"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	functions := executor.ListAvailableLLMFunctions()
	if len(functions) != 2 {
		t.Fatalf("期望 2 个 LLM Function，实际 %d", len(functions))
	}
	for _, fn := range functions {
		if fn.Name != "test.a" && fn.Name != "test.b" {
			t.Fatalf("LLM Function 名称错误：%s", fn.Name)
		}
		if fn.Parameters.Type != "object" {
			t.Fatalf("Parameters.Type 期望 object，实际 %s", fn.Parameters.Type)
		}
	}
}

// ===== 8. ToolResult 字段补全测试 =====

func TestToolExecutor_ToolResultFieldsCompleted(t *testing.T) {
	registry := NewToolRegistry()
	// 工具返回部分字段为空
	emptyResultTool := newMockTool("test.empty", CategoryCustomer, func(ctx context.Context, args map[string]any) (ToolResult, error) {
		// 故意返回空的 ToolResult（仅设置 Success）
		return ToolResult{Success: true}, nil
	})
	registry.MustRegister(emptyResultTool)
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.empty",
	})
	if r.ToolName != "test.empty" {
		t.Fatalf("ToolName 应被补全为 test.empty，实际 %s", r.ToolName)
	}
	// DurationMs 可能为 0（执行 <1ms），仅验证非负
	if r.Timing.DurationMs < 0 {
		t.Fatalf("DurationMs 不应为负数，实际 %d", r.Timing.DurationMs)
	}
	if r.ExecutedAt.IsZero() {
		t.Fatalf("ExecutedAt 应被补全")
	}
}

func TestToolExecutor_AuditTracePassedThrough(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.echo"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	tc := &ToolContext{AuditTrace: "trace-abc-123"}
	r := executor.Execute(context.Background(), ExecuteRequest{
		ToolName: "test.echo",
		ToolCtx:  tc,
	})
	if r.AuditTrace != "trace-abc-123" {
		t.Fatalf("AuditTrace 应被传递到结果，实际 %s", r.AuditTrace)
	}
}

// ===== 9. 全局执行器测试 =====

func TestGlobalExecutor(t *testing.T) {
	// 保存原全局执行器
	original := globalExecutor
	defer func() { globalExecutor = original }()

	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.global"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	SetGlobalExecutor(executor)
	got := GetGlobalExecutor()
	if got == nil {
		t.Fatalf("应返回全局执行器")
	}
	if got != executor {
		t.Fatalf("应返回设置的执行器")
	}
	// 通过全局执行器调用
	result, err := got.ExecuteByName(context.Background(), "test.global", map[string]any{"foo": "bar"})
	if err != nil {
		t.Fatalf("应成功：%v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}
}

func TestGetGlobalExecutor_DefaultNil(t *testing.T) {
	// 保存原全局执行器
	original := globalExecutor
	defer func() { globalExecutor = original }()
	globalExecutor = nil
	if GetGlobalExecutor() != nil {
		t.Fatalf("未初始化时全局执行器应为 nil")
	}
}

// ===== 10. 并发安全测试 =====

func TestToolExecutor_ConcurrentExecute(t *testing.T) {
	registry := NewToolRegistry()
	var calls int32
	registry.MustRegister(countTool("test.concurrent", &calls))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout:    1 * time.Second,
		PermissionChecker: allowAllChecker{},
		RateLimiter:       NewTokenBucketLimiter(10000, 100), // 高 QPS 限流，避免被限
	})
	var wg sync.WaitGroup
	N := 50
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := executor.Execute(context.Background(), ExecuteRequest{
				ToolName: "test.concurrent",
			})
			if r.Err != nil {
				t.Errorf("并发执行应成功：%v", r.Err)
			}
		}()
	}
	wg.Wait()
	if atomic.LoadInt32(&calls) != int32(N) {
		t.Fatalf("期望 %d 次调用，实际 %d", N, calls)
	}
}

func TestToolExecutor_ConcurrentSetOverride(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.a"))
	registry.MustRegister(echoTool("test.b"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	var wg sync.WaitGroup
	// 并发设置 override
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "test.a"
			if i%2 == 0 {
				name = "test.b"
			}
			executor.SetOverride(ToolOverride{
				ToolName: name,
				Timeout:  time.Duration(i) * time.Second,
			})
		}(i)
	}
	// 并发执行
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := "test.a"
			if i%2 == 0 {
				name = "test.b"
			}
			_ = executor.Execute(context.Background(), ExecuteRequest{
				ToolName: name,
			})
		}(i)
	}
	wg.Wait()
	// 不应 panic / 死锁
}

// ===== 11. ClearCache 测试 =====

func TestToolExecutor_ClearCache(t *testing.T) {
	registry := NewToolRegistry()
	registry.MustRegister(echoTool("test.a"))
	registry.MustRegister(echoTool("test.b"))
	executor := NewToolExecutor(registry, ToolExecutorConfig{
		DefaultTimeout: 1 * time.Second,
	})
	// 执行填充缓存
	_ = executor.Execute(context.Background(), ExecuteRequest{ToolName: "test.a"})
	_ = executor.Execute(context.Background(), ExecuteRequest{ToolName: "test.b"})
	executor.mu.RLock()
	cacheLen1 := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen1 != 2 {
		t.Fatalf("期望 2 条缓存，实际 %d", cacheLen1)
	}
	// 清空缓存
	executor.ClearCache()
	executor.mu.RLock()
	cacheLen2 := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen2 != 0 {
		t.Fatalf("清空后应 0 条缓存，实际 %d", cacheLen2)
	}
	// 再次执行应重建缓存
	_ = executor.Execute(context.Background(), ExecuteRequest{ToolName: "test.a"})
	executor.mu.RLock()
	cacheLen3 := len(executor.cache)
	executor.mu.RUnlock()
	if cacheLen3 != 1 {
		t.Fatalf("重建后应 1 条缓存，实际 %d", cacheLen3)
	}
}

// ===== 辅助函数 =====

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// 确保 fmt 包被使用（避免 import 报错）
var _ = fmt.Sprintf
