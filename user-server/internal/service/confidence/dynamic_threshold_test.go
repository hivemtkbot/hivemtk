package confidence


import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// makePolicyEngine 构造内存策略引擎（不依赖 DB）
func makePolicyEngine() *ThresholdPolicyEngine {
	e := NewThresholdPolicyEngine(nil)
	e.loadDefaults()
	return e
}


func TestDynamicThreshold_Calculate_NilPolicyEngine(t *testing.T) {
	c := NewDynamicThresholdCalculator(nil)
	got := c.Calculate(&ThresholdInput{IntentType: "ask_product"})
	if !approxEqual(got, 0.70) {
		t.Errorf("nil 策略引擎应兜底 0.70, got %v", got)
	}
}

func TestDynamicThreshold_Calculate_DefaultPolicy(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		CustomerLevel:     "normal",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.70) {
		t.Errorf("default 策略无调整因子应=0.70, got %v", got)
	}
}

func TestDynamicThreshold_Calculate_VIPCustomer(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		CustomerLevel:     "vip",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.75) {
		t.Errorf("VIP 客户应 +0.05, got %v want 0.75", got)
	}
}

func TestDynamicThreshold_Calculate_LowCustomer(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		CustomerLevel:     "low",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.65) {
		t.Errorf("low 客户应 -0.05, got %v want 0.65", got)
	}
}

func TestDynamicThreshold_Calculate_PeakHour(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.65) {
		t.Errorf("高峰时段应 -0.05, got %v want 0.65", got)
	}
	got = c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 15, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.65) {
		t.Errorf("高峰时段 14-16 应 -0.05, got %v want 0.65", got)
	}
}

func TestDynamicThreshold_Calculate_LowHour(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		AgentAvailability: 0.3,
		Now:               time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.75) {
		t.Errorf("低谷时段应 +0.05, got %v want 0.75", got)
	}
}

func TestDynamicThreshold_Calculate_HighAvailability(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		AgentAvailability: 0.6,
		Now:               time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.60) {
		t.Errorf("高空闲应 -0.10, got %v want 0.60", got)
	}
}

func TestDynamicThreshold_Calculate_LowAvailability(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "unknown_intent",
		AgentAvailability: 0.05,
		Now:               time.Date(2026, 7, 19, 9, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.80) {
		t.Errorf("空闲不足应 +0.10, got %v want 0.80", got)
	}
}

func TestDynamicThreshold_Calculate_ClipMin(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "social",
		CustomerLevel:     "low",
		AgentAvailability: 0.6,
		Now:               time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.40) {
		t.Errorf("应 clip 到 0.40, got %v", got)
	}
}

func TestDynamicThreshold_Calculate_ClipMax(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "complaint",
		CustomerLevel:     "vip",
		AgentAvailability: 0.05,
		Now:               time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.95) {
		t.Errorf("应 clip 到 0.95, got %v", got)
	}
}

func TestDynamicThreshold_Calculate_AllFactorsCombined(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	got := c.Calculate(&ThresholdInput{
		IntentType:        "ask_product",
		CustomerLevel:     "vip",
		AgentAvailability: 0.7,
		Now:               time.Date(2026, 7, 19, 3, 0, 0, 0, time.UTC),
	})
	if !approxEqual(got, 0.70) {
		t.Errorf("VIP+低谷+高空闲应相互抵消=0.70, got %v", got)
	}
}


