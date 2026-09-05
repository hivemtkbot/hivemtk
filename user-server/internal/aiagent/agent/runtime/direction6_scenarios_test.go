package agent_runtime

import (
	"context"
	"strings"
	"testing"
)

// TestDirection6_Scenario1_CalmFAQ 场景1: 平静 + FAQ 问答 → AI 正常处理
func TestDirection6_Scenario1_CalmFAQ(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "d6_s1",
		Content:     "这个产品怎么用？",
		TraceID:     "d6-s1",
	}, &AgentContext{AgentCode: "default", EnableRAG: true})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision.HandoffToHuman {
		t.Errorf("场景1 不应转人工，实际 HandoffToHuman=true (Reason=%s)", decision.HandoffReason)
	}
	if decision.Plan != nil {
		if !strings.Contains(string(decision.Plan.PlanType), "customer_service") && decision.Plan.PlanType != "faq" {
			t.Logf("场景1 PlanType=%s (可接受)", decision.Plan.PlanType)
		}
	}
	if decision.Crisis.Level >= CrisisHigh {
		t.Errorf("场景1 危机等级应较低，实际 %d", decision.Crisis.Level)
	}
	t.Logf("场景1: crisis=%d handoff=%v", decision.Crisis.Level, decision.HandoffToHuman)
}

// TestDirection6_Scenario2_AnxiousOrder 场景2: 焦虑 + 售后查单 → 工具触发 Planner
func TestDirection6_Scenario2_AnxiousOrder(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "d6_s2",
		Content:     "我等了3天了，怎么还没发货?",
		TraceID:     "d6-s2",
	}, &AgentContext{AgentCode: "default", EnableRAG: true})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision.HandoffToHuman {
		t.Errorf("场景2 不应转人工，实际 HandoffToHuman=true")
	}
	if decision.Plan == nil {
		t.Fatal("场景2 期望有 Plan")
	}
	hasOrderTool := false
	for _, tc := range decision.Plan.ToolCalls {
		if strings.Contains(tc.ToolName, "order") || strings.Contains(tc.ToolName, "query") {
			hasOrderTool = true
		}
	}
	if !hasOrderTool {
		t.Logf("场景2 期望有 order 查询工具，实际 tool calls=%+v", decision.Plan.ToolCalls)
	}
	if decision.Crisis.Level > CrisisMedium {
		t.Errorf("场景2 危机等级应 ≤ Medium，实际 %d", decision.Crisis.Level)
	}
	t.Logf("场景2: plan=%s crisis=%d tools=%d",
		planType(decision.Plan), decision.Crisis.Level, toolCount(decision.Plan))
}

// TestDirection6_Scenario3_AngryRefund 场景3: 愤怒 + 投诉退款 → 强制转人工
func TestDirection6_Scenario3_AngryRefund(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "d6_s3",
		Content:     "你们是不是骗子？赶紧退款！",
		TraceID:     "d6-s3",
	}, &AgentContext{AgentCode: "default"})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if !decision.HandoffToHuman {
		t.Error("场景3 必须转人工")
	}
	if decision.Crisis.Level < CrisisHigh {
		t.Errorf("场景3 危机等级应 ≥ High，实际 %d", decision.Crisis.Level)
	}
	if !strings.Contains(decision.Crisis.Reason, "high_risk") {
		t.Errorf("场景3 危机原因应为 high_risk，实际 %s", decision.Crisis.Reason)
	}
	if decision.Reply == "" {
		t.Error("场景3 应有 fallback 回复")
	}
	t.Logf("场景3: reason=%s handoff=%v reply=%s",
		decision.Crisis.Reason, decision.HandoffToHuman, decision.Reply)
}

// TestDirection6_Scenario4_HandoffRequest 场景4: 平静 + 强转人工 → 强制转人工
func TestDirection6_Scenario4_HandoffRequest(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "d6_s4",
		Content:     "我想找你们人工客服。",
		TraceID:     "d6-s4",
	}, &AgentContext{AgentCode: "default"})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if !decision.HandoffToHuman {
		t.Error("场景4 必须转人工")
	}
	if decision.Crisis.Level < CrisisHigh {
		t.Errorf("场景4 危机等级应 ≥ High，实际 %d", decision.Crisis.Level)
	}
	if !strings.Contains(decision.Crisis.Reason, "handoff_human") {
		t.Errorf("场景4 危机原因应含 handoff_human，实际 %s", decision.Crisis.Reason)
	}
	t.Logf("场景4: reason=%s handoff=%v",
		decision.Crisis.Reason, decision.HandoffToHuman)
}

// TestDirection6_Scenario1_StageTrace 场景1 阶段轨迹验证
func TestDirection6_Scenario1_StageTrace(t *testing.T) {
	cycle := NewInferenceCycle()
	ic := &InferenceContext{
		Payload:  CustomerMessagePayload{Content: "这个产品怎么用？", CustomerID: "d6_s1t", ChannelType: "tg"},
		AgentCtx: &AgentContext{AgentCode: "default", EnableRAG: true},
	}
	cycle.PerceptionStage.Execute(context.Background(), ic)
	cycle.AlignmentStage.Execute(context.Background(), ic)
	cycle.GatekeeperStage.Execute(context.Background(), ic)

	if len(ic.Stages) < 3 {
		t.Fatalf("expected 3 stage records, got %d", len(ic.Stages))
	}
	expected := []string{"perception", "alignment", "gatekeeper"}
	for i, s := range ic.Stages {
		if s.Stage != expected[i] {
			t.Errorf("stage %d = %s, want %s", i, s.Stage, expected[i])
		}
	}
	if ic.Crisis.Level > CrisisLow {
		t.Errorf("场景1 crisis level = %d, want <= CrisisLow", ic.Crisis.Level)
	}
}

// TestDirection6_AllScenarios_Batch 一次性跑通 4 个场景
func TestDirection6_AllScenarios_Batch(t *testing.T) {
	cycle := NewInferenceCycle()
	scenarios := []struct {
		name        string
		content     string
		agentCtx    *AgentContext
		wantHandoff bool
	}{
		{"calm_faq", "这个产品怎么用？", &AgentContext{AgentCode: "default", EnableRAG: true}, false},
		{"anxious_order", "我等了3天了，怎么还没发货?", &AgentContext{AgentCode: "default", EnableRAG: true}, false},
		{"angry_refund", "你们是不是骗子？赶紧退款！", &AgentContext{AgentCode: "default"}, true},
		{"handoff_request", "我想找你们人工客服。", &AgentContext{AgentCode: "default"}, true},
	}
	for _, s := range scenarios {
		decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
			ChannelType: "telegram",
			CustomerID:  "d6_batch_" + s.name,
			Content:     s.content,
			TraceID:     "d6-batch-" + s.name,
		}, s.agentCtx)
		if err != nil {
			t.Errorf("[%s] RunOnce error: %v", s.name, err)
			continue
		}
		if decision.HandoffToHuman != s.wantHandoff {
			t.Errorf("[%s] HandoffToHuman=%v, want %v (reason=%s)",
				s.name, decision.HandoffToHuman, s.wantHandoff, decision.Crisis.Reason)
		}
	}
}

func planType(p *ActionPlan) string {
	if p == nil {
		return "(no-plan)"
	}
	return p.PlanType
}

func toolCount(p *ActionPlan) int {
	if p == nil {
		return 0
	}
	return len(p.ToolCalls)
}
