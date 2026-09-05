package ragretrieval

import (
	"context"
	"errors"
	"testing"
)

func TestKeyIntentConstants(t *testing.T) {
	if len(AllKeyIntents) != 7 {
		t.Fatalf("expected 7 key intents, got %d", len(AllKeyIntents))
	}
	for _, k := range AllKeyIntents {
		if KeyIntentDescription[k] == "" {
			t.Errorf("missing description for %s", k)
		}
	}
}

func TestClassifyKeyIntent_Empty(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "", nil)
	if res.Confidence != 0 {
		t.Errorf("expected confidence 0 for empty, got %.2f", res.Confidence)
	}
	if res.Method != "rule" {
		t.Errorf("expected method rule, got %s", res.Method)
	}
}

// 1. 价格异议
func TestClassifyKeyIntent_PriceObjection(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "这个太贵了，我买不起", nil)
	if res.Intent != KeyIntentPriceObjection {
		t.Errorf("expected price_objection, got %s", res.Intent)
	}
	if res.Confidence <= 0.6 {
		t.Errorf("expected confidence > 0.6, got %.2f", res.Confidence)
	}
	if res.Method != "rule" {
		t.Errorf("expected method rule, got %s", res.Method)
	}
}

// 2. 质量异议
func TestClassifyKeyIntent_QualityObjection(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "你们这个质量太差了，假货吧", nil)
	if res.Intent != KeyIntentQualityObjection {
		t.Errorf("expected quality_objection, got %s", res.Intent)
	}
	if res.Evidence == "" {
		t.Error("expected evidence for quality_objection")
	}
}

// 3. 购买意向
func TestClassifyKeyIntent_PurchaseIntent(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "好的，我要了，怎么付款", nil)
	if res.Intent != KeyIntentPurchaseIntent {
		t.Errorf("expected purchase_intent, got %s", res.Intent)
	}
}

// 4. 信任异议
func TestClassifyKeyIntent_TrustObjection(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "你们靠谱吗？不会是骗子吧", nil)
	if res.Intent != KeyIntentTrustObjection {
		t.Errorf("expected trust_objection, got %s", res.Intent)
	}
}

// 5. 竞品异议
func TestClassifyKeyIntent_CompetitorObjection(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "别家比你们便宜多了，我要看看其他品牌", nil)
	if res.Intent != KeyIntentCompetitorObjection {
		t.Errorf("expected competitor_objection, got %s", res.Intent)
	}
}

// 6. 折扣请求
func TestClassifyKeyIntent_DiscountRequest(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "有什么优惠活动吗？能便宜点吗", nil)
	if res.Intent != KeyIntentDiscountRequest {
		t.Errorf("expected discount_request, got %s", res.Intent)
	}
}

// 7. 退款请求
func TestClassifyKeyIntent_RefundRequest(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "我要退款，不想要了", nil)
	if res.Intent != KeyIntentRefundRequest {
		t.Errorf("expected refund_request, got %s", res.Intent)
	}
}

// 规则未命中（无关键词的随机文本）
func TestClassifyKeyIntent_NoRuleMatch_NoLLM(t *testing.T) {
	res := ClassifyKeyIntent(context.Background(), "今天天气真好", nil)
	if res.Method != "rule" {
		t.Errorf("expected method rule, got %s", res.Method)
	}
	if res.Confidence != 0.4 {
		t.Errorf("expected confidence 0.4, got %.2f", res.Confidence)
	}
}

// 规则未命中 + LLM 命中
func TestClassifyKeyIntent_LLMHit(t *testing.T) {
	chat := &mockLLMChatClient{
		resp: `{"intent":"purchase_intent","confidence":0.88,"evidence":"感兴趣","reasoning":"客户表达购买意愿"}`,
	}
	res := ClassifyKeyIntent(context.Background(), "我想了解更多细节", chat)
	if res.Method != "llm" {
		t.Errorf("expected method llm, got %s", res.Method)
	}
	if res.Intent != KeyIntentPurchaseIntent {
		t.Errorf("expected purchase_intent, got %s", res.Intent)
	}
	if res.Confidence < 0.8 {
		t.Errorf("expected confidence >= 0.8, got %.2f", res.Confidence)
	}
}

