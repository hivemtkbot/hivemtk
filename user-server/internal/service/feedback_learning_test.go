package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/testutil"

	"gorm.io/gorm"
)

// setupFeedbackLearningDB 初始化测试 DB
func setupFeedbackLearningDB(t *testing.T) *gorm.DB {
	return testutil.NewTestDB(t,
		&model.SessionMessage{},
		&model.SOPAgent{},
		&model.SOPExecution{},
		&model.SOPNodeTransition{},
		&model.SalesChampionProfileSnapshot{},
		&model.OptimizationSuggestion{},
	)
}

// seedAIMessages 种子数据：AI 回复消息
func seedAIMessages(db *gorm.DB, sessionID string, contents []string, baseTime time.Time) {
	for i, c := range contents {
		msg := model.SessionMessage{
			SessionID:   sessionID,
			Content:     c,
			SenderType:  "ai",
			ContentType: "text",
			CreatedAt:   baseTime.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&msg).Error; err != nil {
			panic(err)
		}
	}
}

// seedCustomerMessages 种子数据：客户消息
func seedCustomerMessages(db *gorm.DB, sessionID string, contents []string, baseTime time.Time) {
	for i, c := range contents {
		msg := model.SessionMessage{
			SessionID:   sessionID,
			Content:     c,
			SenderType:  "user",
			ContentType: "text",
			CreatedAt:   baseTime.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&msg).Error; err != nil {
			panic(err)
		}
	}
}

// TestExtractProfile_AllDimensionsPresent 所有 5 维度都被提取
func TestExtractProfile_AllDimensionsPresent(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"亲，我理解您的顾虑，这款性价比其实很高，现在有活动优惠很划算。",
		"建议您今天下单预约，名额有限，帮您锁定优惠。",
		"好久没联系了，最近有新品到货，非常适合您，帮您留一份。",
		"根据您之前的需求，我为您推荐专属方案，建议安排体验。",
		"感谢您成为我们的老朋友，会员积分可以抵扣，复购再享专属优惠。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(report.Dimensions) != len(model.AllSalesChampionDimensions) {
		t.Errorf("expected %d dimensions, got %d", len(model.AllSalesChampionDimensions), len(report.Dimensions))
	}
	for _, d := range report.Dimensions {
		if d.Name == "" {
			t.Errorf("dimension %s missing name", d.Dimension)
		}
	}
}

// TestExtractProfile_ObjectionHandling 异议处理维度
func TestExtractProfile_ObjectionHandling(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedCustomerMessages(db, "s1", []string{"太贵了"}, baseTime)
	seedAIMessages(db, "s1", []string{
		"我理解您的顾虑，其实这款性价比很高，现在有活动很划算。",
	}, baseTime.Add(30*time.Second))

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionObjectionHandling)
	if dim.SampleCount == 0 {
		t.Errorf("expected objection_handling samples > 0, got 0")
	}
	if dim.PositiveCount == 0 {
		t.Errorf("expected positive count > 0 for AI proper objection response")
	}
}

// TestExtractProfile_ObjectionAbandoned 异议放弃（负向）
func TestExtractProfile_ObjectionAbandoned(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedCustomerMessages(db, "s1", []string{"太贵了，不需要"}, baseTime)
	seedAIMessages(db, "s1", []string{
		"好的，再见",
	}, baseTime.Add(30*time.Second))

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionObjectionHandling)
	if dim.NegativeCount == 0 {
		t.Errorf("expected negative count > 0 for objection abandoned")
	}
}

// TestExtractProfile_ClosingInvitation 逼单邀约维度
func TestExtractProfile_ClosingInvitation(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedCustomerMessages(db, "s1", []string{"可以的，下单吧"}, baseTime)
	seedAIMessages(db, "s1", []string{
		"今天下单可以享受 8 折优惠，帮您安排。",
	}, baseTime.Add(30*time.Second))

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionClosingInvitation)
	if dim.SampleCount == 0 {
		t.Errorf("expected closing_invitation samples > 0")
	}
}

