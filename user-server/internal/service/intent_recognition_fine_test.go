package service

// intent_recognition_fine_test.go 精细意图识别（8 大类 + 7 子类）测试
//
// 五层架构归属: L2 服务层测试
// 设计依据: PRD § 缺口修复
// 私域独立部署: 无 merchant_id 字段
//
// 覆盖范围：
//   - 8 大意图类规则匹配（consult/price_inquiry/objection/after_sale/complaint/churn/intent_buy/ask_product）
//   - 每个大类下的 7 子类细分（共 26 个子类）
//   - confidence < 0.6 触发 LLM 二次识别（用 dispatcher=nil 跳过 LLM）
//   - IntentLog 持久化
//   - GetIntentLogs / GetIntentLogStats / QueryIntentLogsByTraceID 查询

import (
	"context"
	"testing"
	"time"

	"marketing/internal/model"
)

// ===== 8 大类规则匹配测试（每类至少 3 个用例） =====

// 1. consult - general
func TestRecognizeIntent_ConsultGeneral(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, err := rec.RecognizeIntent(context.Background(), "我想咨询一下", "c-1", "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Major != IntentMajorConsult {
		t.Errorf("expected consult, got %s", r.Major)
	}
	if r.Method != "rule" {
		t.Errorf("expected rule method, got %s", r.Method)
	}
}

// 2. consult - product_specific
func TestRecognizeIntent_ConsultProductSpecific(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个产品怎么用", "c-1", "s-1")
	if r.Major != IntentMajorConsult {
		t.Errorf("expected consult, got %s", r.Major)
	}
	if r.Minor != IntentMinorConsultProductSpecific {
		t.Errorf("expected product_specific, got %s", r.Minor)
	}
}

// 3. consult - comparison
func TestRecognizeIntent_ConsultComparison(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这两款产品对比一下区别", "c-1", "s-1")
	if r.Major != IntentMajorConsult {
		t.Errorf("expected consult, got %s", r.Major)
	}
	if r.Minor != IntentMinorConsultComparison {
		t.Errorf("expected comparison, got %s", r.Minor)
	}
}

// 4. price_inquiry - budget_check
func TestRecognizeIntent_PriceBudgetCheck(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个多少钱", "c-1", "s-1")
	if r.Major != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.Major)
	}
	if r.Minor != IntentMinorPriceBudgetCheck {
		t.Errorf("expected budget_check, got %s", r.Minor)
	}
}

// 5. price_inquiry - discount_request
func TestRecognizeIntent_PriceDiscountRequest(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "有什么优惠活动吗", "c-1", "s-1")
	if r.Major != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.Major)
	}
	if r.Minor != IntentMinorPriceDiscountReq {
		t.Errorf("expected discount_request, got %s", r.Minor)
	}
}

// 6. price_inquiry - payment_terms
func TestRecognizeIntent_PricePaymentTerms(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "支持分期付款吗", "c-1", "s-1")
	if r.Major != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.Major)
	}
	if r.Minor != IntentMinorPricePaymentTerms {
		t.Errorf("expected payment_terms, got %s", r.Minor)
	}
}

// 7. objection - price_too_high
func TestRecognizeIntent_ObjectionPriceHigh(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个东西太贵了", "c-1", "s-1")
	if r.Major != IntentMajorObjection {
		t.Errorf("expected objection, got %s", r.Major)
	}
	if r.Minor != IntentMinorObjectionPriceHigh {
		t.Errorf("expected price_too_high, got %s", r.Minor)
	}
}

// 8. objection - trust_issue
func TestRecognizeIntent_ObjectionTrustIssue(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "你们靠谱吗", "c-1", "s-1")
	if r.Major != IntentMajorObjection {
		t.Errorf("expected objection, got %s", r.Major)
	}
	if r.Minor != IntentMinorObjectionTrustIssue {
		t.Errorf("expected trust_issue, got %s", r.Minor)
	}
}

