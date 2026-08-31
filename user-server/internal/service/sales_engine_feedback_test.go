package service

import (
	"context"
	"testing"

	"hivemtk-user/internal/dto"
)

// TestSalesEngine_RecordFeedback_NilLearner feedbackLearner=nil 时静默跳过
// 商业产品级：未注入反馈学习器时不能影响主链路
func TestSalesEngine_RecordFeedback_NilLearner(t *testing.T) {
	engine := &SalesEngine{}
	resp := &SalesResponse{
		Reply: "您好，请问有什么可以帮您？",
		Steps: make([]dto.SalesStepLog, 0, 9),
	}
	engine.recordFeedback(context.Background(), &SalesRequest{}, resp)
	if len(resp.Steps) != 0 {
		t.Errorf("nil learner 不应追加 step，实际追加 %d 个", len(resp.Steps))
	}
}

// TestSalesEngine_RecordFeedback_NilArgs nil 参数安全
func TestSalesEngine_RecordFeedback_NilArgs(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}
	engine.recordFeedback(context.Background(), nil, nil)
	engine.recordFeedback(context.Background(), &SalesRequest{}, nil)
	engine.recordFeedback(context.Background(), nil, &SalesResponse{})
}

// TestSalesEngine_RecordFeedback_RecordSuccess 正常记录反馈
// 商业产品级：AI 自动回复成功时，决策快照被记录到 FeedbackLearner
func TestSalesEngine_RecordFeedback_RecordSuccess(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}

	resp := &SalesResponse{
		Reply:              "您好，光子嫩肤套餐 3 次 2280 元，现在预约还可以享 9 折优惠。",
		TransferredToHuman: false,
		CostTokens:         156,
		LatencyMs:          1200,
		Intent: &dto.RecognizeResult{
			IntentType: IntentAskProduct,
			Confidence: 0.85,
		},
		MatchedSOP: nil,
		Steps:      make([]dto.SalesStepLog, 0, 9),
	}
	req := &SalesRequest{
		SessionID:  "test_session_001",
		CustomerID: "cust_001",
	}

	engine.recordFeedback(context.Background(), req, resp)

	if len(resp.Steps) != 1 {
		t.Fatalf("应追加 1 个 step，实际 %d", len(resp.Steps))
	}
	if resp.Steps[0].Step != "9_feedback_learn" {
		t.Errorf("step 名错误: %s", resp.Steps[0].Step)
	}
	if resp.Steps[0].Status != "ok" {
		t.Errorf("status 应为 ok: %s", resp.Steps[0].Status)
	}

	stats := fl.GetIntentStats(context.Background(), IntentAskProduct)
	if stats == nil {
		t.Fatal("FeedbackLearner 应记录 intent 统计")
	}
	if stats.TotalCount != 1 {
		t.Errorf("TotalCount 应为 1，实际 %d", stats.TotalCount)
	}
	if stats.AvgConfidence != 0.85 {
		t.Errorf("AvgConfidence 应为 0.85，实际 %.2f", stats.AvgConfidence)
	}
	if stats.FailCount != 1 {
		t.Errorf("FailCount 应为 1（CustomerAccept 默认 false），实际 %d", stats.FailCount)
	}
	if stats.SuccessCount != 0 {
		t.Errorf("SuccessCount 应为 0，实际 %d", stats.SuccessCount)
	}

	t.Logf("✅ 反馈学习记录成功: intent=%s conf=%.2f total=%d",
		stats.IntentType, stats.AvgConfidence, stats.TotalCount)
}

// TestSalesEngine_RecordFeedback_Transferred 转人工场景记录负反馈
// 商业产品级：AI 决策转人工时，记录 Transferred=true 便于后续分析转人工原因
func TestSalesEngine_RecordFeedback_Transferred(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}

	resp := &SalesResponse{
		Reply:              "[系统提示] 该内容已转人工处理",
		TransferredToHuman: true,
		TransferReason:     "内容审核未通过: 价格违规",
		CostTokens:         80,
		LatencyMs:          300,
		Intent: &dto.RecognizeResult{
			IntentType: IntentComplaint,
			Confidence: 0.6,
		},
		Steps: make([]dto.SalesStepLog, 0, 9),
	}
	req := &SalesRequest{
		SessionID:  "test_session_002",
		CustomerID: "cust_002",
	}

	engine.recordFeedback(context.Background(), req, resp)

	if len(resp.Steps) != 1 {
		t.Fatalf("应追加 1 个 step，实际 %d", len(resp.Steps))
	}

	stats := fl.GetIntentStats(context.Background(), IntentComplaint)
	if stats == nil {
		t.Fatal("应记录投诉意图")
	}
	if stats.TotalCount != 1 {
		t.Errorf("TotalCount 应为 1，实际 %d", stats.TotalCount)
	}
	if stats.FailCount != 1 {
		t.Errorf("转人工应记为 FailCount，实际 %d", stats.FailCount)
	}

	t.Logf("✅ 转人工反馈记录: intent=%s reason=%s", IntentComplaint, resp.TransferReason)
}