// TestExtractProfile_FollowupActivation 跟进激活维度
func TestExtractProfile_FollowupActivation(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"好久没联系了，最近有新品到货，活动优惠很划算，帮您留一份。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionFollowupActivation)
	if dim.SampleCount == 0 {
		t.Errorf("expected followup_activation samples > 0")
	}
	if dim.PositiveCount == 0 {
		t.Errorf("expected positive count > 0 for value-based followup")
	}
}

// TestExtractProfile_NurturingConversion 培育转化维度
func TestExtractProfile_NurturingConversion(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"根据您之前的需求，我为您推荐专属方案，建议安排体验。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionNurturingConversion)
	if dim.SampleCount == 0 {
		t.Errorf("expected nurturing_conversion samples > 0")
	}
}

// TestExtractProfile_RepurchaseOperation 复购运营维度
func TestExtractProfile_RepurchaseOperation(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"感谢您成为我们的老朋友，会员积分可以抵扣，复购再享专属优惠福利。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	dim := findDimension(report, model.DimensionRepurchaseOperation)
	if dim.SampleCount == 0 {
		t.Errorf("expected repurchase_operation samples > 0")
	}
}

// TestExtractProfile_PersistSnapshot 验证画像快照持久化
func TestExtractProfile_PersistSnapshot(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"理解您的顾虑，性价比很高，建议下单安排体验。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 42, "员工A", "staff_42", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	// 验证快照已持久化
	var count int64
	db.Model(&model.SalesChampionProfileSnapshot{}).Where("staff_id = ?", 42).Count(&count)
	if count != int64(len(model.AllSalesChampionDimensions)) {
		t.Errorf("expected %d snapshots, got %d", len(model.AllSalesChampionDimensions), count)
	}

	// 验证维度字段正确
	var snapshots []model.SalesChampionProfileSnapshot
	db.Where("staff_id = ?", 42).Find(&snapshots)
	dimSet := map[string]bool{}
	for _, s := range snapshots {
		dimSet[s.Dimension] = true
		if s.StaffName != "员工A" {
			t.Errorf("expected staff_name=员工A, got %s", s.StaffName)
		}
		if s.Scenario != "staff_42" {
			t.Errorf("expected scenario=staff_42, got %s", s.Scenario)
		}
	}
	for _, dim := range model.AllSalesChampionDimensions {
		if !dimSet[string(dim)] {
			t.Errorf("missing dimension snapshot: %s", dim)
		}
	}

	_ = report
}

// TestExtractProfile_NoSamples 中性得分（无样本时 50 分）
func TestExtractProfile_NoSamples(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", time.Now().Add(-1*time.Hour), time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	for _, d := range report.Dimensions {
		if d.Score != 50.0 {
			t.Errorf("dimension %s: expected neutral 50, got %.2f", d.Dimension, d.Score)
		}
	}
}

// TestExtractProfile_OverallScore 综合分计算
func TestExtractProfile_OverallScore(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-1 * time.Hour)
	seedAIMessages(db, "s1", []string{
		"理解您的顾虑，性价比很高，建议下单。",
		"好久没联系，新品到货，活动优惠适合您。",
		"老朋友，会员积分复购专属福利。",
	}, baseTime)

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if report.OverallScore < 0 || report.OverallScore > 100 {
		t.Errorf("overall score out of [0,100]: %.2f", report.OverallScore)
	}
}

// seedSOPAgent 创建测试 SOP
func seedSOPAgent(db *gorm.DB, name string) uint {
	agent := model.SOPAgent{
		Name:     name,
		Scenario: "test",
		SOPGraph: model.JSONMap{"nodes": []any{}},
		IsActive: true,
		Version:  1,
	}
	if err := db.Create(&agent).Error; err != nil {
		panic(err)
	}
	return agent.ID
}

