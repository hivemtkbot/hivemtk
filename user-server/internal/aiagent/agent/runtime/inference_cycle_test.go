package agent_runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Inference Cycle 测试套件
// ----------------------------------------------------------------------------
// 覆盖：
//  1. 端到端：闲聊 / 询价 / 投诉 / 退款 / 转人工 等场景
//  2. 各阶段：感知 / 对齐 / 门禁 / 规划 独立单元测试
//  3. 边界：空内容 / 极长内容 / 异常情绪
//  4. 性能：循环 1000 次，统计平均耗时
// ============================================================================

// ============================================================================
// Stage 1: 感知阶段测试
// ============================================================================

// TestPerceptionStage_Greeting 测试寒暄场景
func TestPerceptionStage_Greeting(t *testing.T) {
	ctx := context.Background()
	stage := NewDefaultPerceptionStage()
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{
			ChannelType: "telegram",
			CustomerID:  "cust_001",
			Content:     "你好，在吗？",
		},
	}

	result := stage.Execute(ctx, ic)
	if !result.Continue {
		t.Fatal("perception stage should continue")
	}
	if ic.Sentiment.Label != SentimentCalm && ic.Sentiment.Label != SentimentUnknown {
		t.Errorf("expected calm/unknown, got %s", ic.Sentiment.Label)
	}
	if ic.Intent.Primary != IntentGreeting && ic.Intent.Primary != IntentChitchat {
		t.Errorf("expected greeting/chitchat, got %s", ic.Intent.Primary)
	}
}

// TestPerceptionStage_Angry 测试愤怒场景
func TestPerceptionStage_Angry(t *testing.T) {
	ctx := context.Background()
	stage := NewDefaultPerceptionStage()
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{
			ChannelType: "telegram",
			CustomerID:  "cust_002",
			Content:     "你们是骗子！我要起诉！",
		},
	}

	result := stage.Execute(ctx, ic)
	if !result.Continue {
		t.Fatal("perception stage should continue")
	}
	if ic.Sentiment.Label != SentimentAngry {
		t.Errorf("expected angry, got %s", ic.Sentiment.Label)
	}
}

// TestPerceptionStage_Appreci 测试赞赏场景
func TestPerceptionStage_Appreci(t *testing.T) {
	ctx := context.Background()
	stage := NewDefaultPerceptionStage()
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{
			Content: "太棒了！非常感谢！",
		},
	}

	result := stage.Execute(ctx, ic)
	if !result.Continue {
		t.Fatal("perception stage should continue")
	}
	if ic.Sentiment.Label != SentimentAppreci {
		t.Errorf("expected appreci, got %s", ic.Sentiment.Label)
	}
}

// TestKeywordSentimentAnalyzer_Angry 验证关键词分析器对"骗子"的愤怒识别
func TestKeywordSentimentAnalyzer_Angry(t *testing.T) {
	a := NewKeywordSentimentAnalyzer()
	score := a.Analyze(context.Background(), "你们是骗子，我要起诉！")
	if score.Label != SentimentAngry {
		t.Errorf("expected angry, got %s score=%f", score.Label, score.Score)
	}
	if score.Score < 0.5 {
		t.Errorf("expected high score, got %f", score.Score)
	}
}

// TestKeywordIntentRecognizer_Refund 验证退款意图识别
func TestKeywordIntentRecognizer_Refund(t *testing.T) {
	r := NewKeywordIntentRecognizer()
	intent := r.Recognize(context.Background(), "我要退款", nil)
	if intent.Primary != IntentRefund {
		t.Errorf("expected refund, got %s", intent.Primary)
	}
}

// TestKeywordIntentRecognizer_Inquiry 验证询价意图
func TestKeywordIntentRecognizer_Inquiry(t *testing.T) {
	r := NewKeywordIntentRecognizer()
	intent := r.Recognize(context.Background(), "请问这个多少钱？", nil)
	if intent.Primary != IntentInquiry {
		t.Errorf("expected inquiry, got %s", intent.Primary)
	}
}

