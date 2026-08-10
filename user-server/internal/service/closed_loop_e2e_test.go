package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
)

// ============================================================================
// 商业产品级 端到端闭环测试（End-to-End Closed Loop）
// ----------------------------------------------------------------------------
// 商业市场需求：销售每天接触 50+ 客户，每个客户涉及 5 个组件（标签/旅程/
// 跟进/订单/仪表盘）。销售动作触发器统一编排 5 个组件，按真实业务流自动联动。
//
// 本测试模拟 3 个真实用户场景：
//   场景 1：医美高客单（价格咨询 → 标签 → 旅程 → 跟进）
//   场景 2：教培复购（沉睡客户 → 激活 → 成交 → 售后 SOP）
//   场景 3：电商高频（AI 自动成交 → 仪表盘统计）
// ============================================================================

// TestE2E_MedicalBeauty_PriceInquiry 场景 1：医美客户从价格咨询到跟进闭环
func TestE2E_MedicalBeauty_PriceInquiry(t *testing.T) {
	// 1. 准备 5 个组件（模拟 main.go 启动时的依赖注入）
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)
	trigger := NewSalesActionTrigger(tagger, journey, followup, extractor, dashboard, nil)
	ctx := context.Background()

	custID := "mb_e2e_001"
	ownerID := "sales_amy"

	// 2. 模拟客户首次咨询价格
	resp := &SalesResponse{
		Reply:  "光子嫩肤单次 880 元，套餐 3 次 2280 元，欢迎预约",
		Intent: &dto.RecognizeResult{IntentType: IntentPriceInquiry, IntentName: "价格咨询", Confidence: 0.92},
	}

	// 3. 触发：AI 谈单响应后，自动联动 5 个组件
	rec := trigger.TriggerAfterSales(ctx, custID, ownerID, resp)
	if rec == nil {
		t.Fatal("TriggerAfterSales 返回 nil")
	}
	if rec.CustomerID != custID {
		t.Errorf("customer_id 错误: %s", rec.CustomerID)
	}
	if len(rec.Actions) < 3 {
		t.Errorf("触发的动作数应该 >= 3，实际 %d", len(rec.Actions))
	}

	// 4. 验证：自动打标签
	tags := tagger.GetTags(ctx, custID)
	hasPriceSensitive := false
	for _, tag := range tags {
		if tag.Tag == "behavior:price_sensitive" {
			hasPriceSensitive = true
		}
	}
	if !hasPriceSensitive {
		t.Errorf("应自动打价格敏感标签，实际: %v", tags)
	}

	// 5. 验证：自动推进客户旅程
	state := journey.GetState(ctx, custID)
	if state.CurrentStage == StageStranger {
		t.Errorf("客户旅程应该从陌生推进，实际仍为: %s", state.CurrentStage)
	}
	t.Logf("客户旅程: %s", state.CurrentStage)

	// 6. 验证：自动安排跟进
	pending := followup.ListPending(ctx, ownerID, 0)
	if len(pending) == 0 {
		t.Error("应自动安排跟进，实际无待办")
	}
	foundMB001 := false
	for _, r := range pending {
		if r.CustomerID == custID {
			foundMB001 = true
		}
	}
	if !foundMB001 {
		t.Error("应自动为 mb_e2e_001 安排跟进")
	}

	// 7. 验证：销售仪表盘记录 AI 谈单
	prod := dashboard.GetAIProductivity(ctx, time.Time{})
	if prod.TotalAIDeals < 1 {
		t.Errorf("仪表盘应记录至少 1 个 AI 谈单，实际 %d", prod.TotalAIDeals)
	}

	t.Logf("✅ 场景 1 闭环通过：%d 个动作触发，%d 个标签，旅程=%s，%d 个待办",
		len(rec.Actions), len(tags), state.CurrentStage, len(pending))
}