// TestSalesEngine_RecordFeedback_WithSOP SOP 名正确填充
// 商业产品级：SOP 表现统计需要 SOPName 字段
func TestSalesEngine_RecordFeedback_WithSOP(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}

	sopName := "beauty_inquiry_sop"
	resp := &SalesResponse{
		Reply: "您好，我们的光子嫩肤套餐...",
		Intent: &dto.RecognizeResult{
			IntentType: IntentAskProduct,
			Confidence: 0.9,
		},
		MatchedSOP: &dto.SOPAgent{Name: sopName},
		Steps:      make([]dto.SalesStepLog, 0, 9),
	}
	req := &SalesRequest{
		SessionID:  "test_session_003",
		CustomerID: "cust_003",
	}

	engine.recordFeedback(context.Background(), req, resp)

	sopStats := fl.GetSOPStats(context.Background(), sopName)
	if sopStats == nil {
		t.Fatal("应记录 SOP 统计")
	}
	if sopStats.TotalUsed != 1 {
		t.Errorf("TotalUsed 应为 1，实际 %d", sopStats.TotalUsed)
	}
	if sopStats.PositiveRate != 0 {
		t.Errorf("PositiveRate 应为 0（CustomerAccept 默认 false），实际 %.2f", sopStats.PositiveRate)
	}

	t.Logf("✅ SOP 反馈记录: sop=%s used=%d", sopName, sopStats.TotalUsed)
}

// TestSalesEngine_RecordFeedback_MultipleCalls 多次调用累积统计
// 商业产品级：AI 多轮对话后，FeedbackLearner 应累积统计，为 SuggestConfidenceFloor 提供数据
func TestSalesEngine_RecordFeedback_MultipleCalls(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}

	for i := 0; i < 12; i++ {
		resp := &SalesResponse{
			Reply: "您好...",
			Intent: &dto.RecognizeResult{
				IntentType: IntentAskProduct,
				Confidence: 0.7,
			},
			Steps: make([]dto.SalesStepLog, 0, 9),
		}
		engine.recordFeedback(context.Background(), &SalesRequest{
			SessionID:  "session",
			CustomerID: "cust",
		}, resp)
	}

	stats := fl.GetIntentStats(context.Background(), IntentAskProduct)
	if stats.TotalCount != 12 {
		t.Errorf("TotalCount 应为 12，实际 %d", stats.TotalCount)
	}

	floor := fl.SuggestConfidenceFloor(context.Background(), IntentAskProduct)
	if floor <= 0 {
		t.Errorf("SuggestConfidenceFloor 应 > 0，实际 %.2f", floor)
	}

	t.Logf("✅ 累积统计: total=%d, confidence_floor=%.2f", stats.TotalCount, floor)
}

// TestSalesEngine_RecordFeedback_StepDetail 第 9 步日志详情正确
// 商业产品级：销售仪表盘需要展示第 9 步的决策详情
func TestSalesEngine_RecordFeedback_StepDetail(t *testing.T) {
	fl := NewFeedbackLearner(nil)
	engine := &SalesEngine{feedbackLearner: fl}

	resp := &SalesResponse{
		Reply: "回复内容",
		Intent: &dto.RecognizeResult{
			IntentType: IntentAskProduct,
			Confidence: 0.88,
		},
		MatchedSOP: &dto.SOPAgent{Name: "test_sop"},
		CostTokens: 200,
		LatencyMs:  500,
		Steps:      make([]dto.SalesStepLog, 0, 9),
	}
	engine.recordFeedback(context.Background(), &SalesRequest{
		SessionID:  "s1",
		CustomerID: "c1",
	}, resp)

	if len(resp.Steps) != 1 {
		t.Fatalf("应追加 1 个 step")
	}
	detail := resp.Steps[0].Detail
	checks := []string{"intent=", "conf=", "sop=", "transferred=", "tokens="}
	for _, kw := range checks {
		if !containsStr(detail, kw) {
			t.Errorf("step detail 应包含 %s，实际: %s", kw, detail)
		}
	}
	t.Logf("✅ 第 9 步详情: %s", detail)
}

// containsStr 字符串包含判断（避免和同包其他 contains 冲突）
func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