// seedNodeTransition 创建节点流转记录
func seedNodeTransition(db *gorm.DB, sopID, execID uint, toNode, nodeType, outcome string, durationMs int) {
	t := model.SOPNodeTransition{
		SOPID:       sopID,
		ExecutionID: execID,
		ToNode:      toNode,
		NodeType:    nodeType,
		Outcome:     outcome,
		DurationMs:  durationMs,
	}
	if err := db.Create(&t).Error; err != nil {
		panic(err)
	}
}

// TestAnalyzeNodeConversion_BasicConversion 基础转化率计算
func TestAnalyzeNodeConversion_BasicConversion(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 8; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "node_1", "llm", model.NodeOutcomeSuccess, 1000)
	}
	seedNodeTransition(db, sopID, 9, "node_1", "llm", model.NodeOutcomeAbandoned, 500)
	seedNodeTransition(db, sopID, 10, "node_1", "llm", model.NodeOutcomeFailed, 200)

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(stats.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(stats.Nodes))
	}
	node := stats.Nodes[0]
	if node.EnteredCount != 10 {
		t.Errorf("expected entered=10, got %d", node.EnteredCount)
	}
	if node.SuccessCount != 8 {
		t.Errorf("expected success=8, got %d", node.SuccessCount)
	}
	if node.ConversionRate != 80.0 {
		t.Errorf("expected conversion_rate=80.0, got %.2f", node.ConversionRate)
	}
}

