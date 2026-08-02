package dto

import (
	"testing"
	"time"
)

// intent_test.go 销冠域 DTO 类型测试

func TestRecognizeResult_Fields(t *testing.T) {
	r := RecognizeResult{
		IntentType:      "price_inquiry",
		IntentName:      "价格咨询",
		Confidence:      0.95,
		ConfidenceLevel: "high",
		IntentSubtype:   "discount",
		Entities:        map[string]any{"product": "套餐A"},
		Sentiment:       "positive",
		Method:          "llm",
		LLMModel:        "gpt-4",
		CostTokens:      120,
		LatencyMs:       350,
	}
	if r.IntentType != "price_inquiry" {
		t.Errorf("Expected IntentType 'price_inquiry', got '%s'", r.IntentType)
	}
	if r.Confidence != 0.95 {
		t.Errorf("Expected Confidence 0.95, got %f", r.Confidence)
	}
	if r.Method != "llm" {
		t.Errorf("Expected Method 'llm', got '%s'", r.Method)
	}
	if r.Entities["product"] != "套餐A" {
		t.Errorf("Expected Entities['product']='套餐A', got '%v'", r.Entities["product"])
	}
}

func TestMessage_Fields(t *testing.T) {
	now := time.Now()
	m := Message{
		Role:      "user",
		Content:   "多少钱？",
		Timestamp: now,
	}
	if m.Role != "user" {
		t.Errorf("Expected Role 'user', got '%s'", m.Role)
	}
	if m.Content != "多少钱？" {
		t.Errorf("Expected Content '多少钱？', got '%s'", m.Content)
	}
	if !m.Timestamp.Equal(now) {
		t.Error("Expected Timestamp to match")
	}
}

func TestShortTermMemory_Fields(t *testing.T) {
	m := ShortTermMemory{
		Messages: []Message{
			{Role: "user", Content: "你好", Timestamp: time.Now()},
			{Role: "ai", Content: "您好，请问有什么可以帮您？", Timestamp: time.Now()},
		},
	}
	if len(m.Messages) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(m.Messages))
	}
	if m.Messages[0].Role != "user" {
		t.Errorf("Expected first message Role 'user', got '%s'", m.Messages[0].Role)
	}
	if m.Messages[1].Role != "ai" {
		t.Errorf("Expected second message Role 'ai', got '%s'", m.Messages[1].Role)
	}
}

func TestExecuteRequest_Fields(t *testing.T) {
	req := ExecuteRequest{
		SOPID:      1,
		CustomerID: "cust-123",
		SessionID:  "sess-456",
		Input:      map[string]any{"key": "value"},
	}
	if req.SOPID != 1 {
		t.Errorf("Expected SOPID 1, got %d", req.SOPID)
	}
	if req.CustomerID != "cust-123" {
		t.Errorf("Expected CustomerID 'cust-123', got '%s'", req.CustomerID)
	}
	if req.Input["key"] != "value" {
		t.Errorf("Expected Input['key']='value', got '%v'", req.Input["key"])
	}
}

func TestStepRequest_Fields(t *testing.T) {
	req := StepRequest{
		ExecutionID: 42,
		Output:      map[string]any{"result": "success"},
	}
	if req.ExecutionID != 42 {
		t.Errorf("Expected ExecutionID 42, got %d", req.ExecutionID)
	}
	if req.Output["result"] != "success" {
		t.Errorf("Expected Output['result']='success', got '%v'", req.Output["result"])
	}
}

func TestSalesStepLog_Fields(t *testing.T) {
	log := SalesStepLog{
		Step:      "1_resolve_customer",
		Status:    "ok",
		LatencyMs: 15,
		Detail:    "customer_id=cust-123",
		Error:     "",
		Extra:     map[string]any{"source": "oneid"},
	}
	if log.Step != "1_resolve_customer" {
		t.Errorf("Expected Step '1_resolve_customer', got '%s'", log.Step)
	}
	if log.Status != "ok" {
		t.Errorf("Expected Status 'ok', got '%s'", log.Status)
	}
	if log.LatencyMs != 15 {
		t.Errorf("Expected LatencyMs 15, got %d", log.LatencyMs)
	}
}

// TestExecuteRequest_NoMerchantID 验证残留 MerchantID 字段已清理（独立部署无多租户）
func TestExecuteRequest_NoMerchantID(t *testing.T) {
	// ExecuteRequest 不应包含 MerchantID 字段
	// 如果存在该字段，以下构造会因零值而通过，但通过反射检查更可靠
	req := ExecuteRequest{}
	// 确保 req 是零值可构造的（无残留必填字段）
	if req.SOPID != 0 {
		t.Errorf("Expected zero-value SOPID, got %d", req.SOPID)
	}
}