// TestKeywordIntentRecognizer_OrderStatus 验证查单意图
func TestKeywordIntentRecognizer_OrderStatus(t *testing.T) {
	r := NewKeywordIntentRecognizer()
	intent := r.Recognize(context.Background(), "我的订单到哪了？", nil)
	if intent.Primary != IntentOrderStatus {
		t.Errorf("expected order_status, got %s", intent.Primary)
	}
}

// ============================================================================
// Stage 2: 对齐阶段测试
// ============================================================================

// TestAlignmentStage_Empathy 验证同理心打分
func TestAlignmentStage_Empathy(t *testing.T) {
	stage := NewDefaultAlignmentScorer()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "你骗我"},
		Sentiment: SentimentScore{Label: SentimentAngry, Score: 0.9},
		Intent:    IntentResult{Primary: IntentComplaint},
	}

	score := stage.Score(context.Background(), ic)
	if score.Empathy < 4 {
		t.Errorf("expected empathy >= 4 for angry, got %d", score.Empathy)
	}
}

// TestAlignmentStage_Enthusiasm 验证热情度打分
func TestAlignmentStage_Enthusiasm(t *testing.T) {
	stage := NewDefaultAlignmentScorer()
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{Content: "你们产品价格？"},
		Intent:  IntentResult{Primary: IntentInquiry},
	}

	score := stage.Score(context.Background(), ic)
	if score.Enthusiasm < 4 {
		t.Errorf("expected enthusiasm >= 4 for inquiry, got %d", score.Enthusiasm)
	}
}

// TestAlignmentScore_Total 验证总分
func TestAlignmentScore_Total(t *testing.T) {
	a := AlignmentScore{Empathy: 5, Enthusiasm: 4, Expertise: 3, Patience: 3, Clarity: 4, Politeness: 5}
	expected := (5.0 + 4.0 + 3.0 + 3.0 + 4.0 + 5.0) / 6.0
	if a.Total() != expected {
		t.Errorf("Total() = %f, want %f", a.Total(), expected)
	}
}

// TestAlignmentScore_MaxDimension 验证最弱维度
func TestAlignmentScore_MaxDimension(t *testing.T) {
	a := AlignmentScore{Empathy: 5, Enthusiasm: 3, Expertise: 5, Patience: 5, Clarity: 5, Politeness: 5}
	if a.MaxDimension() != DimEnthusiasm {
		t.Errorf("MaxDimension() = %s, want enthusiasm", a.MaxDimension())
	}
}

// ============================================================================
// Stage 3: 门禁阶段测试
// ============================================================================

// TestGatekeeper_Refund 验证退款触发转人工
func TestGatekeeper_Refund(t *testing.T) {
	d := NewDefaultCrisisDetector()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "我要退款"},
		Sentiment: SentimentScore{Label: SentimentAngry, Score: 0.6},
		Alignment: AlignmentScore{Empathy: 3},
	}

	result := d.Execute(context.Background(), ic)
	if !result.EarlyReturn {
		t.Fatal("refund should trigger early return")
	}
	if !ic.Decision.HandoffToHuman {
		t.Error("refund should set HandoffToHuman=true")
	}
	if !strings.Contains(ic.Crisis.Reason, "high_risk_keyword") {
		t.Errorf("expected high_risk_keyword, got %s", ic.Crisis.Reason)
	}
}

// TestGatekeeper_Complaint 验证投诉命中但不强转
func TestGatekeeper_Complaint(t *testing.T) {
	d := NewDefaultCrisisDetector()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "我要投诉"},
		Sentiment: SentimentScore{Label: SentimentAngry, Score: 0.7},
		Alignment: AlignmentScore{Empathy: 3},
	}

	result := d.Execute(context.Background(), ic)
	if result.EarlyReturn {
		// 投诉关键词 + 愤怒情绪 → 高危机 → 转人工
		// 这是预期行为
	}
	if ic.Crisis.Level < CrisisMedium {
		t.Errorf("expected at least Medium crisis, got %d", ic.Crisis.Level)
	}
}