// 9. objection - timing_bad
func TestRecognizeIntent_ObjectionTimingBad(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "过段时间再说吧", "c-1", "s-1")
	if r.Major != IntentMajorObjection {
		t.Errorf("expected objection, got %s", r.Major)
	}
	if r.Minor != IntentMinorObjectionTimingBad {
		t.Errorf("expected timing_bad, got %s", r.Minor)
	}
}

// 10. objection - competitor_comparison
func TestRecognizeIntent_ObjectionCompetitor(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "别家比你们便宜", "c-1", "s-1")
	if r.Major != IntentMajorObjection {
		t.Errorf("expected objection, got %s", r.Major)
	}
	if r.Minor != IntentMinorObjectionCompetitorCmp {
		t.Errorf("expected competitor_comparison, got %s", r.Minor)
	}
}

// 11. after_sale - quality_issue
func TestRecognizeIntent_AfterSaleQuality(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "买的东西坏了", "c-1", "s-1")
	if r.Major != IntentMajorAfterSale {
		t.Errorf("expected after_sale, got %s", r.Major)
	}
	if r.Minor != IntentMinorAfterSaleQuality {
		t.Errorf("expected quality_issue, got %s", r.Minor)
	}
}

// 12. after_sale - delivery_issue
func TestRecognizeIntent_AfterSaleDelivery(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "快递多久能到", "c-1", "s-1")
	if r.Major != IntentMajorAfterSale {
		t.Errorf("expected after_sale, got %s", r.Major)
	}
	if r.Minor != IntentMinorAfterSaleDelivery {
		t.Errorf("expected delivery_issue, got %s", r.Minor)
	}
}

// 13. after_sale - refund_request
func TestRecognizeIntent_AfterSaleRefund(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "我要退货退款", "c-1", "s-1")
	if r.Major != IntentMajorAfterSale {
		t.Errorf("expected after_sale, got %s", r.Major)
	}
	if r.Minor != IntentMinorAfterSaleRefund {
		t.Errorf("expected refund_request, got %s", r.Minor)
	}
}

// 14. after_sale - warranty
func TestRecognizeIntent_AfterSaleWarranty(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个保修期多久", "c-1", "s-1")
	if r.Major != IntentMajorAfterSale {
		t.Errorf("expected after_sale, got %s", r.Major)
	}
	if r.Minor != IntentMinorAfterSaleWarranty {
		t.Errorf("expected warranty, got %s", r.Minor)
	}
}

// 15. complaint - service_complaint
func TestRecognizeIntent_ComplaintService(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "你们客服服务态度太差了", "c-1", "s-1")
	if r.Major != IntentMajorComplaint {
		t.Errorf("expected complaint, got %s", r.Major)
	}
	if r.Minor != IntentMinorComplaintService {
		t.Errorf("expected service_complaint, got %s", r.Minor)
	}
}

// 16. complaint - product_complaint
func TestRecognizeIntent_ComplaintProduct(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个产品质量差是次品", "c-1", "s-1")
	if r.Major != IntentMajorComplaint {
		t.Errorf("expected complaint, got %s", r.Major)
	}
	if r.Minor != IntentMinorComplaintProduct {
		t.Errorf("expected product_complaint, got %s", r.Minor)
	}
}

// 17. complaint - billing_complaint
func TestRecognizeIntent_ComplaintBilling(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "你们乱扣费多扣钱了", "c-1", "s-1")
	if r.Major != IntentMajorComplaint {
		t.Errorf("expected complaint, got %s", r.Major)
	}
	if r.Minor != IntentMinorComplaintBilling {
		t.Errorf("expected billing_complaint, got %s", r.Minor)
	}
}

// 18. churn - cancel_subscription
func TestRecognizeIntent_ChurnCancelSub(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "我要取消订阅退订", "c-1", "s-1")
	if r.Major != IntentMajorChurn {
		t.Errorf("expected churn, got %s", r.Major)
	}
	if r.Minor != IntentMinorChurnCancelSub {
		t.Errorf("expected cancel_subscription, got %s", r.Minor)
	}
}

// 19. churn - switch_competitor
func TestRecognizeIntent_ChurnSwitchCompetitor(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "我换别家用竞品了", "c-1", "s-1")
	if r.Major != IntentMajorChurn {
		t.Errorf("expected churn, got %s", r.Major)
	}
	if r.Minor != IntentMinorChurnSwitchComp {
		t.Errorf("expected switch_competitor, got %s", r.Minor)
	}
}

