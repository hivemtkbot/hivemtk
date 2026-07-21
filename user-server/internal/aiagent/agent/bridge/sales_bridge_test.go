package agent_bridge

import (
	"context"
	"testing"

	"marketing/internal/aiagent/agent/runtime"
	"marketing/internal/dto"
)

// ============================================================================
// salesEngineBridge 单元测试
// ----------------------------------------------------------------------------
// 验证:
//   1. nil engine 返回 ErrBridgeNotInitialized
//   2. 类型转换正确性
//   3. 响应转换正确性
// ============================================================================

func TestSalesEngineBridge_NilEngine(t *testing.T) {
	b := &salesEngineBridge{engine: nil}
	_, err := b.HandleWithAgent(context.Background(), &agent_runtime.AgentContext{}, &agent_runtime.SalesRequest{})
	if err != agent_runtime.ErrBridgeNotInitialized {
		t.Errorf("expected ErrBridgeNotInitialized, got %v", err)
	}
}

func TestConvertToDTOAgentContext_AllFields(t *testing.T) {
	ac := &agent_runtime.AgentContext{
		AgentID:             42,
		AgentCode:           "test_code",
		Name:                "测试智能体",
		AgentType:           "sales",
		Persona:             "友好专业",
		SystemPrompt:        "你是AI助手",
		Greeting:            "您好",
		RagProductIDs:       []string{"rag_001"},
		SOPIDs:              []string{"sop_001"},
		ScriptLibraryIDs:    []string{"script_001"},
		DecisionStrategyIDs: []string{"strategy_001"},
		ABExperimentIDs:     []string{"exp_001"},
		LLMModel:            "gpt-4o",
		Temperature:         0.8,
		MaxTokens:           1000,
		TopP:                0.95,
		EnableRAG:           true,
		RAGTopK:             5,
		ConfidenceThreshold: 0.85,
		MaxAIConsecutive:    3,
		Version:             2,
	}

	dtoAc := convertToDTOAgentContext(ac)

	if dtoAc.AgentID != 42 {
		t.Errorf("AgentID = %d, want 42", dtoAc.AgentID)
	}
	if dtoAc.AgentCode != "test_code" {
		t.Errorf("AgentCode = %s, want test_code", dtoAc.AgentCode)
	}
	if len(dtoAc.DecisionStrategyIDs) != 1 {
		t.Errorf("DecisionStrategyIDs length = %d, want 1", len(dtoAc.DecisionStrategyIDs))
	}
	if len(dtoAc.ABExperimentIDs) != 1 {
		t.Errorf("ABExperimentIDs length = %d, want 1", len(dtoAc.ABExperimentIDs))
	}
	if dtoAc.Version != 2 {
		t.Errorf("Version = %d, want 2", dtoAc.Version)
	}
}

func TestConvertToDTOAgentContext_NilInput(t *testing.T) {
	if convertToDTOAgentContext(nil) != nil {
		t.Error("nil input should return nil")
	}
}

func TestConvertToDTOSalesRequest_AllFields(t *testing.T) {
	req := &agent_runtime.SalesRequest{
		Channel:    "telegram",
		AccountID:  "tg_001",
		CustomerID: "cust_001",
		Content:    "用户消息",
		TraceID:    "trace_001",
		Raw:        map[string]any{"k": "v"},
	}
	ac := &dto.AgentContext{AgentID: 1}

	dtoReq := convertToDTOSalesRequest(req, ac)

	if dtoReq.CustomerID != "cust_001" {
		t.Errorf("CustomerID = %s, want cust_001", dtoReq.CustomerID)
	}
	if dtoReq.UserMessage != "用户消息" {
		t.Errorf("UserMessage = %s, want 用户消息", dtoReq.UserMessage)
	}
	if dtoReq.Platform != "telegram" {
		t.Errorf("Platform = %s, want telegram", dtoReq.Platform)
	}
	if !dtoReq.AutoExecute {
		t.Error("AutoExecute should be true")
	}
	if dtoReq.AgentContext != ac {
		t.Error("AgentContext not set")
	}
}

func TestConvertToDTOSalesRequest_NilInput(t *testing.T) {
	if convertToDTOSalesRequest(nil, nil) != nil {
		t.Error("nil input should return nil")
	}
}

func TestConvertFromDTOSalesResponse_AllFields(t *testing.T) {
	resp := &dto.SalesResponse{
		Reply:              "AI 回复",
		LLMModel:           "gpt-4o-mini",
		CostTokens:         150,
		TransferredToHuman: false,
		TransferReason:     "none",
	}
	ac := &agent_runtime.AgentContext{
		AgentID:   1,
		AgentCode: "test_agent",
	}
	req := &agent_runtime.SalesRequest{
		Channel:    "wecom",
		CustomerID: "cust_002",
		TraceID:    "trace_002",
	}

	result := convertFromDTOSalesResponse(resp, ac, req, 0)

	if result.ReplyContent != "AI 回复" {
		t.Errorf("ReplyContent = %s, want AI 回复", result.ReplyContent)
	}
	if result.AgentID != 1 {
		t.Errorf("AgentID = %d, want 1", result.AgentID)
	}
	if result.Channel != "wecom" {
		t.Errorf("Channel = %s, want wecom", result.Channel)
	}
	if result.TokensUsed != 150 {
		t.Errorf("TokensUsed = %d, want 150", result.TokensUsed)
	}
	if result.HandoffToHuman {
		t.Error("HandoffToHuman should be false")
	}
}

func TestConvertFromDTOSalesResponse_NilInput(t *testing.T) {
	if convertFromDTOSalesResponse(nil, &agent_runtime.AgentContext{}, &agent_runtime.SalesRequest{}, 0) != nil {
		t.Error("nil response should return nil")
	}
}

func TestConvertFromDTOSalesResponse_WithIntent(t *testing.T) {
	resp := &dto.SalesResponse{
		Reply: "回复",
		Intent: &dto.RecognizeResult{
			Confidence: 0.92,
		},
	}
	ac := &agent_runtime.AgentContext{}
	req := &agent_runtime.SalesRequest{}

	result := convertFromDTOSalesResponse(resp, ac, req, 0)
	if result.Confidence != 0.92 {
		t.Errorf("Confidence = %f, want 0.92", result.Confidence)
	}
}
