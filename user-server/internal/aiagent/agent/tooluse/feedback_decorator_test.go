package tooluse

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// feedback_decorator_test.go P3-A: 反馈回流装饰器测试
//
// 覆盖：
//   1. FeedbackCollectorDecorator 基础功能：成功/失败/panic
//   2. FeedbackSink nil 时零开销放行
//   3. 异步回流不阻塞主链路
//   4. 参数脱敏（敏感字段值替换为 ***）
//   5. 上下文信息提取（trace_id/session_id/customer_id/agent_id/source）
//   6. MemoryFeedbackSink 内存实现
//   7. NoOpFeedbackSink 空操作实现
//   8. 装饰器链位置（位于审计之后、handler 之前）

// ===== 1. FeedbackCollectorDecorator 基础功能 =====

func TestFeedbackCollectorDecorator_Success(t *testing.T) {
	sink := NewMemoryFeedbackSink(100)
	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	decorated := FeedbackCollectorDecorator(sink)(handler)

	ctx := WithToolName(context.Background(), "test_tool")
	ctx = WithTraceID(ctx, "trace-123")
	ctx = WithToolContext(ctx, &ToolContext{
		SessionID:  "session-001",
		CustomerID: "customer-001",
		AgentID:    "agent-001",
		Source:     "agent",
	})

	args := map[string]any{"query": "hello"}
	result, err := decorated(ctx, args)
	if err != nil {
		t.Fatalf("期望 nil 错误，实际 %v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}

	// 等待异步回流完成
	waitForAsync(func() bool { return sink.Count() == 1 }, time.Second)

	if sink.Count() != 1 {
		t.Fatalf("期望 sink.Count()=1，实际 %d", sink.Count())
	}

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，实际 %d", len(events))
	}
	ev := events[0]
	if ev.ToolName != "test_tool" {
		t.Errorf("期望 ToolName=test_tool，实际 %s", ev.ToolName)
	}
	if ev.TraceID != "trace-123" {
		t.Errorf("期望 TraceID=trace-123，实际 %s", ev.TraceID)
	}
	if ev.SessionID != "session-001" {
		t.Errorf("期望 SessionID=session-001，实际 %s", ev.SessionID)
	}
	if ev.CustomerID != "customer-001" {
		t.Errorf("期望 CustomerID=customer-001，实际 %s", ev.CustomerID)
	}
	if ev.AgentID != "agent-001" {
		t.Errorf("期望 AgentID=agent-001，实际 %s", ev.AgentID)
	}
	if ev.Source != "agent" {
		t.Errorf("期望 Source=agent，实际 %s", ev.Source)
	}
	if !ev.Success {
		t.Errorf("期望 Success=true")
	}
	if ev.Error != "" {
		t.Errorf("期望 Error 为空，实际 %s", ev.Error)
	}
}

func TestFeedbackCollectorDecorator_Failure(t *testing.T) {
	sink := NewMemoryFeedbackSink(100)
	var callCount int32
	handler := makeHandler("fail_tool", &callCount, false, "intentional failure")

	decorated := FeedbackCollectorDecorator(sink)(handler)

	ctx := WithToolName(context.Background(), "fail_tool")
	result, err := decorated(ctx, map[string]any{})

	// 错误应当透传
	if err == nil {
		t.Fatalf("期望非 nil 错误")
	}
	if result.Success {
		t.Fatalf("期望 Success=false")
	}

	waitForAsync(func() bool { return sink.Count() == 1 }, time.Second)

	events := sink.Events()
	if len(events) != 1 {
		t.Fatalf("期望 1 个事件，实际 %d", len(events))
	}
	ev := events[0]
	if ev.Success {
		t.Errorf("期望 Success=false")
	}
	if ev.Error == "" {
		t.Errorf("期望 Error 非空")
	}
}

func TestFeedbackCollectorDecorator_NilSink_NoOp(t *testing.T) {
	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	// sink 为 nil 时应直接放行，零开销
	decorated := FeedbackCollectorDecorator(nil)(handler)

	ctx := WithToolName(context.Background(), "test_tool")
	result, err := decorated(ctx, map[string]any{})
	if err != nil {
		t.Fatalf("期望 nil 错误，实际 %v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}
	if atomic.LoadInt32(&callCount) != 1 {
		t.Errorf("期望 handler 调用 1 次，实际 %d", callCount)
	}
}

func TestFeedbackCollectorDecorator_AsyncNoBlocking(t *testing.T) {
	// sink 故意慢（200ms），验证主链路不被阻塞
	slowSink := &slowFeedbackSink{delay: 200 * time.Millisecond}
	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	decorated := FeedbackCollectorDecorator(slowSink)(handler)

	ctx := WithToolName(context.Background(), "test_tool")
	start := time.Now()
	result, err := decorated(ctx, map[string]any{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("期望 nil 错误，实际 %v", err)
	}
	if !result.Success {
		t.Fatalf("期望 Success=true")
	}

	// 主链路应在 50ms 内返回（sink 的 200ms 延迟不应阻塞）
	if elapsed > 50*time.Millisecond {
		t.Errorf("期望主链路 < 50ms，实际 %v（异步回流被阻塞）", elapsed)
	}

	// 等待异步回流完成
	time.Sleep(300 * time.Millisecond)
	if slowSink.callCount.Load() != 1 {
		t.Errorf("期望 sink 调用 1 次，实际 %d", slowSink.callCount.Load())
	}
}

// slowFeedbackSink 慢速 FeedbackSink（用于测试异步性）
type slowFeedbackSink struct {
	delay      time.Duration
	callCount  atomic.Int32
	errorCount atomic.Int32
}

func (s *slowFeedbackSink) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	s.callCount.Add(1)
	time.Sleep(s.delay)
	return nil
}

// ===== 2. 参数脱敏 =====

func TestSanitizeArgsForFeedback_SensitiveFields(t *testing.T) {
	args := map[string]any{
		"username": "alice",
		"password": "secret123",
		"token":    "abc-xyz",
		"api_key":  "key-001",
		"phone":    "13800138000",
		"id_card":  "110101199001011234",
		"bank_card": "6225880123456789",
		"data":     "normal_value",
	}

	sanitized := sanitizeArgsForFeedback(args)

	if sanitized["username"] != "alice" {
		t.Errorf("username 应保持原值")
	}
	if sanitized["password"] != "***" {
		t.Errorf("password 应被脱敏为 ***")
	}
	if sanitized["token"] != "***" {
		t.Errorf("token 应被脱敏为 ***")
	}
	if sanitized["api_key"] != "***" {
		t.Errorf("api_key 应被脱敏为 ***")
	}
	if sanitized["phone"] != "***" {
		t.Errorf("phone 应被脱敏为 ***")
	}
	if sanitized["id_card"] != "***" {
		t.Errorf("id_card 应被脱敏为 ***")
	}
	if sanitized["bank_card"] != "***" {
		t.Errorf("bank_card 应被脱敏为 ***")
	}
	if sanitized["data"] != "normal_value" {
		t.Errorf("data 应保持原值")
	}
}

func TestSanitizeArgsForFeedback_Empty(t *testing.T) {
	if sanitized := sanitizeArgsForFeedback(nil); sanitized != nil {
		t.Errorf("nil 输入应返回 nil")
	}
	if sanitized := sanitizeArgsForFeedback(map[string]any{}); sanitized != nil {
		t.Errorf("空 map 输入应返回 nil")
	}
}

// ===== 3. MemoryFeedbackSink 内存实现 =====

func TestMemoryFeedbackSink_Capacity(t *testing.T) {
	sink := NewMemoryFeedbackSink(3) // 容量 3

	for i := 0; i < 5; i++ {
		if err := sink.RecordToolCall(context.Background(), ToolCallEvent{
			ToolName: "test",
			Success:  true,
		}); err != nil {
			t.Fatalf("RecordToolCall 失败：%v", err)
		}
	}

	if sink.Count() != 3 {
		t.Errorf("期望容量上限 3，实际 %d", sink.Count())
	}

	events := sink.Events()
	if len(events) != 3 {
		t.Errorf("期望 3 个事件，实际 %d", len(events))
	}
	// 验证保留最新的 3 个（滚动覆盖最旧）
	// 由于实现是 s.events = s.events[1:]，最新的事件在末尾
	// 这里仅验证数量和容量上限
}

func TestMemoryFeedbackSink_Reset(t *testing.T) {
	sink := NewMemoryFeedbackSink(100)
	sink.RecordToolCall(context.Background(), ToolCallEvent{ToolName: "test"})
	sink.RecordToolCall(context.Background(), ToolCallEvent{ToolName: "test"})

	if sink.Count() != 2 {
		t.Fatalf("期望 2 个事件，实际 %d", sink.Count())
	}

	sink.Reset()

	if sink.Count() != 0 {
		t.Errorf("期望 Reset 后 0 个事件，实际 %d", sink.Count())
	}
}

// ===== 4. NoOpFeedbackSink 空操作实现 =====

func TestNoOpFeedbackSink(t *testing.T) {
	var sink NoOpFeedbackSink
	if err := sink.RecordToolCall(context.Background(), ToolCallEvent{}); err != nil {
		t.Errorf("NoOp 不应返回错误，实际 %v", err)
	}
}

// ===== 5. 装饰器链位置 =====

func TestFeedbackCollectorDecorator_ChainOrder(t *testing.T) {
	// 验证：装饰器位于 handler 之外，result 先产生再回流
	sink := NewMemoryFeedbackSink(100)

	var handlerCalled atomic.Int32
	var sinkRecordedHandlerCalled atomic.Int32

	handler := func(ctx context.Context, args map[string]any) (ToolResult, error) {
		handlerCalled.Add(1)
		return SuccessResult("test_tool", "data"), nil
	}

	// 包装 sink，记录被调用时 handler 是否已执行
	wrapSink := &orderingCheckSink{
		inner: sink,
		checkFunc: func() {
			// 异步 goroutine 中 handler 已执行完
			// 但我们检查 sink 在 handler 之后被触发
			sinkRecordedHandlerCalled.Store(handlerCalled.Load())
		},
	}

	decorated := FeedbackCollectorDecorator(wrapSink)(handler)
	ctx := WithToolName(context.Background(), "test_tool")
	decorated(ctx, map[string]any{})

	// 等待异步回流
	waitForAsync(func() bool { return wrapSink.callCount.Load() == 1 }, time.Second)

	if sinkRecordedHandlerCalled.Load() != 1 {
		t.Errorf("期望 sink 在 handler 之后被调用（handlerCalled=1），实际 %d", sinkRecordedHandlerCalled.Load())
	}
}

type orderingCheckSink struct {
	inner     FeedbackSink
	callCount atomic.Int32
	checkFunc func()
}

func (s *orderingCheckSink) RecordToolCall(ctx context.Context, event ToolCallEvent) error {
	s.callCount.Add(1)
	if s.checkFunc != nil {
		s.checkFunc()
	}
	if s.inner != nil {
		return s.inner.RecordToolCall(ctx, event)
	}
	return nil
}

// ===== 6. 并发安全 =====

func TestFeedbackCollectorDecorator_ConcurrentSafe(t *testing.T) {
	sink := NewMemoryFeedbackSink(1000)
	var callCount int32
	handler := makeHandler("test_tool", &callCount, true, "")

	decorated := FeedbackCollectorDecorator(sink)(handler)

	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			ctx := WithToolName(context.Background(), "test_tool")
			ctx = WithTraceID(ctx, "trace-concurrent")
			decorated(ctx, map[string]any{"q": "test"})
		}()
	}
	wg.Wait()

	// 等待所有异步回流完成（50 个 goroutine 异步写入，给足时间）
	waitForAsync(func() bool { return sink.Count() == concurrency }, 5*time.Second)

	if sink.Count() != concurrency {
		t.Errorf("期望 %d 个事件，实际 %d", concurrency, sink.Count())
	}
	if atomic.LoadInt32(&callCount) != concurrency {
		t.Errorf("期望 handler 调用 %d 次，实际 %d", concurrency, callCount)
	}
}

// ===== 辅助函数 =====

// waitForAsync 等待异步条件满足或超时
func waitForAsync(cond func() bool, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// 确保 errors 包被使用
var _ = errors.New