// 20. churn - stop_using
func TestRecognizeIntent_ChurnStopUsing(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "别再发了再发举报", "c-1", "s-1")
	if r.Major != IntentMajorChurn {
		t.Errorf("expected churn, got %s", r.Major)
	}
	if r.Minor != IntentMinorChurnStopUsing {
		t.Errorf("expected stop_using, got %s", r.Minor)
	}
}

// 21. intent_buy - ready_to_buy
func TestRecognizeIntent_IntentBuyReady(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "我要下单购买", "c-1", "s-1")
	if r.Major != IntentMajorIntentBuy {
		t.Errorf("expected intent_buy, got %s", r.Major)
	}
	if r.Minor != IntentMinorIntentBuyReady {
		t.Errorf("expected ready_to_buy, got %s", r.Minor)
	}
}

// 22. intent_buy - need_more_info
func TestRecognizeIntent_IntentBuyNeedInfo(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "我还需要更多信息确认一下", "c-1", "s-1")
	if r.Major != IntentMajorIntentBuy {
		t.Errorf("expected intent_buy, got %s", r.Major)
	}
	if r.Minor != IntentMinorIntentBuyNeedInfo {
		t.Errorf("expected need_more_info, got %s", r.Minor)
	}
}

// 23. intent_buy - need_approval
func TestRecognizeIntent_IntentBuyApproval(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "需要老板审批同意", "c-1", "s-1")
	if r.Major != IntentMajorIntentBuy {
		t.Errorf("expected intent_buy, got %s", r.Major)
	}
	if r.Minor != IntentMinorIntentBuyApproval {
		t.Errorf("expected need_approval, got %s", r.Minor)
	}
}

// 24. ask_product - feature_query
func TestRecognizeIntent_AskProductFeature(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个产品有什么功能特点", "c-1", "s-1")
	if r.Major != IntentMajorAskProduct {
		t.Errorf("expected ask_product, got %s", r.Major)
	}
	if r.Minor != IntentMinorAskProductFeature {
		t.Errorf("expected feature_query, got %s", r.Minor)
	}
}

// 25. ask_product - availability
func TestRecognizeIntent_AskProductAvailability(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个有货吗有现货吗", "c-1", "s-1")
	if r.Major != IntentMajorAskProduct {
		t.Errorf("expected ask_product, got %s", r.Major)
	}
	if r.Minor != IntentMinorAskProductAvail {
		t.Errorf("expected availability, got %s", r.Minor)
	}
}

// 26. ask_product - spec_query
func TestRecognizeIntent_AskProductSpec(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个规格参数是多少", "c-1", "s-1")
	if r.Major != IntentMajorAskProduct {
		t.Errorf("expected ask_product, got %s", r.Major)
	}
	if r.Minor != IntentMinorAskProductSpec {
		t.Errorf("expected spec_query, got %s", r.Minor)
	}
}

// ===== 边界场景测试 =====

// 27. 空消息兜底
func TestRecognizeIntent_EmptyMessage(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, err := rec.RecognizeIntent(context.Background(), "", "c-1", "s-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Major != IntentMajorConsult {
		t.Errorf("expected consult for empty, got %s", r.Major)
	}
	if r.Minor != IntentMinorConsultGeneral {
		t.Errorf("expected general for empty, got %s", r.Minor)
	}
	if r.Confidence > 0.5 {
		t.Errorf("expected low confidence for empty, got %f", r.Confidence)
	}
}

// 28. 未命中规则且无 dispatcher 兜底
func TestRecognizeIntent_NoRuleNoDispatcher(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "asdfqwer", "c-1", "s-1")
	// 未命中规则且无 dispatcher → 兜底 consult/general
	if r.Major != IntentMajorConsult {
		t.Errorf("expected consult fallback, got %s", r.Major)
	}
	if r.Method != "rule" {
		t.Errorf("expected rule method, got %s", r.Method)
	}
}

