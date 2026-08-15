package agent_runtime

import (
	"testing"
)

// TestDefaultIntentExtractor_CodeBlock 从 ```json {...} ``` 代码块提取
func TestDefaultIntentExtractor_CodeBlock(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "您好，我们的产品价格是 199 元，欢迎咨询。\n\n```json\n{\"intent\":\"lead_capture\",\"captured_data\":{\"product\":\"会员套餐\",\"whatsapp\":\"+86 138 0000 0000\"}}\n```"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.Intent != "lead_capture" {
		t.Errorf("intent = %s, want lead_capture", ir.Intent)
	}
	if ir.CapturedData["product"] != "会员套餐" {
		t.Errorf("captured product = %s, want 会员套餐", ir.CapturedData["product"])
	}
	if ir.CapturedData["whatsapp"] != "+86 138 0000 0000" {
		t.Errorf("captured whatsapp = %s, want +86 138 0000 0000", ir.CapturedData["whatsapp"])
	}
	if ir.RawJSON == "" {
		t.Error("raw_json should not be empty")
	}
}

// TestDefaultIntentExtractor_BareJSON 从裸 {"intent":...} 段提取
func TestDefaultIntentExtractor_BareJSON(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "您好，已为您查询到订单状态。\n{\"intent\":\"faq\",\"captured_data\":{}}"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.Intent != "faq" {
		t.Errorf("intent = %s, want faq", ir.Intent)
	}
}

// TestDefaultIntentExtractor_HumanTransfer 触发转人工
func TestDefaultIntentExtractor_HumanTransfer(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "抱歉给您带来不便，已为您升级人工客服。\n\n```json\n{\"intent\":\"human_transfer\",\"captured_data\":{\"reason\":\"客户投诉\"}}\n```"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.Intent != "human_transfer" {
		t.Errorf("intent = %s, want human_transfer", ir.Intent)
	}
	if ir.CapturedData["reason"] != "客户投诉" {
		t.Errorf("reason = %s, want 客户投诉", ir.CapturedData["reason"])
	}
}

// TestDefaultIntentExtractor_NoJSON 找不到 JSON 块时返回 nil
func TestDefaultIntentExtractor_NoJSON(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "您好，这是一段没有 JSON 块的纯文本回复。"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir != nil {
		t.Errorf("expected nil BusinessIntentResult, got %+v", ir)
	}
}

// TestDefaultIntentExtractor_EmptyReply 空回复
func TestDefaultIntentExtractor_EmptyReply(t *testing.T) {
	ext := DefaultIntentExtractor{}
	ir, err := ext.Extract("")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir != nil {
		t.Errorf("expected nil for empty reply, got %+v", ir)
	}
}

// TestDefaultIntentExtractor_MultipleBlocks 取最后一个代码块
func TestDefaultIntentExtractor_MultipleBlocks(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "第一条回复。\n\n```json\n{\"intent\":\"faq\",\"captured_data\":{}}\n```\n\n中间一些文字。\n\n```json\n{\"intent\":\"lead_capture\",\"captured_data\":{\"email\":\"a@b.com\"}}\n```"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.Intent != "lead_capture" {
		t.Errorf("intent = %s, want lead_capture (last block)", ir.Intent)
	}
	if ir.CapturedData["email"] != "a@b.com" {
		t.Errorf("email = %s, want a@b.com", ir.CapturedData["email"])
	}
}

// TestDefaultIntentExtractor_MissingIntent intent 缺失时默认 faq
func TestDefaultIntentExtractor_MissingIntent(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "回复。\n```json\n{\"captured_data\":{\"product\":\"x\"}}\n```"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.Intent != "faq" {
		t.Errorf("intent = %s, want faq (default)", ir.Intent)
	}
}

// TestDefaultIntentExtractor_NumberCapturedData 数字类型 captured_data 字段
func TestDefaultIntentExtractor_NumberCapturedData(t *testing.T) {
	ext := DefaultIntentExtractor{}
	reply := "回复。\n```json\n{\"intent\":\"lead_capture\",\"captured_data\":{\"quantity\":3}}\n```"
	ir, err := ext.Extract(reply)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if ir == nil {
		t.Fatal("expected BusinessIntentResult, got nil")
	}
	if ir.CapturedData["quantity"] != "3" {
		t.Errorf("quantity = %s, want 3", ir.CapturedData["quantity"])
	}
}

