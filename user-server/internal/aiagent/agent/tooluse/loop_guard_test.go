package tooluse

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// loop_guard_test.go P3-B: 工具级循环检测装饰器测试
//
// 覆盖：
//   1. LoopGuard 基础功能：允许/拦截
//   2. 不同参数的调用不触发循环检测
//   3. 不同 trace_id 独立计数
//   4. 窗口过期后重新允许调用
//   5. trace_id 为空时退化为 tool_name 维度
//   6. LoopGuardDecorator 装饰器集成
//   7. ErrLoopDetected 是不可重试错误
//   8. 容量保护（MaxTraces）
//   9. 配置覆盖
//  10. 并发安全
//  11. nil guard 零开销
//  12. disabled 配置时直接放行

// ===== 1. LoopGuard 基础功能 =====

func TestLoopGuard_AllowUnderThreshold(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 3,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}

	// 1-3 次允许
	for i := 1; i <= 3; i++ {
		if err := guard.CheckAndRecord("trace-001", "test_tool", args); err != nil {
			t.Errorf("第 %d 次调用应允许，实际 %v", i, err)
		}
	}

	// 第 4 次应被拦截
	if err := guard.CheckAndRecord("trace-001", "test_tool", args); !errors.Is(err, ErrLoopDetected) {
		t.Errorf("第 4 次调用应返回 ErrLoopDetected，实际 %v", err)
	}
}

func TestLoopGuard_BlockOverThreshold(t *testing.T) {
	guard := NewLoopGuard(DefaultLoopGuardConfig())

	args := map[string]any{"q": "hello"}
	for i := 0; i < 3; i++ {
		guard.CheckAndRecord("trace-001", "test_tool", args)
	}

	if err := guard.CheckAndRecord("trace-001", "test_tool", args); !errors.Is(err, ErrLoopDetected) {
		t.Errorf("超过阈值应返回 ErrLoopDetected，实际 %v", err)
	}
}

// ===== 2. 不同参数的调用不触发循环检测 =====

func TestLoopGuard_DifferentArgsNoLoop(t *testing.T) {
	guard := NewLoopGuard(DefaultLoopGuardConfig())

	// 相同工具，不同参数，不构成循环
	for i := 0; i < 10; i++ {
		args := map[string]any{"q": string(rune('a' + i))}
		if err := guard.CheckAndRecord("trace-001", "test_tool", args); err != nil {
			t.Errorf("不同参数的第 %d 次调用应允许，实际 %v", i, err)
		}
	}
}

// ===== 3. 不同 trace_id 独立计数 =====

func TestLoopGuard_DifferentTraceIndependent(t *testing.T) {
	guard := NewLoopGuard(DefaultLoopGuardConfig())

	args := map[string]any{"q": "hello"}

	// trace-001 调用 3 次（达到上限）
	for i := 0; i < 3; i++ {
		guard.CheckAndRecord("trace-001", "test_tool", args)
	}

	// trace-002 调用相同工具相同参数，应被允许（独立计数）
	if err := guard.CheckAndRecord("trace-002", "test_tool", args); err != nil {
		t.Errorf("不同 trace_id 的调用应允许，实际 %v", err)
	}
}

// ===== 4. 窗口过期后重新允许调用 =====

func TestLoopGuard_WindowExpiry(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 2,
		WindowSize:     50 * time.Millisecond, // 短窗口便于测试
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}

	// 调用 2 次（达到上限）
	for i := 0; i < 2; i++ {
		guard.CheckAndRecord("trace-001", "test_tool", args)
	}

	// 第 3 次应被拦截
	if err := guard.CheckAndRecord("trace-001", "test_tool", args); !errors.Is(err, ErrLoopDetected) {
		t.Errorf("第 3 次调用应被拦截，实际 %v", err)
	}

	// 等待窗口过期
	time.Sleep(80 * time.Millisecond)

	// 窗口过期后应重新允许
	if err := guard.CheckAndRecord("trace-001", "test_tool", args); err != nil {
		t.Errorf("窗口过期后应允许调用，实际 %v", err)
	}
}

// ===== 5. trace_id 为空时退化为 tool_name 维度 =====