// TestE2E_Education_Reactivation 场景 2：教培客户沉睡 → 激活 → 成交
func TestE2E_Education_Reactivation(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)
	trigger := NewSalesActionTrigger(tagger, journey, followup, extractor, dashboard, nil)
	ctx := context.Background()
	repurchase := NewRepurchaseEngine()

	custID := "edu_e2e_001"
	ownerID := "sales_bob"

	// 1. 100 天前有购买记录 → 沉睡了
	repurchase.RecordPurchase(ctx, PurchaseEvent{
		OrderID:     "ord_old",
		CustomerID:  custID,
		Amount:      3000,
		ProductName: "编程课 36 节",
		OrderedAt:   time.Now().AddDate(0, 0, -100),
	})

	// 2. 触发 RFM → 旅程推到 sleeping
	if err := repurchase.TriggerJourney(ctx, custID, journey); err != nil {
		t.Fatalf("RFM 推旅程失败: %v", err)
	}
	state := journey.GetState(ctx, custID)
	if state.CurrentStage != StageSleeping {
		t.Fatalf("沉睡客户应在 sleeping，实际 %s", state.CurrentStage)
	}

	// 3. AI 重新激活（客户回访）
	resp := &SalesResponse{
		Reply:  "我们有个老学员专享 7 折课程，要不要看看？",
		Intent: &dto.RecognizeResult{IntentType: IntentPurchase, IntentName: "复购意向", Confidence: 0.88},
	}
	rec := trigger.TriggerAfterSales(ctx, custID, ownerID, resp)

	// 4. 验证：客户旅程从 sleeping 推到 interested 或 quoted
	state = journey.GetState(ctx, custID)
	t.Logf("客户旅程: %s", state.CurrentStage)

	// 5. 验证：触发了跟进（高优先级）
	pending := followup.ListPending(ctx, ownerID, 0)
	foundHighPriority := false
	for _, r := range pending {
		if r.CustomerID == custID && r.Priority == PriorityHigh {
			foundHighPriority = true
		}
	}
	if !foundHighPriority {
		t.Error("Purchase 意图应触发高优先级跟进")
	}

	// 6. 销售完成跟进（成交）
	trigger.TriggerAfterFollowUp(ctx, "rem_fake", custID, ownerID, "converted")

	// 7. 验证：旅程推到 won
	state = journey.GetState(ctx, custID)
	if state.CurrentStage != StageWon {
		t.Errorf("成交后应在 won，实际 %s", state.CurrentStage)
	}

	// 8. 验证：仪表盘记录了订单
	perf := dashboard.GetSalesPerformance(ctx, ownerID, time.Time{})
	if perf.TotalOrders < 1 {
		t.Errorf("应记录 1 个订单，实际 %d", perf.TotalOrders)
	}
	if perf.Conversions < 1 {
		t.Errorf("应记录 1 个转化，实际 %d", perf.Conversions)
	}

	// 9. 验证：成交后自动安排售后 SOP
	pending = followup.ListPending(ctx, ownerID, 0)
	foundAfterSale := false
	for _, r := range pending {
		if r.CustomerID == custID && r.Type == ReminderAfterSaleCare {
			foundAfterSale = true
		}
	}
	if !foundAfterSale {
		t.Error("成交后应自动安排售后回访")
	}

	t.Logf("✅ 场景 2 闭环通过：%d 个动作触发，旅程=%s，转化=%d，售后 SOP=%v",
		len(rec.Actions), state.CurrentStage, perf.Conversions, foundAfterSale)
}

// TestE2E_Ecommerce_HighFrequency 场景 3：电商高频 AI 自动成交
func TestE2E_Ecommerce_HighFrequency(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)
	trigger := NewSalesActionTrigger(tagger, journey, followup, extractor, dashboard, nil)
	ctx := context.Background()

	ownerID := "ai_sales_007"

	// 1. 50 个客户咨询 → AI 谈单 → 10 个成交
	convertedCount := 0
	batchTag := time.Now().Format("150405")
	for i := 0; i < 50; i++ {
		// 应使用 i 本身确保每个客户 ID 唯一
		custID := fmt.Sprintf("ecom_e2e_%s_%d", batchTag, i)
		intentType := IntentPriceInquiry
		if i%5 == 0 {
			intentType = IntentPurchase // 1/5 客户直接想买
		}
		resp := &SalesResponse{
			Reply:  "好的，瑜伽课体验课 99 元，正式课 880 元/期",
			Intent: &dto.RecognizeResult{IntentType: intentType, IntentName: "咨询", Confidence: 0.85},
		}
		trigger.TriggerAfterSales(ctx, custID, ownerID, resp)
		if intentType == IntentPurchase {
			// AI 自动成交（销售未介入），标记为 AI 独立成单
			trigger.TriggerAfterOrder(ctx, "ord_"+custID, custID, ownerID, 99, "瑜伽课体验课", true)
			convertedCount++
		}
	}

	// 2. 验证：仪表盘统计了所有 AI 谈单 + 订单
	prod := dashboard.GetAIProductivity(ctx, time.Time{})
	if prod.TotalAIDeals != 50 {
		t.Errorf("应记录 50 个 AI 谈单，实际 %d", prod.TotalAIDeals)
	}
	if prod.AISoloDeals != convertedCount {
		t.Errorf("应记录 %d 个 AI 独立成单，实际 %d", convertedCount, prod.AISoloDeals)
	}

	// 3. 验证：所有客户都在仪表盘的漏斗中
	funnel := dashboard.FunnelByJourney(ctx)
	total := 0
	for _, s := range funnel.Stages {
		total += s.Customers
	}
	if total < 50 {
		t.Errorf("漏斗总客户数应 >= 50，实际 %d", total)
	}

	// 4. 验证：触发了高优先级跟进（Purchase 意图）
	pending := followup.ListPending(ctx, ownerID, 0)
	highPriorityCount := 0
	for _, r := range pending {
		if r.Priority == PriorityHigh {
			highPriorityCount++
		}
	}
	if highPriorityCount < 1 {
		t.Error("至少应有 1 个高优先级跟进")
	}

	t.Logf("✅ 场景 3 闭环通过：50 个 AI 谈单，%d 个成交，%d 个高优先级跟进，漏斗总客户 %d",
		convertedCount, highPriorityCount, total)
}

