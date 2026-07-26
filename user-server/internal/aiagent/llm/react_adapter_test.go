package llm

import (
	"strings"
	"testing"
)

// react_adapter_test.go ReAct Prompting 适配器测试
//
// 覆盖：
//   D8.1 ParseReActResponse: 解析 Action + Action Input
//   D8.2 ParseReActResponse: 解析 Final Answer
//   D8.3 ParseReActResponse: 解析 Thought
//   D8.4 ParseReActResponse: 空内容 / 无协议文本降级
//   D8.5 ToToolCall: ReAct → ToolCall 转换
//   D8.6 WrapSystemPrompt: 注入工具描述
//   D8.7 AdaptResult: Action 路径注入 ToolCalls
//   D8.8 AdaptResult: Final Answer 路径替换 Content
//   D8.9 IsReActMode: NoFC + Tools 触发
//   D8.10 BuildObservationMessage: Observation 消息构造

// ===== D8.1: 解析 Action + Action Input =====

func TestD8_1_ParseReActResponse_Action(t *testing.T) {
	content := `Thought: 我需要查询客户信息
Action: customer.search
Action Input: {"phone":"13800138000"}`
	result, err := ParseReActResponse(content)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if result.IsFinal {
		t.Fatal("应识别为 Action（IsFinal=false）")
	}
	if result.Action != "customer.search" {
		t.Errorf("Action 期望 customer.search，实际 %s", result.Action)
	}
	if !strings.Contains(result.ActionInput, "13800138000") {
		t.Errorf("ActionInput 期望含 phone，实际 %s", result.ActionInput)
	}
	if result.Thought != "我需要查询客户信息" {
		t.Errorf("Thought 期望 '我需要查询客户信息'，实际 %q", result.Thought)
	}
}

// ===== D8.2: 解析 Final Answer =====

func TestD8_2_ParseReActResponse_FinalAnswer(t *testing.T) {
	content := `Thought: 已获得客户信息
Final Answer: 客户张三，手机 13800138000`
	result, err := ParseReActResponse(content)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if !result.IsFinal {
		t.Fatal("应识别为 Final Answer（IsFinal=true）")
	}
	if !strings.Contains(result.FinalAnswer, "客户张三") {
		t.Errorf("FinalAnswer 期望含 '客户张三'，实际 %q", result.FinalAnswer)
	}
	if result.Action != "" {
		t.Errorf("Final Answer 时 Action 应为空，实际 %q", result.Action)
	}
}

// ===== D8.3: 解析 Thought =====

func TestD8_3_ParseReActResponse_Thought(t *testing.T) {
	// 仅 Thought 无 Action 也不应崩溃（降级为 Final Answer）
	content := `Thought: 我正在思考`
	result, err := ParseReActResponse(content)
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	// 仅 Thought 不匹配 Action/Final Answer，触发降级路径
	if !result.IsFinal {
		t.Fatal("仅 Thought 应降级为 Final Answer")
	}
}

// ===== D8.4: 空内容 / 无协议文本降级 =====

func TestD8_4_ParseReActResponse_EmptyAndFallback(t *testing.T) {
	// 空内容应报错
	if _, err := ParseReActResponse(""); err == nil {
		t.Fatal("空内容应返回错误")
	}

	// 无 ReAct 协议文本降级为 Final Answer
	plainText := "你好，请问需要什么帮助？"
	result, err := ParseReActResponse(plainText)
	if err != nil {
		t.Fatalf("降级解析失败：%v", err)
	}
	if !result.IsFinal {
		t.Fatal("无协议文本应降级为 Final Answer")
	}
	if result.FinalAnswer != plainText {
		t.Errorf("FinalAnswer 应等于原文，实际 %q", result.FinalAnswer)
	}
}

// ===== D8.5: ToToolCall 转换 =====