// TestAnalyzeNodeConversion_BottleneckIdentification 瓶颈节点识别
func TestAnalyzeNodeConversion_BottleneckIdentification(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 8; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "node_1", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(9+i), "node_1", "llm", model.NodeOutcomeAbandoned, 500)
	}
	for i := 0; i < 3; i++ {
		seedNodeTransition(db, sopID, uint(11+i), "node_2", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 7; i++ {
		seedNodeTransition(db, sopID, uint(14+i), "node_2", "llm", model.NodeOutcomeAbandoned, 500)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	nodeMap := map[string]NodeConversionDetail{}
	for _, n := range stats.Nodes {
		nodeMap[n.NodeID] = n
	}
	if !nodeMap["node_2"].IsBottleneck {
		t.Errorf("node_2 should be bottleneck (30%% conversion, 10 samples)")
	}
	if nodeMap["node_1"].IsBottleneck {
		t.Errorf("node_1 should not be bottleneck (80%% conversion)")
	}
}

// TestAnalyzeNodeConversion_WithVariant A/B 测试 variant 过滤
func TestAnalyzeNodeConversion_WithVariant(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 5; i++ {
		t := model.SOPNodeTransition{
			SOPID: sopID, ExecutionID: uint(i + 1), Variant: "A",
			ToNode: "node_a", NodeType: "llm", Outcome: model.NodeOutcomeSuccess,
		}
		db.Create(&t)
	}
	for i := 0; i < 5; i++ {
		t := model.SOPNodeTransition{
			SOPID: sopID, ExecutionID: uint(i + 6), Variant: "B",
			ToNode: "node_b", NodeType: "llm", Outcome: model.NodeOutcomeSuccess,
		}
		db.Create(&t)
	}

	statsA, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "A")
	if err != nil {
		t.Fatalf("analyze A: %v", err)
	}
	for _, n := range statsA.Nodes {
		if n.NodeID == "node_b" {
			t.Errorf("variant A stats should not include node_b")
		}
	}
	if len(statsA.Nodes) != 1 || statsA.Nodes[0].NodeID != "node_a" {
		t.Errorf("expected only node_a for variant A")
	}
}

// TestAnalyzeNodeConversion_DropRate 流失率计算
func TestAnalyzeNodeConversion_DropRate(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 7; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "node_1", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 3; i++ {
		seedNodeTransition(db, sopID, uint(8+i), "node_1", "llm", model.NodeOutcomeAbandoned, 500)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	node := stats.Nodes[0]
	if node.DropRate != 30.0 {
		t.Errorf("expected drop_rate=30.0, got %.2f", node.DropRate)
	}
}

// TestGenerateOptimizationSuggestions_LLMNode LLM 节点建议
func TestGenerateOptimizationSuggestions_LLMNode(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 3; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "llm_node", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 7; i++ {
		seedNodeTransition(db, sopID, uint(4+i), "llm_node", "llm", model.NodeOutcomeAbandoned, 500)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "test_sop",
		NodeConversion: stats,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(suggestions) == 0 {
		t.Fatal("expected at least 1 suggestion for low-conversion LLM node")
	}
	sug := suggestions[0]
	if sug.SuggestionType != model.SuggestionTypePromptRewrite {
		t.Errorf("expected prompt_rewrite for LLM node, got %s", sug.SuggestionType)
	}
	if sug.Priority != 2 {
		t.Errorf("expected priority 2 (high) for LLM bottleneck, got %d", sug.Priority)
	}
	if sug.SuggestionText == "" {
		t.Errorf("suggestion text should not be empty")
	}
	// 验证持久化
	var dbCount int64
	db.Model(&model.OptimizationSuggestion{}).Where("sop_id = ?", sopID).Count(&dbCount)
	if dbCount == 0 {
		t.Errorf("suggestion should be persisted to DB")
	}
}

// TestGenerateOptimizationSuggestions_ConditionNode 条件节点建议
func TestGenerateOptimizationSuggestions_ConditionNode(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "cond_node", "condition", model.NodeOutcomeSuccess, 200)
	}
	for i := 0; i < 8; i++ {
		seedNodeTransition(db, sopID, uint(3+i), "cond_node", "condition", model.NodeOutcomeAbandoned, 100)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "test_sop",
		NodeConversion: stats,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	found := false
	for _, sug := range suggestions {
		if sug.SuggestionType == model.SuggestionTypeBranchPrune {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected branch_prune suggestion for condition node")
	}
}

// TestGenerateOptimizationSuggestions_ActionNode 动作节点建议
func TestGenerateOptimizationSuggestions_ActionNode(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "action_node", "action", model.NodeOutcomeSuccess, 300)
	}
	for i := 0; i < 6; i++ {
		seedNodeTransition(db, sopID, uint(3+i), "action_node", "action", model.NodeOutcomeAbandoned, 100)
	}
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(9+i), "action_node", "action", model.NodeOutcomeFailed, 50)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "test_sop",
		NodeConversion: stats,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	foundTiming := false
	for _, sug := range suggestions {
		if sug.SuggestionType == model.SuggestionTypeTimingAdjust {
			foundTiming = true
			break
		}
	}
	if !foundTiming {
		t.Errorf("expected timing_adjust for action node with high abandonment")
	}
}

// TestGenerateOptimizationSuggestions_HighConversionSkipped 高转化节点不生成建议
func TestGenerateOptimizationSuggestions_HighConversionSkipped(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	for i := 0; i < 9; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "good_node", "llm", model.NodeOutcomeSuccess, 500)
	}
	seedNodeTransition(db, sopID, 10, "good_node", "llm", model.NodeOutcomeAbandoned, 200)

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "test_sop",
		NodeConversion: stats,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for high conversion node, got %d", len(suggestions))
	}
}

// TestGenerateOptimizationSuggestions_MinSampleFilter 最小样本数过滤
func TestGenerateOptimizationSuggestions_MinSampleFilter(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "test_sop")
	seedNodeTransition(db, sopID, 1, "small_node", "llm", model.NodeOutcomeSuccess, 500)
	seedNodeTransition(db, sopID, 2, "small_node", "llm", model.NodeOutcomeAbandoned, 200)

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "test_sop",
		NodeConversion: stats,
		MinSampleCount: 5,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(suggestions) != 0 {
		t.Errorf("expected 0 suggestions for small sample node, got %d", len(suggestions))
	}
}