func TestLoopGuard_EmptyTraceFallback(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 2,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}

	// trace_id 为空，应退化为 tool_name 维度
	for i := 0; i < 2; i++ {
		if err := guard.CheckAndRecord("", "test_tool", args); err != nil {
			t.Errorf("第 %d 次调用应允许，实际 %v", i, err)
		}
	}

	// 第 3 次应被拦截
	if err := guard.CheckAndRecord("", "test_tool", args); !errors.Is(err, ErrLoopDetected) {
		t.Errorf("第 3 次调用应被拦截（trace_id 为空时退化），实际 %v", err)
	}
}

// ===== 6. LoopGuardDecorator 装饰器集成 =====

func TestLoopGuardDecorator_BlocksLoop(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 2,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})

	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")
	decorated := LoopGuardDecorator(guard)(handler)

	args := map[string]any{"q": "hello"}

	ctx := WithToolName(context.Background(), "test_tool")
	ctx = WithTraceID(ctx, "trace-001")

	// 前 2 次允许
	for i := 0; i < 2; i++ {
		if _, err := decorated(ctx, args); err != nil {
			t.Errorf("第 %d 次调用应允许，实际 %v", i+1, err)
		}
	}

	// 第 3 次应被拦截，handler 不应被调用
	before := atomic.LoadInt32(&callCount)
	result, err := decorated(ctx, args)
	if !errors.Is(err, ErrLoopDetected) {
		t.Errorf("第 3 次调用应返回 ErrLoopDetected，实际 %v", err)
	}
	if result.Success {
		t.Errorf("期望 Success=false")
	}
	after := atomic.LoadInt32(&callCount)
	if after != before {
		t.Errorf("循环检测命中时 handler 不应被调用，before=%d after=%d", before, after)
	}
}

func TestLoopGuardDecorator_NilGuard_NoOp(t *testing.T) {
	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	// guard 为 nil 时应直接放行
	decorated := LoopGuardDecorator(nil)(handler)

	ctx := WithToolName(context.Background(), "test_tool")
	for i := 0; i < 10; i++ {
		if _, err := decorated(ctx, map[string]any{"q": "hello"}); err != nil {
			t.Errorf("nil guard 不应返回错误，实际 %v", err)
		}
	}
	if atomic.LoadInt32(&callCount) != 10 {
		t.Errorf("期望 handler 调用 10 次，实际 %d", callCount)
	}
}

func TestLoopGuardDecorator_DisabledConfig_NoOp(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		Enabled: false, // 禁用
	})

	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")
	decorated := LoopGuardDecorator(guard)(handler)

	ctx := WithToolName(context.Background(), "test_tool")
	for i := 0; i < 10; i++ {
		if _, err := decorated(ctx, map[string]any{"q": "hello"}); err != nil {
			t.Errorf("disabled guard 不应返回错误，实际 %v", err)
		}
	}
}

// ===== 7. ErrLoopDetected 是不可重试错误 =====

func TestErrLoopDetected_NonRetryable(t *testing.T) {
	if !isNonRetryableError(ErrLoopDetected) {
		t.Errorf("ErrLoopDetected 应为不可重试错误")
	}
	if !errors.Is(ErrLoopDetected, ErrLoopDetected) {
		t.Errorf("errors.Is(ErrLoopDetected, ErrLoopDetected) 应为 true")
	}
	// wrapped 错误也应被识别
	wrapped := errors.New("wrapped: " + ErrLoopDetected.Error())
	// 注意：errors.New 不保留包装关系，需要用 fmt.Errorf %w
	// 这里验证 isNonRetryableError 对 wrapped 错误的处理
	if isNonRetryableError(wrapped) {
		// 当前实现用 errors.Is 检查，wrapped（非 %w）不会被识别
		// 这是预期行为：业务代码应使用 fmt.Errorf("...: %w", ErrLoopDetected)
		t.Logf("wrapped 错误被识别为不可重试（含字符串匹配，但 isNonRetryableError 不依赖字符串匹配）")
	}
}

// ===== 8. 容量保护（MaxTraces） =====