// LLM 返回非法 intent → 降级
func TestClassifyKeyIntent_LLMInvalidIntent(t *testing.T) {
	chat := &mockLLMChatClient{
		resp: `{"intent":"unknown_intent","confidence":0.7}`,
	}
	res := ClassifyKeyIntent(context.Background(), "无关键词文本", chat)
	if res.Method != "rule" {
		t.Errorf("expected method rule fallback, got %s", res.Method)
	}
}

// LLM 返回非 JSON → 降级
func TestClassifyKeyIntent_LLMInvalidJSON(t *testing.T) {
	chat := &mockLLMChatClient{
		resp: `not a json`,
	}
	res := ClassifyKeyIntent(context.Background(), "无关键词文本", chat)
	if res.Method != "rule" {
		t.Errorf("expected method rule fallback, got %s", res.Method)
	}
}

// LLM 调用失败 → 降级
func TestClassifyKeyIntent_LLMError(t *testing.T) {
	chat := &mockLLMChatClient{err: errors.New("LLM unavailable")}
	res := ClassifyKeyIntent(context.Background(), "无关键词文本", chat)
	if res.Method != "rule" {
		t.Errorf("expected method rule fallback, got %s", res.Method)
	}
}

func TestIsKeyIntent(t *testing.T) {
	if !IsKeyIntent(context.Background(), "太贵了", KeyIntentPriceObjection, nil) {
		t.Error("expected price_objection match")
	}
	if IsKeyIntent(context.Background(), "太贵了", KeyIntentPurchaseIntent, nil) {
		t.Error("expected not match purchase_intent")
	}
	if IsKeyIntent(context.Background(), "今天天气好", KeyIntentPurchaseIntent, nil) {
		t.Error("expected false when no rule match and no chat")
	}
	chat := &mockLLMChatClient{resp: `{"intent":"purchase_intent","confidence":0.9}`}
	if !IsKeyIntent(context.Background(), "无关键词文本", KeyIntentPurchaseIntent, chat) {
		t.Error("expected purchase_intent via LLM")
	}
}

func TestExtractKeyIntents(t *testing.T) {
	texts := []string{
		"太贵了",
		"我要退款",
		"好的要了",
	}
	results := ExtractKeyIntents(context.Background(), texts, nil)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].Intent != KeyIntentPriceObjection {
		t.Errorf("[0] expected price_objection, got %s", results[0].Intent)
	}
	if results[1].Intent != KeyIntentRefundRequest {
		t.Errorf("[1] expected refund_request, got %s", results[1].Intent)
	}
	if results[2].Intent != KeyIntentPurchaseIntent {
		t.Errorf("[2] expected purchase_intent, got %s", results[2].Intent)
	}
}

func TestIsValidKeyIntent(t *testing.T) {
	if !isValidKeyIntent(KeyIntentPriceObjection) {
		t.Error("expected valid key intent")
	}
	if isValidKeyIntent(KeyIntentType("invalid")) {
		t.Error("expected invalid key intent")
	}
}

func TestExtractJSONFromStr(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{`{"a":1}`, `{"a":1}`},
		{`prefix {"a":1} suffix`, `{"a":1}`},
		{`{"a":{"b":2}}`, `{"a":{"b":2}}`},
		{`not json`, `not json`},
		{``, ``},
	}
	for _, c := range cases {
		if got := extractJSONFromStr(c.input); got != c.expected {
			t.Errorf("extractJSONFromStr(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestMatchKeyIntentByRule_NoMatch(t *testing.T) {
	res := matchKeyIntentByRule("今天天气不错")
	if res != nil {
		t.Errorf("expected nil for non-key text, got %+v", res)
	}
}

func TestMatchKeyIntentByRule_HighestScore(t *testing.T) {
	res := matchKeyIntentByRule("我不放心，这家店骗人")
	if res == nil {
		t.Fatal("expected match")
	}
	if res.Intent != KeyIntentTrustObjection {
		t.Errorf("expected trust_objection, got %s", res.Intent)
	}
}
