package tooluse

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestStateMachine_NormalText 验证正常文本流
func TestStateMachine_NormalText(t *testing.T) {
	sm := NewStreamStateMachine()
	ctx := context.Background()

	chunks := []string{"你好", "，", "请问", "有什么", "可以帮您？"}
	for _, c := range chunks {
		action, err := sm.Process(ctx, c)
		if err != nil {
			t.Fatalf("process error: %v", err)
		}
		if action != ActionForwardToClient {
			t.Errorf("chunk %q: expected forward, got %s", c, action)
		}
	}

	if sm.State() != StateNormal {
		t.Errorf("state = %s, want normal", sm.State())
	}
}

// TestStateMachine_DetectTrigger 验证触发词检测
func TestStateMachine_DetectTrigger(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	action, _ := sm.Process(ctx, "好的，")
	if action != ActionForwardToClient {
		t.Fatal("first chunk should forward")
	}

	action, _ = sm.Process(ctx, "我需要调用工具：")
	if action != ActionBuffer {
	}

	action, _ = sm.Process(ctx, `{"tool":`)
	if action != ActionBuffer {
		t.Errorf("partial JSON should buffer, got %s", action)
	}

	action, _ = sm.Process(ctx, `"customer.search","args":{"q":"张三"}}`)
	if action != ActionExecuteTool {
		t.Errorf("complete JSON should trigger execute, got %s", action)
	}

	if sm.State() != StateExecuting {
		t.Errorf("state = %s, want executing", sm.State())
	}
	if sm.ToolName() != "customer.search" {
		t.Errorf("tool = %s, want customer.search", sm.ToolName())
	}
	if sm.ToolArgs()["q"] != "张三" {
		t.Errorf("args q = %v, want 张三", sm.ToolArgs()["q"])
	}
}

// TestStateMachine_NestedJSON 验证嵌套 JSON
func TestStateMachine_NestedJSON(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "好")
	_, _ = sm.Process(ctx, "调用工具：")

	action, err := sm.Process(ctx, `{"tool":"x","args":{"a":{"b":1},"c":[1,2,3]}}`)
	if err != nil {
		t.Fatalf("process error: %v", err)
	}
	if action != ActionExecuteTool {
		t.Errorf("nested JSON should trigger, got %s err=%v", action, err)
	}
	if sm.ToolArgs()["a"].(map[string]any)["b"].(float64) != 1 {
		t.Errorf("nested value wrong: %v", sm.ToolArgs())
	}
}

// TestStateMachine_JSONInString 验证字符串内的 {} 不影响解析
func TestStateMachine_JSONInString(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "调用工具：")
	action, _ := sm.Process(ctx, `{"tool":"x","args":{"text":"hello {world}"}}`)
	if action != ActionExecuteTool {
		t.Errorf("JSON with string { } should trigger, got %s", action)
	}
}

// TestStateMachine_ParseError 验证 JSON 解析失败
func TestStateMachine_ParseError(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "调用工具：")
	action, err := sm.Process(ctx, `{not valid json}`)
	if err == nil {
		t.Error("expected parse error")
	}
	if action != ActionFail {
		t.Errorf("action = %s, want fail", action)
	}
}

// TestStateMachine_MarkExecuted 验证 MarkToolExecuted 转换
func TestStateMachine_MarkExecuted(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "调用工具：{\"tool\":\"x\",\"args\":{}}")
	if sm.State() != StateExecuting {
		t.Fatal("should be executing")
	}
	sm.MarkToolExecuted(ToolResult{Success: true}, nil)
	if sm.State() != StateReassembling {
		t.Errorf("state = %s, want reassembling", sm.State())
	}
	sm.MarkReassembled()
	if sm.State() != StateDone {
		t.Errorf("state = %s, want done", sm.State())
	}
}

// TestStateMachine_Transitions 验证状态转换历史
func TestStateMachine_Transitions(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "调用工具：{\"tool\":\"x\",\"args\":{}}")
	sm.MarkToolExecuted(ToolResult{Success: true}, nil)
	sm.MarkReassembled()

	transitions := sm.Transitions()
	if len(transitions) < 3 {
		t.Errorf("expected at least 3 transitions, got %d", len(transitions))
	}
	for i, tr := range transitions {
		if tr.From == "" || tr.To == "" {
			t.Errorf("transition %d: from/to empty", i)
		}
	}
}