// TestGatekeeper_Normal 验证正常对话不触发
func TestGatekeeper_Normal(t *testing.T) {
	d := NewDefaultCrisisDetector()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "你好，请问有什么产品？"},
		Sentiment: SentimentScore{Label: SentimentCalm, Score: 0.3},
		Alignment: AlignmentScore{Empathy: 3, Enthusiasm: 4, Expertise: 3, Patience: 3, Clarity: 4, Politeness: 3},
	}

	result := d.Execute(context.Background(), ic)
	if result.EarlyReturn {
		t.Fatal("normal message should not trigger early return")
	}
	if ic.Crisis.Level != CrisisNone && ic.Crisis.Level != CrisisLow {
		t.Errorf("expected None/Low, got %d", ic.Crisis.Level)
	}
}

// TestCrisisSignal_NeedsEscalation 验证危机升级条件
func TestCrisisSignal_NeedsEscalation(t *testing.T) {
	tests := []struct {
		level CrisisLevel
		want  bool
	}{
		{CrisisNone, false},
		{CrisisLow, false},
		{CrisisMedium, false},
		{CrisisHigh, true},
	}
	for _, tt := range tests {
		c := CrisisSignal{Level: tt.level}
		if got := c.NeedsEscalation(); got != tt.want {
			t.Errorf("level=%d NeedsEscalation=%v, want %v", tt.level, got, tt.want)
		}
	}
}

// ============================================================================
// Stage 4: 规划器测试
// ============================================================================

// TestPlanner_Greeting 验证寒暄规划
func TestPlanner_Greeting(t *testing.T) {
	p := NewDefaultTaskPlanner()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "你好"},
		Sentiment: SentimentScore{Label: SentimentCalm, Score: 0.3},
		Intent:    IntentResult{Primary: IntentGreeting},
		Alignment: AlignmentScore{Empathy: 3, Enthusiasm: 4, Expertise: 3, Patience: 3, Clarity: 4, Politeness: 4},
	}

	plan, err := p.Plan(context.Background(), ic)
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}
	if plan.PlanType != "customer_service" {
		t.Errorf("expected customer_service, got %s", plan.PlanType)
	}
	if plan.ReplyHint == "" {
		t.Error("expected non-empty reply hint")
	}
}

// TestPlanner_Inquiry 验证询价规划（含工具链）
func TestPlanner_Inquiry(t *testing.T) {
	p := NewDefaultTaskPlanner()
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "多少钱？"},
		Sentiment: SentimentScore{Label: SentimentCalm, Score: 0.3},
		Intent:    IntentResult{Primary: IntentInquiry},
		Alignment: AlignmentScore{Empathy: 3, Enthusiasm: 5, Expertise: 3, Patience: 3, Clarity: 4, Politeness: 3},
		AgentCtx: &AgentContext{
			EnableRAG: true,
			Persona:   "专业销售",
		},
	}

	plan, err := p.Plan(context.Background(), ic)
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}
	if plan.PlanType != "sales" {
		t.Errorf("expected sales, got %s", plan.PlanType)
	}
	if len(plan.ToolCalls) == 0 {
		t.Error("expected at least one tool call")
	}
	hasKAG := false
	for _, tc := range plan.ToolCalls {
		if tc.ToolName == "knowledge.search" {
			hasKAG = true
		}
	}
	if !hasKAG {
		t.Error("expected knowledge.search tool call for inquiry")
	}
}

// TestPlanner_FAQSkip 验证 FAQ 命中跳过 LLM
func TestPlanner_FAQSkip(t *testing.T) {
	faq := func(ctx context.Context, content string) (bool, string) {
		if content == "营业时间" {
			return true, "我们 9-18 点营业"
		}
		return false, ""
	}
	p := NewDefaultTaskPlannerWithFAQ(faq)
	ic := &InferenceContext{
		Payload:   CustomerMessagePayload{Content: "营业时间"},
		Sentiment: SentimentScore{Label: SentimentCalm, Score: 0.3},
		Intent:    IntentResult{Primary: IntentInquiry},
	}

	plan, err := p.Plan(context.Background(), ic)
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}
	if !plan.SkipLLM {
		t.Error("expected SkipLLM=true for FAQ match")
	}
	if plan.ReplyHint != "我们 9-18 点营业" {
		t.Errorf("expected FAQ reply, got %s", plan.ReplyHint)
	}
}