// TestReviewSuggestion_Approve 审核通过
func TestReviewSuggestion_Approve(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sug := model.OptimizationSuggestion{
		SOPID:          1,
		SOPName:        "test",
		NodeID:         "n1",
		SuggestionType: model.SuggestionTypePromptRewrite,
		SuggestionText: "test",
		Status:         model.SuggestionStatusPending,
	}
	db.Create(&sug)

	err := svc.ReviewSuggestion(context.Background(), sug.ID, 100, "approve")
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	var updated model.OptimizationSuggestion
	db.First(&updated, sug.ID)
	if updated.Status != model.SuggestionStatusApproved {
		t.Errorf("expected approved, got %s", updated.Status)
	}
	if updated.ReviewedBy != 100 {
		t.Errorf("expected reviewed_by=100, got %d", updated.ReviewedBy)
	}
	if updated.ReviewedAt == nil {
		t.Errorf("reviewed_at should be set")
	}
}

// TestReviewSuggestion_Apply 应用建议
func TestReviewSuggestion_Apply(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sug := model.OptimizationSuggestion{
		SOPID:          1,
		SOPName:        "test",
		NodeID:         "n1",
		SuggestionType: model.SuggestionTypeBranchPrune,
		SuggestionText: "test",
		Status:         model.SuggestionStatusPending,
	}
	db.Create(&sug)

	err := svc.ReviewSuggestion(context.Background(), sug.ID, 200, "apply")
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	var updated model.OptimizationSuggestion
	db.First(&updated, sug.ID)
	if updated.Status != model.SuggestionStatusApplied {
		t.Errorf("expected applied, got %s", updated.Status)
	}
	if updated.AppliedAt == nil {
		t.Errorf("applied_at should be set")
	}
}

// TestReviewSuggestion_Reject 拒绝建议
func TestReviewSuggestion_Reject(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sug := model.OptimizationSuggestion{
		SOPID:          1,
		SOPName:        "test",
		NodeID:         "n1",
		SuggestionType: model.SuggestionTypeNodeMerge,
		SuggestionText: "test",
		Status:         model.SuggestionStatusPending,
	}
	db.Create(&sug)

	err := svc.ReviewSuggestion(context.Background(), sug.ID, 300, "reject")
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	var updated model.OptimizationSuggestion
	db.First(&updated, sug.ID)
	if updated.Status != model.SuggestionStatusRejected {
		t.Errorf("expected rejected, got %s", updated.Status)
	}
}

// TestReviewSuggestion_InvalidAction 非法操作
func TestReviewSuggestion_InvalidAction(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sug := model.OptimizationSuggestion{
		SOPID: 1, SOPName: "test", NodeID: "n1", SuggestionText: "test", Status: model.SuggestionStatusPending,
	}
	db.Create(&sug)

	err := svc.ReviewSuggestion(context.Background(), sug.ID, 100, "invalid")
	if err == nil {
		t.Errorf("expected error for invalid action")
	}
}

// TestRecordNodeTransition 记录节点流转
func TestRecordNodeTransition(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	transition := &model.SOPNodeTransition{
		SOPID:       1,
		ExecutionID: 100,
		CustomerID:  "c1",
		SessionID:   "s1",
		ToNode:      "node_1",
		NodeType:    "llm",
		Outcome:     model.NodeOutcomeSuccess,
		DurationMs:  1500,
	}
	err := svc.RecordNodeTransition(context.Background(), transition)
	if err != nil {
		t.Fatalf("record: %v", err)
	}

	var count int64
	db.Model(&model.SOPNodeTransition{}).Where("sop_id = ?", 1).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 transition, got %d", count)
	}
}

// TestRecordNodeTransition_Nil 安全性测试
func TestRecordNodeTransition_Nil(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	err := svc.RecordNodeTransition(context.Background(), nil)
	if err != nil {
		t.Errorf("nil transition should not return error, got %v", err)
	}
}