// TestStateMachine_Reset 验证重置
func TestStateMachine_Reset(t *testing.T) {
	sm := NewStreamStateMachine()
	sm.SetTrigger("调用工具：")
	ctx := context.Background()

	_, _ = sm.Process(ctx, "调用工具：{\"tool\":\"x\",\"args\":{}}")
	sm.Reset()

	if sm.State() != StateNormal {
		t.Errorf("after reset, state = %s, want normal", sm.State())
	}
	if sm.ToolName() != "" {
		t.Error("after reset, tool name should be empty")
	}
}

// TestMatchJSONEnd 验证 JSON 结束匹配
func TestMatchJSONEnd(t *testing.T) {
	tests := []struct {
		input    string
		start    int
		expected bool
		endIdx   int
	}{
		{`{"a":1}`, 0, true, 6},
		{`{"a":{"b":2}}`, 0, true, 12},
		{`{"a":[1,2,3]}`, 0, true, 12},
		{`{"a":"x{y}z"}`, 0, true, 12},
		{`{"a":1`, 0, false, -1},
		{`{`, 0, false, -1},
	}
	for _, tt := range tests {
		end, ok := matchJSONEnd(tt.input, tt.start)
		if ok != tt.expected {
			t.Errorf("matchJSONEnd(%q) ok=%v, want %v", tt.input, ok, tt.expected)
		}
		if ok && end != tt.endIdx {
			t.Errorf("matchJSONEnd(%q) end=%d, want %d", tt.input, end, tt.endIdx)
		}
	}
}

// TestToolRouter_BasicRoute 测试基本路由
func TestToolRouter_BasicRoute(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})

	testTool := &testMockTool{
		nameVal:   "test.echo",
		category:  CategoryBusiness,
		resultStr: "echo_ok",
	}
	if err := registry.Register(testTool); err != nil {
		t.Fatalf("register: %v", err)
	}

	router := NewToolRouter(executor, nil, RouterConfig{})

	result := router.Route(context.Background(), "test.echo", map[string]any{"x": 1}, &ToolContext{
		AgentID: "agent1",
	})
	if result.Err != nil {
		t.Fatalf("route error: %v", result.Err)
	}
	if !result.Result.Success {
		t.Error("expected success")
	}
	if result.Result.Data != "echo_ok" {
		t.Errorf("data = %v, want echo_ok", result.Result.Data)
	}

	stats := router.GetStats()
	if stats.TotalCalls != 1 {
		t.Errorf("total calls = %d, want 1", stats.TotalCalls)
	}
	if stats.SuccessCalls != 1 {
		t.Errorf("success = %d, want 1", stats.SuccessCalls)
	}
}

// TestToolRouter_NotFound 测试工具不存在
func TestToolRouter_NotFound(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	router := NewToolRouter(executor, nil, RouterConfig{})

	result := router.Route(context.Background(), "nonexistent.tool", nil, nil)
	if result.Err == nil {
		t.Error("expected error for nonexistent tool")
	}
}

// TestToolRouter_CircuitBreaker 测试熔断
func TestToolRouter_CircuitBreaker(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})

	failTool := &testMockTool{
		nameVal:  "fail.always",
		category: CategoryBusiness,
		failErr:  true,
	}
	if err := registry.Register(failTool); err != nil {
		t.Fatalf("register: %v", err)
	}

	router := NewToolRouter(executor, nil, RouterConfig{
		FailThreshold:    3,
		CooldownDuration: 5 * time.Second,
	})

	for i := 0; i < 3; i++ {
		router.Route(context.Background(), "fail.always", nil, nil)
	}

	result := router.Route(context.Background(), "fail.always", nil, nil)
	if !result.CircuitOpen {
		t.Error("expected circuit open after 3 failures")
	}

	stats := router.GetStats()
	if stats.CircuitOpenCalls == 0 {
		t.Error("expected CircuitOpenCalls > 0")
	}

	router.ResetCircuit("fail.always")
	result = router.Route(context.Background(), "fail.always", nil, nil)
	if result.CircuitOpen {
		t.Error("after reset, circuit should be closed")
	}
}

// TestToolRouter_RateLimit 测试限流
func TestToolRouter_RateLimit(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	testTool := &testMockTool{nameVal: "rate.test", category: CategoryBusiness, resultStr: "ok"}
	registry.Register(testTool)

	called := 0
	limiter := &testRateLimiter{acquireFunc: func(_ context.Context, _ string) error {
		called++
		if called > 1 {
			return &testRateLimitError{}
		}
		return nil
	}}

	router := NewToolRouter(executor, limiter, RouterConfig{})

	r1 := router.Route(context.Background(), "rate.test", nil, nil)
	if r1.RateLimit {
		t.Error("first call should not be rate limited")
	}
	r2 := router.Route(context.Background(), "rate.test", nil, nil)
	if !r2.RateLimit {
		t.Error("second call should be rate limited")
	}

	stats := router.GetStats()
	if stats.RateLimitedCalls == 0 {
		t.Error("expected RateLimitedCalls > 0")
	}
}