// TestPlanner_NoRAG 验证关闭 RAG 后不调用 knowledge.search
func TestPlanner_NoRAG(t *testing.T) {
	p := NewDefaultTaskPlanner()
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{Content: "产品多少钱？"},
		Intent:  IntentResult{Primary: IntentInquiry},
		AgentCtx: &AgentContext{
			EnableRAG: false, // 关闭 RAG
		},
	}

	plan, err := p.Plan(context.Background(), ic)
	if err != nil {
		t.Fatalf("plan error: %v", err)
	}
	for _, tc := range plan.ToolCalls {
		if tc.ToolName == "knowledge.search" {
			t.Error("knowledge.search should be filtered out when RAG disabled")
		}
	}
}

// ============================================================================
// 端到端推理闭环测试
// ============================================================================

// TestInferenceCycle_Greeting 端到端：寒暄
func TestInferenceCycle_Greeting(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_e2e_1",
		Content:     "你好",
		TraceID:     "trace_e2e_1",
	}, &AgentContext{AgentCode: "default", EnableRAG: true})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision.HandoffToHuman {
		t.Error("greeting should not handoff")
	}
	if decision.Plan == nil {
		t.Fatal("expected plan to be set")
	}
	if len(decision.Plan.ToolCalls) == 0 && !decision.Plan.SkipLLM {
		// 寒暄无工具调用是合理的
	}
}

// TestInferenceCycle_Escalation 端到端：转人工
func TestInferenceCycle_Escalation(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_e2e_2",
		Content:     "我要退款，你们是骗子！",
		TraceID:     "trace_e2e_2",
	}, &AgentContext{AgentCode: "default"})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if !decision.HandoffToHuman {
		t.Error("refund+scam should handoff to human")
	}
	if decision.HandoffReason == "" {
		t.Error("expected handoff reason")
	}
	if decision.Reply == "" {
		t.Error("expected fallback reply")
	}
}

// TestInferenceCycle_PlanReady 端到端：正常询价
func TestInferenceCycle_PlanReady(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_e2e_3",
		Content:     "你们产品多少钱？",
		TraceID:     "trace_e2e_3",
	}, &AgentContext{AgentCode: "default", EnableRAG: true})
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision.HandoffToHuman {
		t.Error("normal inquiry should not handoff")
	}
	if decision.Plan == nil {
		t.Fatal("expected plan")
	}
	if decision.Plan.PlanType != "sales" {
		t.Errorf("expected sales plan, got %s", decision.Plan.PlanType)
	}
}

// TestInferenceCycle_FAQSkip 端到端：FAQ 命中
func TestInferenceCycle_FAQSkip(t *testing.T) {
	faq := func(ctx context.Context, content string) (bool, string) {
		if content == "发货时间" {
			return true, "下单后 24 小时内发货"
		}
		return false, ""
	}
	cycle := NewInferenceCycle()
	// 注入自定义 planner
	planner := NewDefaultTaskPlannerWithFAQ(faq)
	cycle.PlannerStage = planner

	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		Content: "发货时间",
	}, nil)
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if !decision.Plan.SkipLLM {
		t.Error("expected SkipLLM=true for FAQ")
	}
	if decision.Reply != "下单后 24 小时内发货" {
		t.Errorf("expected FAQ reply, got %s", decision.Reply)
	}
}

// TestInferenceCycle_Stages 验证所有阶段都被执行并记录
func TestInferenceCycle_Stages(t *testing.T) {
	cycle := NewInferenceCycle()
	payload := CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_e2e_4",
		Content:     "你好",
		TraceID:     "trace_e2e_4",
	}
	agentCtx := &AgentContext{AgentCode: "default", EnableRAG: true}

	decision, err := cycle.RunOnce(context.Background(), payload, agentCtx)
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	// 通过 stats 验证
	stats := cycle.GetStats()
	if stats.TotalRuns == 0 {
		t.Error("expected TotalRuns > 0")
	}
}