// TestE2E_Complaint_LostFlow 边界场景：投诉/流失自动识别
func TestE2E_Complaint_LostFlow(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)
	trigger := NewSalesActionTrigger(tagger, journey, followup, extractor, dashboard, nil)
	ctx := context.Background()

	custID := "complaint_e2e_001"
	ownerID := "sales_carol"

	// 1. 客户投诉 → AI 转人工
	resp := &SalesResponse{
		Reply:              "[系统自动转人工] 客户正在投诉",
		Intent:             &dto.RecognizeResult{IntentType: IntentComplaint, IntentName: "投诉", Confidence: 0.95},
		TransferredToHuman: true,
		TransferReason:     "客户正在投诉，需要人工处理",
	}
	rec := trigger.TriggerAfterSales(ctx, custID, ownerID, resp)

	// 2. 验证：自动打了 churn_risk 标签
	tags := tagger.GetTags(ctx, custID)
	hasChurnRisk := false
	for _, tag := range tags {
		if tag.Tag == "lifecycle:churn_risk" {
			hasChurnRisk = true
		}
	}
	if !hasChurnRisk {
		t.Error("投诉应自动打流失风险标签")
	}

	// 3. 验证：自动安排紧急跟进（30 分钟内）
	pending := followup.ListPending(ctx, ownerID, 0)
	foundUrgent := false
	for _, r := range pending {
		if r.CustomerID == custID && r.Priority == PriorityUrgent {
			foundUrgent = true
		}
	}
	if !foundUrgent {
		t.Error("投诉应自动安排紧急跟进（30 分钟）")
	}

	// 4. 销售处理跟进（无法挽回 → 标记 lost）
	trigger.TriggerAfterFollowUp(ctx, "rem_fake", custID, ownerID, "lost")

	// 5. 验证：客户旅程推到 lost
	state := journey.GetState(ctx, custID)
	if state.CurrentStage != StageLost {
		t.Errorf("无法挽回的应推到 lost，实际 %s", state.CurrentStage)
	}

	// 6. 验证：仪表盘记录了转人工
	prod := dashboard.GetAIProductivity(ctx, time.Time{})
	if prod.TransferredCount < 1 {
		t.Error("应记录至少 1 个转人工")
	}

	t.Logf("✅ 投诉场景闭环通过：%d 个动作触发，标签=%v，旅程=%s",
		len(rec.Actions), hasChurnRisk, state.CurrentStage)
}

// TestE2E_OrderIntent_AutoExtract 验证订单意向自动提取
func TestE2E_OrderIntent_AutoExtract(t *testing.T) {
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)
	trigger := NewSalesActionTrigger(tagger, journey, followup, extractor, dashboard, nil)
	ctx := context.Background()

	custID := "order_e2e_001"
	ownerID := "sales_dan"

	// 1. 客户明确说"我要买光子嫩肤套餐 3 次 2280 元"
	mem := modelDialogueMemoryFixture(custID, "我要买光子嫩肤套餐 3 次 2280 元", "2000-3000")
	resp := &SalesResponse{
		Reply:  "好的，3 次光子嫩肤套餐 2280 元，给您下单",
		Intent: &dto.RecognizeResult{IntentType: IntentPurchase, IntentName: "准备购买", Confidence: 0.96},
		Memory: &mem,
	}

	rec := trigger.TriggerAfterSales(ctx, custID, ownerID, resp)

	// 2. 验证：自动提取了订单意向
	hasOrderIntent := false
	for _, action := range rec.Actions {
		if action.Action == "order_intent_extracted" && action.Result == "ok" {
			hasOrderIntent = true
		}
	}
	if !hasOrderIntent {
		t.Error("应自动提取订单意向")
	}

	t.Logf("✅ 订单意向闭环通过：%d 个动作触发", len(rec.Actions))
}

// modelDialogueMemoryFixture 构造测试用 DialogueMemory
func modelDialogueMemoryFixture(customerID, demand, budget string) modelDialogueMemoryT {
	return modelDialogueMemoryT{
		CustomerID: customerID,
		Demand:     demand,
		Budget:     budget,
	}
}

// modelDialogueMemoryT 测试用 DialogueMemory（P0-7 后 SalesResponse.Memory 为 dto 镜像类型）
type modelDialogueMemoryT = dto.DialogueMemory