// TestToolRouter_CostTracking 测试成本统计
func TestToolRouter_CostTracking(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	tool := &testMockTool{nameVal: "cost.test", category: CategoryBusiness, resultStr: "ok"}
	registry.Register(tool)

	router := NewToolRouter(executor, nil, RouterConfig{DefaultToolCost: 0.01})
	router.SetToolCost("cost.test", 0.05)

	for i := 0; i < 3; i++ {
		router.Route(context.Background(), "cost.test", nil, nil)
	}

	stats := router.GetStats()
	expectedCost := 0.05 * 3
	if stats.TotalCost < expectedCost-0.001 || stats.TotalCost > expectedCost+0.001 {
		t.Errorf("total cost = %f, want %f", stats.TotalCost, expectedCost)
	}
}

// TestToolRouter_Stats 测试统计重置
func TestToolRouter_Stats(t *testing.T) {
	router := NewToolRouter(nil, nil, RouterConfig{})
	router.ResetStats()
	stats := router.GetStats()
	if stats.TotalCalls != 0 {
		t.Error("after reset, TotalCalls should be 0")
	}
}

// TestDoubleIntercept_DirectText 测试无工具调用的直接流
func TestDoubleIntercept_DirectText(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	router := NewToolRouter(executor, nil, RouterConfig{})

	secondPass := &testMockSecondPass{
		generateFunc: func(_ context.Context, _, _ string, _ ToolResult, _ func(string)) (string, error) {
			t.Error("should not call second pass for direct text")
			return "", nil
		},
	}

	orch, err := NewDoubleInterceptOrchestrator(OrchestratorConfig{
		Router:     router,
		SecondPass: secondPass,
	})
	if err != nil {
		t.Fatalf("create orch: %v", err)
	}

	stream := make(chan string, 3)
	stream <- "你好，"
	stream <- "请问"
	stream <- "需要什么帮助？"
	close(stream)

	reply, err := orch.Run(context.Background(), "hello", stream)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if !strings.Contains(reply, "你好") {
		t.Errorf("reply = %s, should contain 你好", reply)
	}

	msgs := orch.GetClientMessages()
	for _, m := range msgs {
		if !m.Forwarded {
			t.Error("all messages should be forwarded for direct text")
		}
	}
}

// TestDoubleIntercept_WithToolCall 测试有工具调用
func TestDoubleIntercept_WithToolCall(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})

	tool := &testMockTool{
		nameVal:   "customer.get",
		category:  CategoryCustomer,
		resultStr: `{"name":"张三","level":"VIP"}`,
	}
	registry.Register(tool)

	router := NewToolRouter(executor, nil, RouterConfig{})

	secondPass := &testMockSecondPass{
		generateFunc: func(_ context.Context, originalContent, toolName string, _ ToolResult, _ func(string)) (string, error) {
			if toolName != "customer.get" {
				t.Errorf("second pass tool = %s, want customer.get", toolName)
			}
			return "已为您查到客户信息：VIP 张三", nil
		},
	}

	orch, err := NewDoubleInterceptOrchestrator(OrchestratorConfig{
		Router:     router,
		SecondPass: secondPass,
	})
	if err != nil {
		t.Fatalf("create orch: %v", err)
	}

	stream := make(chan string, 5)
	stream <- "好的，让我查"
	stream <- "调用工具："
	stream <- `{"tool":"customer.get","args":{"id":"u1"}}`
	close(stream)

	reply, err := orch.Run(context.Background(), "查客户", stream)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	if !strings.Contains(reply, "VIP 张三") {
		t.Errorf("reply = %s, should contain VIP 张三", reply)
	}

	msgs := orch.GetClientMessages()
	hasIntercepted := false
	for _, m := range msgs {
		if !m.Forwarded {
			hasIntercepted = true
		}
	}
	if !hasIntercepted {
		t.Error("expected at least one intercepted message")
	}

	results := orch.GetToolResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 tool result, got %d", len(results))
	}
	if results[0].ToolName != "customer.get" {
		t.Errorf("tool name = %s, want customer.get", results[0].ToolName)
	}

	stats := orch.GetStats()
	if stats.ToolExecutions != 1 {
		t.Errorf("tool executions = %d, want 1", stats.ToolExecutions)
	}
	if stats.InterceptedChunks == 0 {
		t.Error("expected intercepted chunks > 0")
	}
}