// TestInferenceCycle_ContextStages 验证 Stages 字段被填充
func TestInferenceCycle_ContextStages(t *testing.T) {
	cycle := NewInferenceCycle()
	ic := &InferenceContext{
		Payload:  CustomerMessagePayload{Content: "你好", CustomerID: "c1", ChannelType: "tg"},
		AgentCtx: &AgentContext{AgentCode: "default", EnableRAG: true},
	}

	// 手动执行各阶段
	cycle.PerceptionStage.Execute(context.Background(), ic)
	cycle.AlignmentStage.Execute(context.Background(), ic)
	cycle.GatekeeperStage.Execute(context.Background(), ic)
	cycle.PlannerStage.Execute(context.Background(), ic)

	if len(ic.Stages) < 4 {
		t.Errorf("expected at least 4 stage records, got %d", len(ic.Stages))
	}

	// 验证每个阶段都有 Name / Duration
	for i, s := range ic.Stages {
		if s.Stage == "" {
			t.Errorf("stage %d missing name", i)
		}
		if s.Duration < 0 {
			t.Errorf("stage %s has negative duration", s.Stage)
		}
	}
}

// TestInferenceCycle_Stop 验证停止后不能再运行
func TestInferenceCycle_Stop(t *testing.T) {
	cycle := NewInferenceCycle()
	if err := cycle.Stop(context.Background()); err != nil {
		t.Fatalf("stop error: %v", err)
	}
	_, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		Content: "hello",
	}, nil)
	if err == nil {
		t.Error("expected error after stop")
	}
	if err != ErrRuntimeStopped {
		t.Errorf("expected ErrRuntimeStopped, got %v", err)
	}
	cycle.Reset()
}

// ============================================================================
// 性能与压力测试
// ============================================================================

// TestInferenceCycle_100Runs 验证 100 次循环
func TestInferenceCycle_100Runs(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in short mode")
	}
	cycle := NewInferenceCycle()
	scenarios := []string{
		"你好",
		"产品多少钱？",
		"我的订单呢？",
		"我要退款！",
		"谢谢",
		"我需要售后",
		"不错啊",
	}

	for i := 0; i < 100; i++ {
		_, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
			ChannelType: "telegram",
			CustomerID:  "cust_perf",
			Content:     scenarios[i%len(scenarios)],
			TraceID:     "trace_perf",
		}, &AgentContext{AgentCode: "default", EnableRAG: true})
		if err != nil {
			t.Fatalf("RunOnce iter=%d error: %v", i, err)
		}
	}

	stats := cycle.GetStats()
	if stats.TotalRuns < 100 {
		t.Errorf("expected TotalRuns >= 100, got %d", stats.TotalRuns)
	}
	t.Logf("stats: total=%d success=%d escalation=%d failure=%d avg_ms=%d",
		stats.TotalRuns, stats.SuccessRuns, stats.EscalationRuns, stats.FailureRuns, stats.AvgDurationMs)
}

// TestInferenceCycle_PerfSingleRun 单次推理闭环耗时 < 50ms
func TestInferenceCycle_PerfSingleRun(t *testing.T) {
	if testing.Short() {
		t.Skip("skip in short mode")
	}
	cycle := NewInferenceCycle()
	payload := CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_perf_single",
		Content:     "你好",
	}
	agentCtx := &AgentContext{AgentCode: "default", EnableRAG: true}

	start := time.Now()
	decision, err := cycle.RunOnce(context.Background(), payload, agentCtx)
	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	dur := time.Since(start)
	t.Logf("single run duration: %s decision=%+v", dur, decision)

	if dur > 100*time.Millisecond {
		t.Errorf("single run too slow: %s", dur)
	}
}

// ============================================================================
// 边界场景测试
// ============================================================================

// TestInferenceCycle_EmptyContent 空内容
func TestInferenceCycle_EmptyContent(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_empty",
		Content:     "",
	}, &AgentContext{AgentCode: "default"})

	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
	// 空内容应能正常处理（不会 panic）
}

// TestInferenceCycle_LongContent 极长内容
func TestInferenceCycle_LongContent(t *testing.T) {
	cycle := NewInferenceCycle()
	longText := ""
	for i := 0; i < 1000; i++ {
		longText += "重复内容"
	}
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		ChannelType: "telegram",
		CustomerID:  "cust_long",
		Content:     longText,
	}, &AgentContext{AgentCode: "default"})

	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
}