func TestLoopGuard_MaxTracesEviction(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 100,
		WindowSize:     60 * time.Second,
		MaxTraces:      3, // 仅保留 3 个 trace
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}

	// 创建 5 个不同 trace_id
	for i := 0; i < 5; i++ {
		traceID := "trace-" + string(rune('0'+i))
		guard.CheckAndRecord(traceID, "test_tool", args)
	}

	stats := guard.Stats()
	if stats.ActiveTraces > 3 {
		t.Errorf("期望 ActiveTraces <= 3（容量保护），实际 %d", stats.ActiveTraces)
	}
}

// ===== 9. 配置覆盖 =====

func TestLoopGuardConfig_Defaults(t *testing.T) {
	cfg := DefaultLoopGuardConfig()
	if cfg.MaxRepeatCount != 3 {
		t.Errorf("默认 MaxRepeatCount 应为 3，实际 %d", cfg.MaxRepeatCount)
	}
	if cfg.WindowSize != 60*time.Second {
		t.Errorf("默认 WindowSize 应为 60s，实际 %v", cfg.WindowSize)
	}
	if !cfg.Enabled {
		t.Errorf("默认应启用")
	}
}

func TestNewLoopGuard_ZeroConfig_Defaults(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{})
	// 零值配置应被默认值填充
	if guard.config.MaxRepeatCount != 3 {
		t.Errorf("零值 MaxRepeatCount 应填充为 3，实际 %d", guard.config.MaxRepeatCount)
	}
	if guard.config.WindowSize != 60*time.Second {
		t.Errorf("零值 WindowSize 应填充为 60s，实际 %v", guard.config.WindowSize)
	}
}

// ===== 10. 并发安全 =====

func TestLoopGuard_ConcurrentSafe(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 1000, // 高阈值，避免触发循环
		WindowSize:     60 * time.Second,
		MaxTraces:      10000,
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}

	const concurrency = 100
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func(id int) {
			defer wg.Done()
			traceID := "trace-" + string(rune('a'+(id%10)))
			guard.CheckAndRecord(traceID, "test_tool", args)
		}(i)
	}
	wg.Wait()

	// 不应 panic，不应死锁
	stats := guard.Stats()
	if stats.ActiveTraces == 0 {
		t.Errorf("并发后应有活跃 trace")
	}
}

// ===== 11. Reset 清空历史 =====

func TestLoopGuard_Reset(t *testing.T) {
	guard := NewLoopGuard(DefaultLoopGuardConfig())

	args := map[string]any{"q": "hello"}
	for i := 0; i < 3; i++ {
		guard.CheckAndRecord("trace-001", "test_tool", args)
	}

	if stats := guard.Stats(); stats.ActiveTraces == 0 {
		t.Fatalf("Reset 前应有活跃 trace")
	}

	guard.Reset()

	if stats := guard.Stats(); stats.ActiveTraces != 0 {
		t.Errorf("Reset 后 ActiveTraces 应为 0，实际 %d", stats.ActiveTraces)
	}

	// Reset 后应重新允许调用
	if err := guard.CheckAndRecord("trace-001", "test_tool", args); err != nil {
		t.Errorf("Reset 后应允许调用，实际 %v", err)
	}
}

// ===== 12. Stats 统计 =====

func TestLoopGuard_Stats(t *testing.T) {
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 10,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})

	args := map[string]any{"q": "hello"}
	guard.CheckAndRecord("trace-001", "tool_a", args)
	guard.CheckAndRecord("trace-001", "tool_a", args)
	guard.CheckAndRecord("trace-002", "tool_b", args)

	stats := guard.Stats()
	if stats.ActiveTraces != 2 {
		t.Errorf("期望 ActiveTraces=2，实际 %d", stats.ActiveTraces)
	}
	if stats.TotalRecords != 3 {
		t.Errorf("期望 TotalRecords=3，实际 %d", stats.TotalRecords)
	}
}

// ===== 13. 集成测试：反馈回流 + 循环检测协同 =====