// TestDoubleIntercept_RecursiveGuard 测试防止递归调用
func TestDoubleIntercept_RecursiveGuard(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})

	tool := &testMockTool{nameVal: "x.y", category: CategoryBusiness, resultStr: "ok"}
	registry.Register(tool)
	router := NewToolRouter(executor, nil, RouterConfig{})

	secondPass := &testMockSecondPass{
		generateFunc: func(_ context.Context, _, _ string, _ ToolResult, chunkHandler func(string)) (string, error) {
			chunkHandler("您的订单已发货调用工具：{\"tool\":\"order.get\"}")
			return "您的订单已发货", nil
		},
	}

	orch, _ := NewDoubleInterceptOrchestrator(OrchestratorConfig{
		Router:     router,
		SecondPass: secondPass,
	})

	stream := make(chan string, 3)
	stream <- "调用工具："
	stream <- `{"tool":"x.y","args":{}}`
	close(stream)

	_, err := orch.Run(context.Background(), "x", stream)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}

	results := orch.GetToolResults()
	if len(results) != 1 {
		t.Errorf("expected 1 tool execution (no recursive), got %d", len(results))
	}
}

// TestDoubleIntercept_Stats 验证统计
func TestDoubleIntercept_Stats(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	tool := &testMockTool{nameVal: "y.z", category: CategoryBusiness, resultStr: "ok"}
	registry.Register(tool)
	router := NewToolRouter(executor, nil, RouterConfig{})

	secondPass := &testMockSecondPass{
		generateFunc: func(_ context.Context, _, _ string, _ ToolResult, _ func(string)) (string, error) {
			return "ok reply", nil
		},
	}

	orch, _ := NewDoubleInterceptOrchestrator(OrchestratorConfig{
		Router:     router,
		SecondPass: secondPass,
	})

	stream := make(chan string, 3)
	stream <- "调用工具：{\"tool\":\"y.z\",\"args\":{}}"
	close(stream)

	_, _ = orch.Run(context.Background(), "x", stream)

	stats := orch.GetStats()
	if stats.ToolExecutions != 1 {
		t.Errorf("tool executions = %d, want 1", stats.ToolExecutions)
	}
	if stats.TotalDuration <= 0 {
		t.Error("total duration should be > 0")
	}
}

// TestDoubleIntercept_RequiresRouter 测试 Router 必须
func TestDoubleIntercept_RequiresRouter(t *testing.T) {
	_, err := NewDoubleInterceptOrchestrator(OrchestratorConfig{})
	if err == nil {
		t.Error("expected error when router is nil")
	}
}

// TestDoubleIntercept_RequiresSecondPass 测试 SecondPass 必须
func TestDoubleIntercept_RequiresSecondPass(t *testing.T) {
	registry := NewToolRegistry()
	executor := NewToolExecutor(registry, ToolExecutorConfig{})
	router := NewToolRouter(executor, nil, RouterConfig{})

	_, err := NewDoubleInterceptOrchestrator(OrchestratorConfig{
		Router: router,
	})
	if err == nil {
		t.Error("expected error when secondPass is nil")
	}
}

type testMockTool struct {
	nameVal   string
	category  ToolCategory
	resultStr string
	failErr   bool
}

func (t *testMockTool) Name() string           { return t.nameVal }
func (t *testMockTool) Category() ToolCategory { return t.category }
func (t *testMockTool) Description() string    { return "mock tool" }
func (t *testMockTool) Parameters() ToolParameters {
	return ToolParameters{Type: "object", Properties: map[string]ToolParam{}}
}
func (t *testMockTool) Execute(_ context.Context, _ map[string]any) (ToolResult, error) {
	if t.failErr {
		return ToolResult{Success: false, Error: "mock failure"}, &testToolError{msg: "mock failure"}
	}
	return ToolResult{Success: true, Data: t.resultStr}, nil
}

type testToolError struct{ msg string }

func (e *testToolError) Error() string { return e.msg }

type testRateLimiter struct {
	acquireFunc func(ctx context.Context, key string) error
}

func (r *testRateLimiter) Acquire(ctx context.Context, key string) error {
	return r.acquireFunc(ctx, key)
}

type testRateLimitError struct{}

func (e *testRateLimitError) Error() string { return "rate limited" }

type testMockSecondPass struct {
	generateFunc func(ctx context.Context, originalContent, toolName string, toolResult ToolResult, chunkHandler func(string)) (string, error)
}

func (m *testMockSecondPass) GenerateReassembledReply(
	ctx context.Context,
	originalContent, toolName string,
	toolResult ToolResult,
	chunkHandler func(string),
) (string, error) {
	return m.generateFunc(ctx, originalContent, toolName, toolResult, chunkHandler)
}
