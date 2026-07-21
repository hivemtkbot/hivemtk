package confidence

// veto_rule_test.go 6 条一票否决规则单元测试
//
// 覆盖：
//  1. VetoComplaint：complaint / churn / 其他意图
//  2. VetoExplicit：中英文关键词 / 空消息 / 无关键词
//  3. VetoLoop：<3 轮 / 3 轮相同 / 3 轮不同
//  4. VetoLowEntity：expected 空 / 低 EntityComp / 高 EntityComp
//  5. VetoLowRAG：低 RAGQual / 高 RAGQual
//  6. VetoHighEntropy：低 LLMEntropy / 高 LLMEntropy
//  7. VetoChain：默认顺序 / 显式优先 / 无触发

import (
	"testing"

	"marketing/internal/dto"
)

// ============================================================================
// VetoComplaint 测试
// ============================================================================

func TestVetoComplaint_ComplaintIntent(t *testing.T) {
	r := &VetoComplaint{}
	ctx := &VetoContext{IntentType: "complaint"}
	triggered, reason := r.Check(&dto.FiveSignals{}, ctx)
	if !triggered || reason != "veto_complaint" {
		t.Errorf("complaint 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoComplaint_ChurnIntent(t *testing.T) {
	r := &VetoComplaint{}
	ctx := &VetoContext{IntentType: "churn"}
	triggered, reason := r.Check(&dto.FiveSignals{}, ctx)
	if !triggered || reason != "veto_complaint" {
		t.Errorf("churn 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoComplaint_OtherIntent(t *testing.T) {
	r := &VetoComplaint{}
	for _, intent := range []string{"ask_product", "price_inquiry", "greeting", "objection", "after_sale"} {
		ctx := &VetoContext{IntentType: intent}
		triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
		if triggered {
			t.Errorf("意图 %s 不应触发 veto_complaint", intent)
		}
	}
}

// ============================================================================
// VetoExplicit 测试
// ============================================================================

func TestVetoExplicit_ChineseKeywords(t *testing.T) {
	r := &VetoExplicit{}
	keywords := []string{"转人工", "人工客服", "找人工", "真人客服", "转接人工"}
	for _, kw := range keywords {
		ctx := &VetoContext{CustomerMessage: "我要" + kw}
		triggered, reason := r.Check(&dto.FiveSignals{}, ctx)
		if !triggered || reason != "veto_explicit" {
			t.Errorf("关键词 %q 应触发, got triggered=%v reason=%v", kw, triggered, reason)
		}
	}
}

func TestVetoExplicit_EnglishKeywords(t *testing.T) {
	r := &VetoExplicit{}
	keywords := []string{"real agent", "human agent", "transfer to human"}
	for _, kw := range keywords {
		ctx := &VetoContext{CustomerMessage: "I want " + kw}
		triggered, reason := r.Check(&dto.FiveSignals{}, ctx)
		if !triggered || reason != "veto_explicit" {
			t.Errorf("英文关键词 %q 应触发, got triggered=%v reason=%v", kw, triggered, reason)
		}
	}
}

func TestVetoExplicit_CaseInsensitive(t *testing.T) {
	r := &VetoExplicit{}
	ctx := &VetoContext{CustomerMessage: "I WANT HUMAN AGENT please"}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if !triggered {
		t.Errorf("大写英文关键词应触发（大小写不敏感）")
	}
}

func TestVetoExplicit_EmptyMessage(t *testing.T) {
	r := &VetoExplicit{}
	ctx := &VetoContext{CustomerMessage: ""}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Errorf("空消息不应触发")
	}
}

func TestVetoExplicit_NoKeyword(t *testing.T) {
	r := &VetoExplicit{}
	ctx := &VetoContext{CustomerMessage: "我想咨询一下产品价格"}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Errorf("无关键词消息不应触发")
	}
}

// ============================================================================
// VetoLoop 测试
// ============================================================================

func TestVetoLoop_LessThan3Turns(t *testing.T) {
	r := &VetoLoop{}
	ctx := &VetoContext{LastNTurns: []string{"hi", "hello", "hey"}}
	// 长度 < 6（< 3 轮 user+ai）
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Errorf("对话长度 < 6 不应触发")
	}
}

func TestVetoLoop_Exactly6TurnsDifferent(t *testing.T) {
	r := &VetoLoop{}
	ctx := &VetoContext{LastNTurns: []string{"q1", "a1", "q2", "a2", "q3", "a3"}}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Errorf("最后 3 条不同的消息不应触发")
	}
}