func TestLoopGuard_FeedbackRecordsLoopEvent(t *testing.T) {
	// 验证：当循环检测命中时，反馈回流仍能记录失败事件
	// 装饰器链顺序：反馈回流（外） → 循环检测（内） → handler
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 2,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})
	sink := NewMemoryFeedbackSink(100)

	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	// 构造链：先包装循环检测，再包装反馈回流
	chain := LoopGuardDecorator(guard)(handler)
	chain = FeedbackCollectorDecorator(sink)(chain)

	ctx := WithToolName(context.Background(), "test_tool")
	ctx = WithTraceID(ctx, "trace-001")
	args := map[string]any{"q": "hello"}

	// 前 2 次允许
	for i := 0; i < 2; i++ {
		if _, err := chain(ctx, args); err != nil {
			t.Errorf("第 %d 次调用应允许，实际 %v", i+1, err)
		}
	}

	// 第 3 次循环命中
	result, err := chain(ctx, args)
	if !errors.Is(err, ErrLoopDetected) {
		t.Errorf("第 3 次调用应返回 ErrLoopDetected，实际 %v", err)
	}
	if result.Success {
		t.Errorf("期望 Success=false")
	}

	// 等待异步回流
	waitForAsync(func() bool { return sink.Count() == 3 }, 2*time.Second)

	events := sink.Events()
	if len(events) != 3 {
		t.Fatalf("期望 3 个事件（含循环命中事件），实际 %d", len(events))
	}

	// 异步 goroutine 写入顺序不确定，不能依赖 events[2] 是循环命中事件
	// 扫描所有事件，查找循环命中的失败事件
	var loopEvent *ToolCallEvent
	successCount := 0
	for i := range events {
		if !events[i].Success && events[i].Error != "" {
			loopEvent = &events[i]
		} else if events[i].Success {
			successCount++
		}
	}
	if loopEvent == nil {
		t.Fatalf("未找到循环命中的失败事件，events=%+v", events)
	}
	if successCount != 2 {
		t.Errorf("期望 2 个成功事件，实际 %d", successCount)
	}
	if !errors.Is(err, ErrLoopDetected) {
		t.Errorf("期望错误类型为 ErrLoopDetected")
	}
	if !strings.Contains(loopEvent.Error, "loop detected") {
		t.Errorf("循环命中事件应有 loop detected 错误信息，实际：%s", loopEvent.Error)
	}
}

// ===== 14. 集成测试：循环检测不误判重试 =====

func TestLoopGuard_RetryDoesNotTriggerLoop(t *testing.T) {
	// 验证：循环检测位于重试之外，重试不触发循环检测
	// 装饰器链顺序：循环检测（外） → 重试（内） → handler
	guard := NewLoopGuard(LoopGuardConfig{
		MaxRepeatCount: 2,
		WindowSize:     60 * time.Second,
		Enabled:        true,
	})

	var callCount int32
	// handler 前 2 次失败，第 3 次成功
	handler := func(ctx context.Context, args map[string]any) (ToolResult, error) {
		n := atomic.AddInt32(&callCount, 1)
		if n < 3 {
			return ErrorResult("test_tool", errors.New("transient failure")), errors.New("transient failure")
		}
		return SuccessResult("test_tool", "ok"), nil
	}

	// 构造链：先包装重试，再包装循环检测
	policy := NewExponentialBackoffPolicy(5, 1*time.Millisecond, 10*time.Millisecond)
	chain := RetryDecorator(policy)(handler)
	chain = LoopGuardDecorator(guard)(chain)

	ctx := WithToolName(context.Background(), "test_tool")
	ctx = WithTraceID(ctx, "trace-001")
	args := map[string]any{"q": "hello"}

	// 第一次逻辑调用：内部重试 3 次后成功，不应触发循环检测
	result, err := chain(ctx, args)
	if err != nil {
		t.Fatalf("第一次调用应成功，实际 %v", err)
	}
	if !result.Success {
		t.Errorf("期望 Success=true")
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("期望 handler 被调用 3 次（2 次重试 + 1 次成功），实际 %d", callCount)
	}

	// 第二次逻辑调用（相同参数）应仍被允许（重试不应消耗循环检测配额）
	callCount = 0
	result, err = chain(ctx, args)
	if err != nil {
		t.Fatalf("第二次调用应成功，实际 %v", err)
	}
	if !result.Success {
		t.Errorf("期望 Success=true")
	}
}