// TestListPendingSuggestions 列出待审核建议
func TestListPendingSuggestions(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	for i := 0; i < 3; i++ {
		sug := model.OptimizationSuggestion{
			SOPID: 1, SOPName: "test", NodeID: "n1",
			SuggestionType: model.SuggestionTypePromptRewrite,
			SuggestionText: "test", Status: model.SuggestionStatusPending,
			Priority: i,
		}
		db.Create(&sug)
	}
	applied := model.OptimizationSuggestion{
		SOPID: 1, SOPName: "test", NodeID: "n2",
		SuggestionType: model.SuggestionTypeBranchPrune,
		SuggestionText: "applied", Status: model.SuggestionStatusApplied,
	}
	db.Create(&applied)

	list, err := svc.ListPendingSuggestions(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 pending suggestions, got %d", len(list))
	}
	for _, s := range list {
		if s.Status != model.SuggestionStatusPending {
			t.Errorf("all should be pending, got %s", s.Status)
		}
	}
}

// TestListPendingSuggestions_PriorityOrder 按优先级排序
func TestListPendingSuggestions_PriorityOrder(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	for _, p := range []int{0, 2, 1} {
		sug := model.OptimizationSuggestion{
			SOPID: 1, SOPName: "test", NodeID: "n1",
			SuggestionType: model.SuggestionTypePromptRewrite,
			SuggestionText: "test", Status: model.SuggestionStatusPending,
			Priority: p,
		}
		db.Create(&sug)
	}

	list, err := svc.ListPendingSuggestions(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].Priority != 2 {
		t.Errorf("expected first priority=2, got %d", list[0].Priority)
	}
	if list[1].Priority != 1 {
		t.Errorf("expected second priority=1, got %d", list[1].Priority)
	}
}

// TestGetLatestProfile 获取最新画像
func TestGetLatestProfile(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	now := time.Now()
	for _, dim := range model.AllSalesChampionDimensions {
		s1 := model.SalesChampionProfileSnapshot{
			StaffID: 1, StaffName: "A", Scenario: "ai_champion",
			Dimension: string(dim), Score: 60, GeneratedAt: now.Add(-2 * time.Hour),
		}
		db.Create(&s1)
		s2 := model.SalesChampionProfileSnapshot{
			StaffID: 1, StaffName: "A", Scenario: "ai_champion",
			Dimension: string(dim), Score: 80, GeneratedAt: now.Add(-1 * time.Hour),
		}
		db.Create(&s2)
	}

	list, err := svc.GetLatestProfile(context.Background(), 1, "ai_champion")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(list) != 5 {
		t.Errorf("expected 5 dimensions, got %d", len(list))
	}
}

// TestClassifyObjectionHandling_Positive 异议处理正向
func TestClassifyObjectionHandling_Positive(t *testing.T) {
	tests := []struct {
		name     string
		aiReply  string
		customer string
		wantHit  bool
		wantPos  bool
	}{
		{"价格异议+解释", "我理解您的顾虑，性价比很高，值得购买。", "太贵了", true, true},
		{"不需要+价值塑造", "其实这款适合您，活动优惠很划算。", "不需要", true, true},
		{"考虑+限时", "今天活动优惠，帮您锁定名额。", "考虑一下", true, true},
		{"异议+放弃", "好的，再见", "太贵了", true, false},
		{"无异议不命中", "您好，请问有什么可以帮您？", "你好", false, false},
		{"竞品对比", "对比下来我们性价比更高，适合您。", "别人家更便宜", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, ev := classifyObjectionHandling(tt.aiReply, tt.customer)
			if hit != tt.wantHit {
				t.Errorf("hit=%v, want %v", hit, tt.wantHit)
			}
			if hit && ev.positive != tt.wantPos {
				t.Errorf("positive=%v, want %v", ev.positive, tt.wantPos)
			}
		})
	}
}