func TestVetoLoop_Last3Same(t *testing.T) {
	r := &VetoLoop{}
	// 最后 3 条相同（q3, a3, q3 - 用户重复提问）
	ctx := &VetoContext{LastNTurns: []string{"q1", "a1", "q2", "a2", "为什么", "为什么", "为什么"}}
	// 长度 7，最后 3 条相同
	triggered, reason := r.Check(&dto.FiveSignals{}, ctx)
	if !triggered || reason != "veto_loop" {
		t.Errorf("最后 3 条相同应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoLoop_Last3SameButEmpty(t *testing.T) {
	r := &VetoLoop{}
	// 最后 3 条都是空字符串
	ctx := &VetoContext{LastNTurns: []string{"q1", "a1", "q2", "a2", "", "", ""}}
	triggered, _ := r.Check(&dto.FiveSignals{}, ctx)
	if triggered {
		t.Errorf("最后 3 条空字符串不应触发")
	}
}

// ============================================================================
// VetoLowEntity 测试
// ============================================================================

func TestVetoLowEntity_EmptyExpected(t *testing.T) {
	r := &VetoLowEntity{Threshold: 0.2}
	ctx := &VetoContext{ExpectedEntities: nil}
	signals := &dto.FiveSignals{EntityComp: 0.0}
	triggered, _ := r.Check(signals, ctx)
	if triggered {
		t.Errorf("expected 为空（intent 不需要实体）不应触发")
	}
}

func TestVetoLowEntity_LowEntityComp(t *testing.T) {
	r := &VetoLowEntity{Threshold: 0.2}
	ctx := &VetoContext{ExpectedEntities: map[string]any{"product": "iPhone"}}
	signals := &dto.FiveSignals{EntityComp: 0.1} // < 0.2
	triggered, reason := r.Check(signals, ctx)
	if !triggered || reason != "veto_low_entity" {
		t.Errorf("EntityComp=0.1 < 0.2 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoLowEntity_HighEntityComp(t *testing.T) {
	r := &VetoLowEntity{Threshold: 0.2}
	ctx := &VetoContext{ExpectedEntities: map[string]any{"product": "iPhone"}}
	signals := &dto.FiveSignals{EntityComp: 0.5} // > 0.2
	triggered, _ := r.Check(signals, ctx)
	if triggered {
		t.Errorf("EntityComp=0.5 > 0.2 不应触发")
	}
}

func TestVetoLowEntity_DefaultThreshold(t *testing.T) {
	r := &VetoLowEntity{Threshold: 0} // 0 应被替换为 0.2
	ctx := &VetoContext{ExpectedEntities: map[string]any{"k": "v"}}
	signals := &dto.FiveSignals{EntityComp: 0.15} // < 0.2
	triggered, _ := r.Check(signals, ctx)
	if !triggered {
		t.Errorf("Threshold=0 应默认 0.2, EntityComp=0.15 应触发")
	}
}

func TestVetoLowEntity_BoundaryEqual(t *testing.T) {
	r := &VetoLowEntity{Threshold: 0.2}
	ctx := &VetoContext{ExpectedEntities: map[string]any{"k": "v"}}
	signals := &dto.FiveSignals{EntityComp: 0.2} // = 阈值，< 为严格小于
	triggered, _ := r.Check(signals, ctx)
	if triggered {
		t.Errorf("EntityComp=0.2 等于阈值不应触发（< 严格小于）")
	}
}

// ============================================================================
// VetoLowRAG 测试
// ============================================================================

func TestVetoLowRAG_LowRAGQual(t *testing.T) {
	r := &VetoLowRAG{Threshold: 0.1}
	signals := &dto.FiveSignals{RAGQual: 0.05} // < 0.1
	triggered, reason := r.Check(signals, &VetoContext{})
	if !triggered || reason != "veto_low_rag" {
		t.Errorf("RAGQual=0.05 < 0.1 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoLowRAG_HighRAGQual(t *testing.T) {
	r := &VetoLowRAG{Threshold: 0.1}
	signals := &dto.FiveSignals{RAGQual: 0.5} // > 0.1
	triggered, _ := r.Check(signals, &VetoContext{})
	if triggered {
		t.Errorf("RAGQual=0.5 > 0.1 不应触发")
	}
}

func TestVetoLowRAG_DefaultThreshold(t *testing.T) {
	r := &VetoLowRAG{Threshold: 0} // 默认 0.1
	signals := &dto.FiveSignals{RAGQual: 0.05}
	triggered, _ := r.Check(signals, &VetoContext{})
	if !triggered {
		t.Errorf("Threshold=0 应默认 0.1, RAGQual=0.05 应触发")
	}
}

// ============================================================================
// VetoHighEntropy 测试
// ============================================================================

func TestVetoHighEntropy_LowLLMEntropy(t *testing.T) {
	r := &VetoHighEntropy{Threshold: 0.2}
	signals := &dto.FiveSignals{LLMEntropy: 0.1} // < 0.2
	triggered, reason := r.Check(signals, &VetoContext{})
	if !triggered || reason != "veto_high_entropy" {
		t.Errorf("LLMEntropy=0.1 < 0.2 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoHighEntropy_HighLLMEntropy(t *testing.T) {
	r := &VetoHighEntropy{Threshold: 0.2}
	signals := &dto.FiveSignals{LLMEntropy: 0.7} // > 0.2
	triggered, _ := r.Check(signals, &VetoContext{})
	if triggered {
		t.Errorf("LLMEntropy=0.7 > 0.2 不应触发")
	}
}

func TestVetoHighEntropy_DefaultThreshold(t *testing.T) {
	r := &VetoHighEntropy{Threshold: 0}
	signals := &dto.FiveSignals{LLMEntropy: 0.15}
	triggered, _ := r.Check(signals, &VetoContext{})
	if !triggered {
		t.Errorf("Threshold=0 应默认 0.2, LLMEntropy=0.15 应触发")
	}
}

// ============================================================================
// VetoChain 测试
// ============================================================================

func TestVetoChain_DefaultOrder(t *testing.T) {
	c := NewVetoChain()
	rules := c.Rules()
	if len(rules) != 6 {
		t.Errorf("默认 VetoChain 应有 6 条规则, got %d", len(rules))
	}
	// 验证顺序：Explicit, Complaint, Loop, LowEntity, LowRAG, HighEntropy
	if _, ok := rules[0].(*VetoExplicit); !ok {
		t.Errorf("规则 0 应为 VetoExplicit, got %T", rules[0])
	}
	if _, ok := rules[1].(*VetoComplaint); !ok {
		t.Errorf("规则 1 应为 VetoComplaint, got %T", rules[1])
	}
	if _, ok := rules[2].(*VetoLoop); !ok {
		t.Errorf("规则 2 应为 VetoLoop, got %T", rules[2])
	}
	if _, ok := rules[3].(*VetoLowEntity); !ok {
		t.Errorf("规则 3 应为 VetoLowEntity, got %T", rules[3])
	}
	if _, ok := rules[4].(*VetoLowRAG); !ok {
		t.Errorf("规则 4 应为 VetoLowRAG, got %T", rules[4])
	}
	if _, ok := rules[5].(*VetoHighEntropy); !ok {
		t.Errorf("规则 5 应为 VetoHighEntropy, got %T", rules[5])
	}
}

func TestVetoChain_ExplicitPriority(t *testing.T) {
	// 同时满足 Explicit + Complaint + LowEntity + LowRAG + HighEntropy
	// 应返回 veto_explicit（优先级最高）
	c := NewVetoChain()
	signals := &dto.FiveSignals{EntityComp: 0.0, RAGQual: 0.0, LLMEntropy: 0.0}
	ctx := &VetoContext{
		IntentType:       "complaint",
		CustomerMessage:  "我要转人工",
		ExpectedEntities: map[string]any{"k": "v"},
		LastNTurns:       []string{"q", "a", "q", "a", "q", "q", "q"},
	}
	triggered, reason := c.Check(signals, ctx)
	if !triggered || reason != "veto_explicit" {
		t.Errorf("Explicit 应优先级最高, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoChain_ComplaintSecondPriority(t *testing.T) {
	// 满足 Complaint + LowEntity + LowRAG + HighEntropy（不含 Explicit）
	c := NewVetoChain()
	signals := &dto.FiveSignals{EntityComp: 0.0, RAGQual: 0.0, LLMEntropy: 0.0}
	ctx := &VetoContext{
		IntentType:       "complaint",
		CustomerMessage:  "我要投诉",
		ExpectedEntities: map[string]any{"k": "v"},
	}
	triggered, reason := c.Check(signals, ctx)
	if !triggered || reason != "veto_complaint" {
		t.Errorf("Complaint 应为第二优先级, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoChain_NoTrigger(t *testing.T) {
	c := NewVetoChain()
	signals := &dto.FiveSignals{
		IntentConf: 0.9, EntityComp: 0.8, CtxRelev: 0.7, RAGQual: 0.6, LLMEntropy: 0.8,
	}
	ctx := &VetoContext{
		IntentType:      "ask_product",
		CustomerMessage: "请问这个产品的价格是多少",
		LastNTurns:      []string{"q1", "a1", "q2", "a2", "q3", "a3"},
	}
	triggered, reason := c.Check(signals, ctx)
	if triggered {
		t.Errorf("无触发条件时不应触发, got reason=%v", reason)
	}
}

func TestVetoChain_OnlyLowEntity(t *testing.T) {
	c := NewVetoChain()
	signals := &dto.FiveSignals{
		IntentConf: 0.9, EntityComp: 0.1, CtxRelev: 0.9, RAGQual: 0.8, LLMEntropy: 0.9,
	}
	ctx := &VetoContext{
		IntentType:       "ask_product",
		CustomerMessage:  "我要买产品",
		ExpectedEntities: map[string]any{"product": "iPhone"},
	}
	triggered, reason := c.Check(signals, ctx)
	if !triggered || reason != "veto_low_entity" {
		t.Errorf("仅 LowEntity 满足时应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoChain_CustomRules(t *testing.T) {
	// 自定义规则链
	customRules := []VetoRule{
		&VetoLowRAG{Threshold: 0.5},
		&VetoLowEntity{Threshold: 0.5},
	}
	c := NewVetoChainWithRules(customRules)
	rules := c.Rules()
	if len(rules) != 2 {
		t.Errorf("自定义规则链应有 2 条规则, got %d", len(rules))
	}
	// 第一个规则触发
	signals := &dto.FiveSignals{RAGQual: 0.3} // < 0.5
	ctx := &VetoContext{ExpectedEntities: map[string]any{"k": "v"}}
	triggered, reason := c.Check(signals, ctx)
	if !triggered || reason != "veto_low_rag" {
		t.Errorf("自定义规则 LowRAG 应触发, got triggered=%v reason=%v", triggered, reason)
	}
}

func TestVetoChain_EmptyRules(t *testing.T) {
	c := NewVetoChainWithRules(nil)
	triggered, _ := c.Check(&dto.FiveSignals{}, &VetoContext{})
	if triggered {
		t.Errorf("空规则链不应触发")
	}
}