// 29. 置信度范围校验
func TestRecognizeIntent_ConfidenceRange(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个多少钱", "c-1", "s-1")
	if r.Confidence < 0 || r.Confidence > 1 {
		t.Errorf("confidence out of range: %f", r.Confidence)
	}
}

// 30. LatencyMs 非负
func TestRecognizeIntent_LatencyNonNegative(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r, _ := rec.RecognizeIntent(context.Background(), "这个多少钱", "c-1", "s-1")
	if r.LatencyMs < 0 {
		t.Errorf("latency should be non-negative, got %d", r.LatencyMs)
	}
}

// ===== IntentLog 持久化测试 =====

// 31. IntentLog 异步落库
func TestRecognizeIntent_PersistIntentLog(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	_, _ = rec.RecognizeIntent(context.Background(), "这个多少钱", "c-persist", "s-persist")
	// 等待异步落库
	time.Sleep(200 * time.Millisecond)
	var logs []model.IntentLog
	if err := db.Where("customer_id = ?", "c-persist").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("expected intent log persisted, got 0")
	}
	if logs[0].IntentMajor != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", logs[0].IntentMajor)
	}
	if logs[0].IntentMinor != IntentMinorPriceBudgetCheck {
		t.Errorf("expected budget_check, got %s", logs[0].IntentMinor)
	}
	if logs[0].Method != "rule" {
		t.Errorf("expected rule method, got %s", logs[0].Method)
	}
}

// ===== GetIntentLogs 查询测试 =====

// 32. GetIntentLogs 按 customer_id 过滤
func TestGetIntentLogs_ByCustomerID(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	// 写入测试数据
	now := time.Now()
	db.Create(&model.IntentLog{CustomerID: "c-A", SessionID: "s-A", Message: "msg1",
		IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
		Confidence: 0.9, Method: "rule", Timestamp: now})
	db.Create(&model.IntentLog{CustomerID: "c-B", SessionID: "s-B", Message: "msg2",
		IntentMajor: IntentMajorPriceInquiry, IntentMinor: IntentMinorPriceBudgetCheck,
		Confidence: 0.8, Method: "rule", Timestamp: now})

	logs, err := rec.GetIntentLogs(context.Background(), "c-A", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].CustomerID != "c-A" {
		t.Errorf("expected c-A, got %s", logs[0].CustomerID)
	}
}

// 33. GetIntentLogs 按 major 过滤
func TestGetIntentLogs_ByMajor(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	now := time.Now()
	db.Create(&model.IntentLog{CustomerID: "c-X", SessionID: "s-X", Message: "msg1",
		IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
		Confidence: 0.9, Method: "rule", Timestamp: now})
	db.Create(&model.IntentLog{CustomerID: "c-X", SessionID: "s-X", Message: "msg2",
		IntentMajor: IntentMajorComplaint, IntentMinor: IntentMinorComplaintService,
		Confidence: 0.85, Method: "rule", Timestamp: now})

	logs, err := rec.GetIntentLogs(context.Background(), "", IntentMajorComplaint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 complaint log, got %d", len(logs))
	}
	if logs[0].IntentMajor != IntentMajorComplaint {
		t.Errorf("expected complaint, got %s", logs[0].IntentMajor)
	}
}

// 34. GetIntentLogs limit 截断
func TestGetIntentLogs_LimitTruncation(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		db.Create(&model.IntentLog{CustomerID: "c-L", SessionID: "s-L", Message: "msg",
			IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
			Confidence: 0.9, Method: "rule", Timestamp: now})
	}
	logs, err := rec.GetIntentLogs(context.Background(), "c-L", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs (limit), got %d", len(logs))
	}
}