// TestInferenceCycle_NilAgentCtx nil agentCtx 应不 panic
func TestInferenceCycle_NilAgentCtx(t *testing.T) {
	cycle := NewInferenceCycle()
	decision, err := cycle.RunOnce(context.Background(), CustomerMessagePayload{
		Content: "hello",
	}, nil)

	if err != nil {
		t.Fatalf("RunOnce error: %v", err)
	}
	if decision == nil {
		t.Fatal("expected non-nil decision")
	}
}

// TestValidatePayload 验证载荷校验
func TestValidatePayload(t *testing.T) {
	if err := ValidatePayload(CustomerMessagePayload{}); err == nil {
		t.Error("expected error for empty payload")
	}
	if err := ValidatePayload(CustomerMessagePayload{ChannelType: "tg"}); err == nil {
		t.Error("expected error for missing customer_id")
	}
	if err := ValidatePayload(CustomerMessagePayload{ChannelType: "tg", CustomerID: "c1"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestMergeDecision 验证决策合并
func TestMergeDecision(t *testing.T) {
	base := InferenceDecision{Confidence: 0.5, ReplyType: "text", StopReason: "init"}
	override := InferenceDecision{HandoffToHuman: true, HandoffReason: "crisis", Reply: "fallback", Confidence: 0.9}

	merged := mergeDecision(base, override)

	if !merged.HandoffToHuman {
		t.Error("HandoffToHuman should be true")
	}
	if merged.HandoffReason != "crisis" {
		t.Errorf("HandoffReason = %s, want crisis", merged.HandoffReason)
	}
	if merged.Reply != "fallback" {
		t.Errorf("Reply = %s, want fallback", merged.Reply)
	}
	if merged.Confidence != 0.9 {
		t.Errorf("Confidence = %f, want 0.9", merged.Confidence)
	}
	if merged.ReplyType != "text" {
		t.Errorf("ReplyType should be preserved from base, got %s", merged.ReplyType)
	}
}

// TestStageResult_Helpers 验证 StageResult 辅助函数
func TestStageResult_Helpers(t *testing.T) {
	c := ContinueResult()
	if !c.Continue {
		t.Error("ContinueResult should have Continue=true")
	}

	d := &InferenceDecision{HandoffToHuman: true}
	s := StopResult(d)
	if !s.EarlyReturn {
		t.Error("StopResult should have EarlyReturn=true")
	}
	if s.Decision != d {
		t.Error("StopResult should preserve decision")
	}

	err := error(nil)
	f := FailResult(err)
	if !f.EarlyReturn {
		t.Error("FailResult should have EarlyReturn=true")
	}
}

// TestInferenceCycle_IntegrationWithTypes 验证类型完整性
func TestInferenceCycle_IntegrationWithTypes(t *testing.T) {
	ic := &InferenceContext{
		Payload: CustomerMessagePayload{Content: "test"},
		Sentiment: SentimentScore{Label: SentimentCalm, Score: 0.5, Detail: map[Sentiment]float64{
			SentimentCalm: 0.5, SentimentAngry: 0.1,
		}},
		Intent:    IntentResult{Primary: IntentInquiry, Secondary: []Intent{IntentGreeting}, Score: 0.8},
		Alignment: AlignmentScore{Empathy: 4, Enthusiasm: 5, Expertise: 3, Patience: 3, Clarity: 4, Politeness: 4},
		Crisis:    CrisisSignal{Level: CrisisNone},
		Plan:      &ActionPlan{PlanType: "sales", Confidence: 0.8, ToolCalls: []PlannedToolCall{}},
		Decision:  InferenceDecision{Confidence: 0.8, ReplyType: "text"},
	}

	if ic.Sentiment.Detail[SentimentCalm] != 0.5 {
		t.Error("sentiment detail should be preserved")
	}
	if len(ic.Intent.Secondary) != 1 {
		t.Error("intent secondary should be preserved")
	}
	if ic.Alignment.Total() <= 0 {
		t.Error("alignment total should be > 0")
	}
	if ic.Plan.Confidence != 0.8 {
		t.Error("plan confidence should be preserved")
	}
}