func TestD8_5_ToToolCall_Conversion(t *testing.T) {
	// Action 路径
	r := &ReActParseResult{
		IsFinal:     false,
		Action:      "customer.search",
		ActionInput: `{"phone":"13800138000"}`,
	}
	tc := r.ToToolCall("react_test_1")
	if tc == nil {
		t.Fatal("Action 路径应返回非 nil ToolCall")
	}
	if tc.ID != "react_test_1" {
		t.Errorf("ID 期望 react_test_1，实际 %s", tc.ID)
	}
	if tc.Type != "function" {
		t.Errorf("Type 期望 function，实际 %s", tc.Type)
	}
	if tc.Function.Name != "customer.search" {
		t.Errorf("Function.Name 期望 customer.search，实际 %s", tc.Function.Name)
	}
	if tc.Function.Arguments != `{"phone":"13800138000"}` {
		t.Errorf("Function.Arguments 错误：%s", tc.Function.Arguments)
	}

	// Final Answer 路径应返回 nil
	finalR := &ReActParseResult{
		IsFinal:     true,
		FinalAnswer: "你好",
	}
	if tc := finalR.ToToolCall("test"); tc != nil {
		t.Fatal("Final Answer 路径应返回 nil ToolCall")
	}
}

// ===== D8.6: WrapSystemPrompt 注入工具描述 =====

func TestD8_6_WrapSystemPrompt(t *testing.T) {
	adapter := NewReActAdapter()
	original := "你是营销助手"
	tools := []ToolDefinition{
		{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "customer.search",
				Description: "按手机号搜索客户",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"phone": map[string]any{"type": "string"},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunctionSchema{
				Name:        "order.query",
				Description: "查询订单",
			},
		},
	}
	wrapped := adapter.WrapSystemPrompt(original, tools)

	// 验证：包含原始 prompt
	if !strings.Contains(wrapped, original) {
		t.Error("WrapSystemPrompt 应保留原 SystemPrompt")
	}
	// 验证：包含每个工具名
	for _, tool := range tools {
		if !strings.Contains(wrapped, tool.Function.Name) {
			t.Errorf("WrapSystemPrompt 应含工具名 %s", tool.Function.Name)
		}
		if !strings.Contains(wrapped, tool.Function.Description) {
			t.Errorf("WrapSystemPrompt 应含工具描述 %s", tool.Function.Description)
		}
	}
	// 验证：包含 ReAct 协议关键字
	for _, keyword := range []string{"Thought", "Action", "Action Input", "Final Answer"} {
		if !strings.Contains(wrapped, keyword) {
			t.Errorf("WrapSystemPrompt 应含 ReAct 关键字 %q", keyword)
		}
	}
}

// ===== D8.7: AdaptResult Action 路径 =====

func TestD8_7_AdaptResult_ActionPath(t *testing.T) {
	adapter := NewReActAdapter()
	result := &DispatchResult{
		Provider: "mtk-llm",
		Model:    "qwen2.5-3b-instruct",
		Content: `Thought: 查询客户
Action: customer.search
Action Input: {"phone":"13800138000"}`,
		FinishReason: "stop",
	}
	adapted := adapter.AdaptResult(result)
	if adapted == nil {
		t.Fatal("AdaptResult 返回 nil")
	}
	if len(adapted.ToolCalls) != 1 {
		t.Fatalf("期望 1 个 ToolCall，实际 %d", len(adapted.ToolCalls))
	}
	tc := adapted.ToolCalls[0]
	if tc.Function.Name != "customer.search" {
		t.Errorf("ToolCall Function.Name 期望 customer.search，实际 %s", tc.Function.Name)
	}
	if adapted.FinishReason != "tool_calls" {
		t.Errorf("FinishReason 期望 tool_calls，实际 %s", adapted.FinishReason)
	}
	if !strings.HasPrefix(tc.ID, "react_") {
		t.Errorf("ToolCall ID 应以 react_ 开头，实际 %s", tc.ID)
	}
}

// ===== D8.8: AdaptResult Final Answer 路径 =====