// 35. GetIntentLogs limit 默认值
func TestGetIntentLogs_LimitDefault(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	logs, err := rec.GetIntentLogs(context.Background(), "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	// 空表也应返回 0 条
	if logs == nil {
		t.Error("expected non-nil logs slice")
	}
}

// ===== GetIntentLogStats 统计测试 =====

// 36. GetIntentLogStats 按 major 聚合
func TestGetIntentLogStats_ByMajor(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	now := time.Now()
	db.Create(&model.IntentLog{CustomerID: "c-1", SessionID: "s-1", Message: "m",
		IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
		Confidence: 0.9, Method: "rule", Timestamp: now})
	db.Create(&model.IntentLog{CustomerID: "c-2", SessionID: "s-2", Message: "m",
		IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
		Confidence: 0.8, Method: "rule", Timestamp: now})
	db.Create(&model.IntentLog{CustomerID: "c-3", SessionID: "s-3", Message: "m",
		IntentMajor: IntentMajorPriceInquiry, IntentMinor: IntentMinorPriceBudgetCheck,
		Confidence: 0.7, Method: "rule", Timestamp: now})

	stats, err := rec.GetIntentLogStats(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	// 检查 by_major / by_minor / by_method 均存在（具体类型由 GORM Scan 决定）
	if stats["by_major"] == nil {
		t.Error("expected by_major to be non-nil")
	}
	if stats["by_minor"] == nil {
		t.Error("expected by_minor to be non-nil")
	}
	if stats["by_method"] == nil {
		t.Error("expected by_method to be non-nil")
	}
	if stats["days"].(int) != 7 {
		t.Errorf("expected 7 days, got %v", stats["days"])
	}
}

// 37. GetIntentLogStats 默认 7 天
func TestGetIntentLogStats_DefaultDays(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	stats, err := rec.GetIntentLogStats(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	days, _ := stats["days"].(int)
	if days != 7 {
		t.Errorf("expected default 7 days, got %d", days)
	}
}

// ===== QueryIntentLogsByTraceID 测试 =====

// 38. QueryIntentLogsByTraceID 按 trace_id 查询
func TestQueryIntentLogsByTraceID(t *testing.T) {
	rec, db := newIntentRecognizer(t)
	now := time.Now()
	db.Create(&model.IntentLog{CustomerID: "c-T", SessionID: "s-T", Message: "m1",
		IntentMajor: IntentMajorConsult, IntentMinor: IntentMinorConsultGeneral,
		Confidence: 0.9, Method: "rule", TraceID: "trace-abc", Timestamp: now})
	db.Create(&model.IntentLog{CustomerID: "c-T", SessionID: "s-T", Message: "m2",
		IntentMajor: IntentMajorPriceInquiry, IntentMinor: IntentMinorPriceBudgetCheck,
		Confidence: 0.8, Method: "rule", TraceID: "trace-abc", Timestamp: now.Add(time.Second)})
	db.Create(&model.IntentLog{CustomerID: "c-T", SessionID: "s-T", Message: "m3",
		IntentMajor: IntentMajorComplaint, IntentMinor: IntentMinorComplaintService,
		Confidence: 0.7, Method: "rule", TraceID: "trace-other", Timestamp: now})

	logs, err := rec.QueryIntentLogsByTraceID(context.Background(), "trace-abc")
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	// 应按 timestamp ASC 排序
	if logs[0].Message != "m1" {
		t.Errorf("expected m1 first, got %s", logs[0].Message)
	}
}

// 39. QueryIntentLogsByTraceID 空 trace_id 返回 nil
func TestQueryIntentLogsByTraceID_EmptyTraceID(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	logs, err := rec.QueryIntentLogsByTraceID(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if logs != nil {
		t.Errorf("expected nil for empty trace_id, got %v", logs)
	}
}

// ===== 工具函数测试 =====

// 40. isValidMajor
func TestIsValidMajor(t *testing.T) {
	if !isValidMajor(IntentMajorConsult) {
		t.Error("consult should be valid")
	}
	if !isValidMajor(IntentMajorPriceInquiry) {
		t.Error("price_inquiry should be valid")
	}
	if !isValidMajor(IntentMajorObjection) {
		t.Error("objection should be valid")
	}
	if !isValidMajor(IntentMajorAfterSale) {
		t.Error("after_sale should be valid")
	}
	if !isValidMajor(IntentMajorComplaint) {
		t.Error("complaint should be valid")
	}
	if !isValidMajor(IntentMajorChurn) {
		t.Error("churn should be valid")
	}
	if !isValidMajor(IntentMajorIntentBuy) {
		t.Error("intent_buy should be valid")
	}
	if !isValidMajor(IntentMajorAskProduct) {
		t.Error("ask_product should be valid")
	}
	if isValidMajor("invalid_major") {
		t.Error("invalid_major should be invalid")
	}
}

// 41. isValidMinor
func TestIsValidMinor(t *testing.T) {
	if !isValidMinor(IntentMajorConsult, IntentMinorConsultGeneral) {
		t.Error("consult/general should be valid")
	}
	if !isValidMinor(IntentMajorPriceInquiry, IntentMinorPriceDiscountReq) {
		t.Error("price_inquiry/discount_request should be valid")
	}
	if !isValidMinor(IntentMajorObjection, IntentMinorObjectionCompetitorCmp) {
		t.Error("objection/competitor_comparison should be valid")
	}
	// major 合法但 minor 不属于
	if isValidMinor(IntentMajorConsult, IntentMinorPriceBudgetCheck) {
		t.Error("consult/budget_check should be invalid (cross-major)")
	}
	// major 非法
	if isValidMinor("invalid_major", IntentMinorConsultGeneral) {
		t.Error("invalid_major/* should be invalid")
	}
}

// 42. getDefaultMinor
func TestGetDefaultMinor(t *testing.T) {
	if got := getDefaultMinor(IntentMajorConsult); got != IntentMinorConsultGeneral {
		t.Errorf("expected general, got %s", got)
	}
	if got := getDefaultMinor(IntentMajorPriceInquiry); got != IntentMinorPriceBudgetCheck {
		t.Errorf("expected budget_check, got %s", got)
	}
	if got := getDefaultMinor(IntentMajorObjection); got != IntentMinorObjectionPriceHigh {
		t.Errorf("expected price_too_high, got %s", got)
	}
	if got := getDefaultMinor("invalid_major"); got != IntentMinorConsultGeneral {
		t.Errorf("expected general fallback, got %s", got)
	}
}

// ===== recognizeFineByRule 单元测试 =====

// 43. recognizeFineByRule 空字符串
func TestRecognizeFineByRule_Empty(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	if r := rec.recognizeFineByRule(context.Background(), ""); r != nil {
		t.Errorf("expected nil for empty, got %+v", r)
	}
}

// 44. recognizeFineByRule 未命中
func TestRecognizeFineByRule_NoMatch(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	if r := rec.recognizeFineByRule(context.Background(), "asdfqwer"); r != nil {
		t.Errorf("expected nil for no match, got %+v", r)
	}
}

// 45. recognizeFineByRule 命中子类
func TestRecognizeFineByRule_HitMinor(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	r := rec.recognizeFineByRule(context.Background(), "我要退货退款")
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if r.Major != IntentMajorAfterSale {
		t.Errorf("expected after_sale, got %s", r.Major)
	}
	if r.Minor != IntentMinorAfterSaleRefund {
		t.Errorf("expected refund_request, got %s", r.Minor)
	}
}

// ===== 多关键词混合测试 =====

// 46. 多关键词同时命中选最高分
func TestRecognizeFineByRule_MultiKeywordBestScore(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	// 同时含价格+优惠，应命中 price_inquiry/discount_request（weight=3）
	r, _ := rec.RecognizeIntent(context.Background(), "价格多少有优惠吗", "c-1", "s-1")
	if r.Major != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.Major)
	}
}

// 47. 大类关键词命中但无子类命中
func TestRecognizeFineByRule_MajorOnlyFallback(t *testing.T) {
	rec, _ := newIntentRecognizer(t)
	// "报价" 是 price_inquiry 大类关键词，未命中具体子类
	r := rec.recognizeFineByRule(context.Background(), "请给我报价")
	if r == nil {
		t.Fatal("expected non-nil result for major-only hit")
	}
	if r.Major != IntentMajorPriceInquiry {
		t.Errorf("expected price_inquiry, got %s", r.Major)
	}
	// 大类命中时应回退到第一个子类
	if r.Minor != IntentMinorPriceBudgetCheck {
		t.Errorf("expected budget_check (first minor), got %s", r.Minor)
	}
}