func TestDynamicThreshold_DetermineBand_Handoff(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.30, 0.70, policy)
	if got != dto.BandHandoff {
		t.Errorf("conf=0.30 应为 handoff, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_LLMFallback(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.50, 0.70, policy)
	if got != dto.BandLLMFallback {
		t.Errorf("conf=0.50 应为 llm_fallback, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_Review(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.70, 0.70, policy)
	if got != dto.BandReview {
		t.Errorf("conf=0.70 应为 review, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_Auto(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.80, 0.70, policy)
	if got != dto.BandAuto {
		t.Errorf("conf=0.80 应为 auto, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_BoundaryHandoff(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.40, 0.70, policy)
	if got != dto.BandLLMFallback {
		t.Errorf("conf=0.40 边界应为 llm_fallback, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_BoundaryReview(t *testing.T) {
	e := makePolicyEngine()
	c := NewDynamicThresholdCalculator(e)
	policy := e.GetPolicy("default")
	got := c.DetermineBand(0.75, 0.70, policy)
	if got != dto.BandAuto {
		t.Errorf("conf=0.75 边界应为 auto, got %v", got)
	}
}

func TestDynamicThreshold_DetermineBand_NilPolicy(t *testing.T) {
	c := NewDynamicThresholdCalculator(nil)
	if got := c.DetermineBand(0.30, 0.70, nil); got != dto.BandHandoff {
		t.Errorf("nil policy conf=0.30 应为 handoff, got %v", got)
	}
	if got := c.DetermineBand(0.50, 0.70, nil); got != dto.BandLLMFallback {
		t.Errorf("nil policy conf=0.50 应为 llm_fallback, got %v", got)
	}
	if got := c.DetermineBand(0.70, 0.70, nil); got != dto.BandReview {
		t.Errorf("nil policy conf=0.70 应为 review, got %v", got)
	}
	if got := c.DetermineBand(0.80, 0.70, nil); got != dto.BandAuto {
		t.Errorf("nil policy conf=0.80 应为 auto, got %v", got)
	}
}


func TestThresholdPolicyEngine_LoadPolicies_NilRepo(t *testing.T) {
	e := NewThresholdPolicyEngine(nil)
	if err := e.LoadPolicies(context.Background()); err != nil {
		t.Errorf("nil repo LoadPolicies 应返回 nil, got %v", err)
	}
	p := e.GetPolicy("complaint")
	if p == nil {
		t.Fatalf("complaint 策略不应为 nil")
	}
	if !approxEqual(p.BaseThreshold, 0.85) {
		t.Errorf("complaint base=0.85, got %v", p.BaseThreshold)
	}
}

func TestThresholdPolicyEngine_GetPolicy_NotFound(t *testing.T) {
	e := makePolicyEngine()
	p := e.GetPolicy("unknown_intent_xyz")
	if p == nil {
		t.Fatalf("未知 intent 不应返回 nil")
	}
	if !approxEqual(p.BaseThreshold, 0.70) {
		t.Errorf("未知 intent 应返回 default 0.70, got %v", p.BaseThreshold)
	}
}

func TestThresholdPolicyEngine_GetPolicy_SpecificIntent(t *testing.T) {
	e := makePolicyEngine()
	cases := []struct {
		intent string
		base   float64
	}{
		{"complaint", 0.85},
		{"churn", 0.85},
		{"objection", 0.75},
		{"ask_product", 0.70},
		{"ask_service", 0.70},
		{"price_inquiry", 0.65},
		{"purchase", 0.65},
		{"after_sale", 0.80},
		{"social", 0.50},
		{"greeting", 0.50},
		{"default", 0.70},
	}
	for _, tc := range cases {
		p := e.GetPolicy(tc.intent)
		if p == nil {
			t.Errorf("intent %s 不应返回 nil", tc.intent)
			continue
		}
		if !approxEqual(p.BaseThreshold, tc.base) {
			t.Errorf("intent %s base=%v want %v", tc.intent, p.BaseThreshold, tc.base)
		}
	}
}

func TestThresholdPolicyEngine_UpdatePolicy(t *testing.T) {
	e := makePolicyEngine()
	newPolicy := &model.ThresholdPolicy{
		PolicyID:      "policy_complaint_v2",
		IntentType:    "complaint",
		BaseThreshold: 0.90, 
		IsActive:      true,
	}
	if err := e.UpdatePolicy(context.Background(), newPolicy); err != nil {
		t.Fatalf("UpdatePolicy 失败: %v", err)
	}
	p := e.GetPolicy("complaint")
	if !approxEqual(p.BaseThreshold, 0.90) {
		t.Errorf("更新后 complaint base=0.90, got %v", p.BaseThreshold)
	}
}

func TestThresholdPolicyEngine_AllPolicies(t *testing.T) {
	e := makePolicyEngine()
	all := e.AllPolicies()
	if len(all) < 10 {
		t.Errorf("默认策略应至少 10 条, got %d", len(all))
	}
}

func TestThresholdPolicyEngine_PolicyFields(t *testing.T) {
	e := makePolicyEngine()
	p := e.GetPolicy("default")
	if p.CustomerLevelWeight != 0.05 {
		t.Errorf("CustomerLevelWeight=0.05, got %v", p.CustomerLevelWeight)
	}
	if p.TimeslotWeight != 0.05 {
		t.Errorf("TimeslotWeight=0.05, got %v", p.TimeslotWeight)
	}
	if p.AgentAvailabilityWeight != 0.10 {
		t.Errorf("AgentAvailabilityWeight=0.10, got %v", p.AgentAvailabilityWeight)
	}
	if !approxEqual(p.BandHandoffUpper, 0.40) {
		t.Errorf("BandHandoffUpper=0.40, got %v", p.BandHandoffUpper)
	}
	if !approxEqual(p.BandFallbackUpper, 0.60) {
		t.Errorf("BandFallbackUpper=0.60, got %v", p.BandFallbackUpper)
	}
	if !approxEqual(p.BandReviewUpper, 0.75) {
		t.Errorf("BandReviewUpper=0.75, got %v", p.BandReviewUpper)
	}
}