// TestClassifyClosingInvitation 逼单邀约分类
func TestClassifyClosingInvitation(t *testing.T) {
	tests := []struct {
		name     string
		aiReply  string
		customer string
		wantHit  bool
		wantPos  bool
	}{
		{"下单+客户同意", "今天下单可以享受 8 折，帮您安排。", "好的，下单", true, true},
		{"预约+客户同意", "帮您预约明天到店体验。", "可以，安排吧", true, true},
		{"逼单+客户拒绝", "马上付款锁定名额。", "再看看吧", true, false},
		{"无逼单不命中", "这款产品 199 元。", "多少钱", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, ev := classifyClosingInvitation(tt.aiReply, tt.customer)
			if hit != tt.wantHit {
				t.Errorf("hit=%v, want %v", hit, tt.wantHit)
			}
			if hit && ev.positive != tt.wantPos {
				t.Errorf("positive=%v, want %v", ev.positive, tt.wantPos)
			}
		})
	}
}

// TestClassifyFollowupActivation 跟进激活分类
func TestClassifyFollowupActivation(t *testing.T) {
	tests := []struct {
		name    string
		aiReply string
		wantHit bool
		wantPos bool
	}{
		{"新品跟进", "好久没联系，新品到货，帮您留一份。", true, true},
		{"活动跟进", "最近有活动优惠，适合您。", true, true},
		{"催促跟进", "怎么不回复，赶紧下单。", true, false},
		{"无跟进不命中", "您好，请问有什么可以帮您？", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, ev := classifyFollowupActivation(tt.aiReply, "")
			if hit != tt.wantHit {
				t.Errorf("hit=%v, want %v", hit, tt.wantHit)
			}
			if hit && ev.positive != tt.wantPos {
				t.Errorf("positive=%v, want %v", ev.positive, tt.wantPos)
			}
		})
	}
}

// TestClassifyNurturingConversion 培育转化分类
func TestClassifyNurturingConversion(t *testing.T) {
	tests := []struct {
		name    string
		aiReply string
		wantHit bool
		wantPos bool
	}{
		{"个性化推荐", "根据您之前的需求，我为您推荐专属方案。", true, true},
		{"定制方案", "了解您的需求，帮您安排定制方案。", true, true},
		{"培育无推进", "上次我们聊过。", true, false},
		{"无培育不命中", "您好，请问有什么可以帮您？", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, ev := classifyNurturingConversion(tt.aiReply, "")
			if hit != tt.wantHit {
				t.Errorf("hit=%v, want %v", hit, tt.wantHit)
			}
			if hit && ev.positive != tt.wantPos {
				t.Errorf("positive=%v, want %v", ev.positive, tt.wantPos)
			}
		})
	}
}

// TestClassifyRepurchaseOperation 复购运营分类
func TestClassifyRepurchaseOperation(t *testing.T) {
	tests := []struct {
		name    string
		aiReply string
		wantHit bool
		wantPos bool
	}{
		{"会员权益", "感谢老朋友，会员积分可以抵扣，复购专属优惠。", true, true},
		{"老客福利", "老用户专享福利，帮您安排。", true, true},
		{"复购触达", "老朋友，再次购买有惊喜。", true, true},
		{"无复购不命中", "您好，请问有什么可以帮您？", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hit, ev := classifyRepurchaseOperation(tt.aiReply, "")
			if hit != tt.wantHit {
				t.Errorf("hit=%v, want %v", hit, tt.wantHit)
			}
			if hit && ev.positive != tt.wantPos {
				t.Errorf("positive=%v, want %v", ev.positive, tt.wantPos)
			}
		})
	}
}