func TestD8_8_AdaptResult_FinalAnswerPath(t *testing.T) {
	adapter := NewReActAdapter()
	result := &DispatchResult{
		Provider: "mtk-llm",
		Model:    "qwen2.5-3b-instruct",
		Content: `Thought: 已查询
Final Answer: 客户张三，手机 13800138000`,
		FinishReason: "stop",
	}
	adapted := adapter.AdaptResult(result)
	if adapted == nil {
		t.Fatal("AdaptResult 返回 nil")
	}
	if len(adapted.ToolCalls) != 0 {
		t.Errorf("Final Answer 路径不应有 ToolCall，实际 %d 个", len(adapted.ToolCalls))
	}
	if !strings.Contains(adapted.Content, "客户张三") {
		t.Errorf("Content 应替换为 Final Answer，实际 %q", adapted.Content)
	}
	if adapted.FinishReason != "stop" {
		t.Errorf("FinishReason 期望 stop，实际 %s", adapted.FinishReason)
	}
}

// ===== D8.9: IsReActMode 触发条件 =====

func TestD8_9_IsReActMode(t *testing.T) {
	// NoFC=true + Tools 非空 → 触发
	req := &DispatchRequest{
		Tools: []ToolDefinition{{Type: "function", Function: ToolFunctionSchema{Name: "test"}}},
	}
	if !IsReActMode(req, true) {
		t.Error("NoFC=true + Tools 应触发 ReAct 模式")
	}

	// NoFC=true + Tools 为空 → 不触发
	emptyReq := &DispatchRequest{}
	if IsReActMode(emptyReq, true) {
		t.Error("NoFC=true + 空 Tools 不应触发 ReAct 模式")
	}

	// NoFC=false + Tools 非空 → 不触发（走原生 FC 路径）
	if IsReActMode(req, false) {
		t.Error("NoFC=false 不应触发 ReAct 模式")
	}

	// nil req → 不触发
	if IsReActMode(nil, true) {
		t.Error("nil req 不应触发 ReAct 模式")
	}
}

// ===== D8.10: BuildObservationMessage =====

func TestD8_10_BuildObservationMessage(t *testing.T) {
	msg := BuildObservationMessage("customer.search", "react_123_1", `{"id":"cust-001"}`)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user，实际 %s", msg.Role)
	}
	if !strings.HasPrefix(msg.Content, "Observation:") {
		t.Errorf("Content 应以 'Observation:' 开头，实际 %q", msg.Content)
	}
	if !strings.Contains(msg.Content, `{"id":"cust-001"}`) {
		t.Errorf("Content 应含 observation 内容，实际 %q", msg.Content)
	}
	if msg.ToolCallID != "react_123_1" {
		t.Errorf("ToolCallID 期望 react_123_1，实际 %s", msg.ToolCallID)
	}
	if msg.Name != "customer.search" {
		t.Errorf("Name 期望 customer.search，实际 %s", msg.Name)
	}
}

// ===== D8.11: 并发安全（toolCallID 唯一性） =====

func TestD8_11_AdaptResult_ConcurrentIDUniqueness(t *testing.T) {
	adapter := NewReActAdapter()
	const N = 100
	ids := make(map[string]bool, N)
	done := make(chan string, N)
	for i := 0; i < N; i++ {
		go func() {
			r := &DispatchResult{
				Content: `Action: test.tool
Action Input: {}`,
			}
			adapted := adapter.AdaptResult(r)
			if len(adapted.ToolCalls) > 0 {
				done <- adapted.ToolCalls[0].ID
			} else {
				done <- ""
			}
		}()
	}
	for i := 0; i < N; i++ {
		id := <-done
		if id == "" {
			t.Fatal("并发场景下 AdaptResult 应返回 ToolCall")
		}
		if ids[id] {
			t.Fatalf("ToolCall ID 重复：%s", id)
		}
		ids[id] = true
	}
}