// TestPRD_Acceptance_ExtractDimensionsFromDialog 验收：销冠对话可自动提取能力维度
func TestPRD_Acceptance_ExtractDimensionsFromDialog(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	baseTime := time.Now().Add(-10 * time.Hour)
	seedCustomerMessages(db, "s1", []string{"太贵了，不需要"}, baseTime)
	seedAIMessages(db, "s1", []string{
		"我理解您的顾虑，其实性价比很高，活动优惠很划算，值得购买。",
	}, baseTime.Add(30*time.Second))

	seedCustomerMessages(db, "s2", []string{"可以的，下单吧"}, baseTime.Add(1*time.Hour))
	seedAIMessages(db, "s2", []string{
		"好的，今天下单享受 8 折，帮您安排。",
	}, baseTime.Add(1*time.Hour+30*time.Second))

	seedAIMessages(db, "s3", []string{
		"好久没联系，最近新品到货，活动优惠适合您，帮您留一份。",
	}, baseTime.Add(2*time.Hour))

	seedAIMessages(db, "s4", []string{
		"根据您之前的需求，我为您推荐专属方案，建议安排体验。",
	}, baseTime.Add(3*time.Hour))

	seedAIMessages(db, "s5", []string{
		"感谢老朋友，会员积分可抵扣，复购再享专属福利。",
	}, baseTime.Add(4*time.Hour))

	report, err := svc.ExtractProfile(context.Background(), 0, "智能体", "ai_champion", baseTime, time.Now())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	for _, dim := range report.Dimensions {
		if dim.SampleCount == 0 {
			t.Errorf("验收失败：维度 %s 未提取到样本", dim.Dimension)
		}
	}
}

// TestPRD_Acceptance_NodeConversionStats 验收：SOP 节点转化率可统计
func TestPRD_Acceptance_NodeConversionStats(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "验收SOP")
	for i := 0; i < 8; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "start", "start", model.NodeOutcomeSuccess, 100)
	}
	for i := 0; i < 6; i++ {
		seedNodeTransition(db, sopID, uint(i+9), "llm_1", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(i+15), "llm_1", "llm", model.NodeOutcomeAbandoned, 500)
	}
	for i := 0; i < 3; i++ {
		seedNodeTransition(db, sopID, uint(i+17), "end", "end", model.NodeOutcomeSuccess, 50)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if len(stats.Nodes) == 0 {
		t.Errorf("验收失败：未统计到节点")
	}
	for _, n := range stats.Nodes {
		if n.ConversionRate < 0 || n.ConversionRate > 100 {
			t.Errorf("验收失败：节点 %s 转化率越界 %.2f", n.NodeID, n.ConversionRate)
		}
	}
}

// TestPRD_Acceptance_LowConversionSuggestion 验收：低转化节点自动生成优化建议
func TestPRD_Acceptance_LowConversionSuggestion(t *testing.T) {
	db := setupFeedbackLearningDB(t)
	svc := NewFeedbackLearningService(db)

	sopID := seedSOPAgent(db, "验收SOP")
	for i := 0; i < 2; i++ {
		seedNodeTransition(db, sopID, uint(i+1), "bad_llm", "llm", model.NodeOutcomeSuccess, 1000)
	}
	for i := 0; i < 8; i++ {
		seedNodeTransition(db, sopID, uint(i+3), "bad_llm", "llm", model.NodeOutcomeAbandoned, 500)
	}

	stats, err := svc.AnalyzeNodeConversion(context.Background(), sopID, "")
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	suggestions, err := svc.GenerateOptimizationSuggestions(context.Background(), OptimizationSuggestionInput{
		SOPID:          sopID,
		SOPName:        "验收SOP",
		NodeConversion: stats,
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(suggestions) == 0 {
		t.Errorf("验收失败：低转化节点未生成优化建议")
	}
	for _, sug := range suggestions {
		if sug.SuggestionText == "" {
			t.Errorf("验收失败：建议内容为空")
		}
		if sug.CurrentScore >= 50 {
			t.Errorf("验收失败：建议的当前得分应 < 50，实际 %.2f", sug.CurrentScore)
		}
	}
}

// findDimension 在报告中查找指定维度
func findDimension(report *ChampionProfileReport, dim model.SalesChampionDimension) ChampionDimensionScore {
	for _, d := range report.Dimensions {
		if d.Dimension == dim {
			return d
		}
	}
	return ChampionDimensionScore{}
}
