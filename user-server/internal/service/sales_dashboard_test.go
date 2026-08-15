package service

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
)



// 场景 1：医美完整旅程
func TestScenario_MedicalBeauty_CompleteJourney(t *testing.T) {
	ctx := context.Background()
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	dashboard := NewSalesDashboard(journey)

	custID := "mb_001"

	tags := tagger.TagFromSalesResponse(context.Background(), custID, &SalesResponse{
		Intent: &dto.RecognizeResult{IntentType: IntentPriceInquiry, Confidence: 0.9},
	})
	if len(tags) == 0 {
		t.Fatal("should tag price_sensitive")
	}

	intents := extractor.ExtractFromText(context.Background(), custID, "我想要光子嫩肤套餐，880元一次")
	if len(intents) == 0 {
		t.Fatal("should extract order intent")
	}
	if intents[0].ProductName != "光子嫩肤" {
		t.Errorf("product mismatch: %s", intents[0].ProductName)
	}

	if _, err := journey.Transition(ctx, custID, StageLead, "ai_chat", "ai", "客户留资", nil); err != nil {
		t.Fatalf("transition failed: %v", err)
	}

	r, err := followup.Schedule(ctx, custID, "sales_01", ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title:    "首次跟进",
		Priority: PriorityHigh,
	})
	if err != nil || r == nil {
		t.Fatalf("schedule failed: %v", err)
	}

	journey.Transition(ctx, custID, StageContact, "manual", "sales_01", "加微成功", nil)
	journey.Transition(ctx, custID, StageInterested, "ai_chat", "ai", "咨询光子嫩肤", nil)

	journey.Transition(ctx, custID, StageQuoted, "manual", "sales_01", "发送报价", nil)
	journey.Transition(ctx, custID, StageWon, "order", "system", "支付成功", nil)

	dashboard.RecordOrder(context.Background(), OrderEvent{
		OrderID:     "ord_001",
		CustomerID:  custID,
		OwnerID:     "sales_01",
		Amount:      880,
		ProductName: "光子嫩肤",
		IsAIHandled: false,
		OrderedAt:   time.Now(),
	})

	dashboard.RecordAIDeal(context.Background(), AIDealEvent{
		CustomerID: custID,
		OwnerID:    "sales_01",
		Intent:     IntentPriceInquiry,
		Replied:    true,
		CostTokens: 1500,
		LatencyMs:  1200,
		OccurredAt: time.Now(),
	})

	state := journey.GetState(context.Background(), custID)
	if state.CurrentStage != StageWon {
		t.Errorf("expected Won, got %s", state.CurrentStage)
	}

	allTags := tagger.GetTags(context.Background(), custID)
	hasPrice := false
	for _, tag := range allTags {
		if tag.Tag == "behavior:price_sensitive" {
			hasPrice = true
		}
	}
	if !hasPrice {
		t.Errorf("should have price_sensitive tag, got %v", allTags)
	}

	pending := followup.ListPending(context.Background(), "sales_01", 0)
	if len(pending) < 1 {
		t.Errorf("should have at least 1 pending followup")
	}
}

// 场景 2：教培客户沉睡激活
func TestScenario_Education_Reactivation(t *testing.T) {
	journey := NewCustomerJourneyService()
	engine := NewRepurchaseEngine()
	dashboard := NewSalesDashboard(journey)

	custID := "edu_001"

	engine.RecordPurchase(context.Background(), PurchaseEvent{
		OrderID:     "ord_2025",
		CustomerID:  custID,
		Amount:      3000,
		ProductName: "编程课 36 节",
		OrderedAt:   time.Now().AddDate(0, 0, -100),
	})

	rfm := engine.ComputeRFM(context.Background(), custID)
	if rfm.Segment != RFMTYPEHibernating && rfm.Segment != RFMTYPEAttention && rfm.Segment != RFMTYPELost {
		t.Errorf("expected Hibernating/Attention/Lost, got %s", rfm.Segment)
	}

	if err := engine.TriggerJourney(context.Background(), custID, journey); err != nil {
		t.Fatalf("trigger journey failed: %v", err)
	}
	state := journey.GetState(context.Background(), custID)
	if rfm.Segment == RFMTYPEAttention || rfm.Segment == RFMTYPEHibernating {
		if state.CurrentStage != StageSleeping {
			t.Errorf("Attention/Hibernating should be sleeping, got %s", state.CurrentStage)
		}
	} else {
		if state.CurrentStage != StageLost {
			t.Errorf("Lost should be lost, got %s", state.CurrentStage)
		}
		return
	}

	plan := engine.GenerateReactivationPlan(context.Background(), custID)
	if len(plan) == 0 {
		t.Error("hibernating should have plan")
	}
	if len(plan) < 3 {
		t.Errorf("expected >= 3 waves, got %d", len(plan))
	}

	dashboard.RecordFollowUp(context.Background(), FollowUpEvent{
		CustomerID: custID,
		OwnerID:    "sales_02",
		Channel:    "wechat",
		IsAI:       true,
		Result:     "no_reply",
		OccurredAt: time.Now(),
	})

	dashboard.RecordFollowUp(context.Background(), FollowUpEvent{
		CustomerID: custID,
		OwnerID:    "sales_02",
		Channel:    "wechat",
		IsAI:       true,
		Result:     "converted",
		OccurredAt: time.Now().Add(14 * 24 * time.Hour),
	})

	dashboard.RecordOrder(context.Background(), OrderEvent{
		OrderID:     "ord_2026",
		CustomerID:  custID,
		OwnerID:     "sales_02",
		Amount:      1500,
		ProductName: "编程课 18 节",
		IsAIHandled: true, 
		OrderedAt:   time.Now().Add(14 * 24 * time.Hour),
	})

	pred := engine.Predict(context.Background(), custID)
	if pred.Probability < 0 {
		t.Error("prediction should not be negative")
	}
}

// 场景 3：销售仪表盘数据验证
func TestScenario_Dashboard_TopPerformers(t *testing.T) {
	journey := NewCustomerJourneyService()
	dashboard := NewSalesDashboard(journey)

	sales := []SalesProfile{
		{SalesID: "s_01", Name: "销冠小张", Team: "A", Tags: []string{"高客单", "异议处理强", "耐心"}},
		{SalesID: "s_02", Name: "销冠小李", Team: "A", Tags: []string{"高客单", "异议处理强"}},
		{SalesID: "s_03", Name: "小王", Team: "A", Tags: []string{"一般"}},
		{SalesID: "s_04", Name: "小赵", Team: "B", Tags: []string{"努力"}},
		{SalesID: "s_05", Name: "小孙", Team: "B", Tags: []string{"新员工"}},
	}
	for _, s := range sales {
		dashboard.RegisterSales(context.Background(), s)
	}

	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 50000, OrderedAt: time.Now()})
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 30000, OrderedAt: time.Now()})
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_02", Amount: 40000, OrderedAt: time.Now()})
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_03", Amount: 5000, OrderedAt: time.Now()})
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_04", Amount: 2000, OrderedAt: time.Now()})

	for i := 0; i < 20; i++ {
		dashboard.RecordFollowUp(context.Background(), FollowUpEvent{
			OwnerID:    "s_01",
			Result:     "converted",
			OccurredAt: time.Now(),
		})
	}
	for i := 0; i < 30; i++ {
		dashboard.RecordFollowUp(context.Background(), FollowUpEvent{
			OwnerID:    "s_02",
			Result:     "no_reply",
			OccurredAt: time.Now(),
		})
	}

	for i := 0; i < 100; i++ {
		dashboard.RecordAIDeal(context.Background(), AIDealEvent{
			OwnerID:     "s_01",
			Replied:     true,
			Transferred: i%5 == 0,
			CostTokens:  1500,
			LatencyMs:   1200,
			OccurredAt:  time.Now(),
		})
	}
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 10000, IsAIHandled: true, OrderedAt: time.Now()})

	rankings := dashboard.GetTeamRanking(context.Background(), time.Time{}, 5)
	if len(rankings) != 5 {
		t.Errorf("expected 5 rankings, got %d", len(rankings))
	}
	if rankings[0].SalesID != "s_01" {
		t.Errorf("top 1 should be s_01, got %s", rankings[0].SalesID)
	}
	if rankings[0].TotalRevenue != 90000 {
		t.Errorf("s_01 should have 90000 revenue, got %f", rankings[0].TotalRevenue)
	}

	prod := dashboard.GetAIProductivity(context.Background(), time.Time{})
	if prod.TotalAIDeals != 100 {
		t.Errorf("expected 100 AI deals, got %d", prod.TotalAIDeals)
	}
	if prod.AISoloDeals != 1 {
		t.Errorf("expected 1 AI solo deal, got %d", prod.AISoloDeals)
	}

	champion := dashboard.GetChampionProfile(context.Background(), time.Time{})
	if len(champion.TopPerformers) < 1 {
		t.Error("should have at least 1 top performer")
	}
	if len(champion.CommonTags) == 0 {
		t.Error("top performers should share common tags")
	}
	t.Logf("Champions: %d, Common Tags: %v", len(champion.TopPerformers), champion.CommonTags)
	t.Logf("Insights: %v", champion.Insights)
}

// Sales Dashboard 单元测试
func TestSalesDashboard_FunnelByJourney(t *testing.T) {
	ctx := context.Background()
	journey := NewCustomerJourneyService()
	dashboard := NewSalesDashboard(journey)

	_, _ = journey.Transition(ctx, "c1", StageStranger, "ai", "ai", "", nil)
	_, _ = journey.Transition(ctx, "c2", StageLead, "ai", "ai", "", nil)
	_, _ = journey.Transition(ctx, "c3", StageContact, "ai", "ai", "", nil)
	_, _ = journey.Transition(ctx, "c4", StageInterested, "ai", "ai", "", nil)
	_, _ = journey.Transition(ctx, "c5", StageQuoted, "ai", "ai", "", nil)
	_, _ = journey.Transition(ctx, "c6", StageWon, "ai", "ai", "", nil)

	funnel := dashboard.FunnelByJourney(context.Background())
	if funnel == nil {
		t.Fatal("funnel should not be nil")
	}
	if len(funnel.Stages) < 5 {
		t.Errorf("expected >= 5 stages, got %d", len(funnel.Stages))
	}
	if funnel.TotalWon < 1 {
		t.Errorf("expected at least 1 won, got %d", funnel.TotalWon)
	}
}

func TestSalesDashboard_GetSalesPerformance(t *testing.T) {
	journey := NewCustomerJourneyService()
	dashboard := NewSalesDashboard(journey)
	dashboard.RegisterSales(context.Background(), SalesProfile{SalesID: "s_01", Name: "小张", Team: "A"})

	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 1000, OrderedAt: time.Now()})
	dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 2000, OrderedAt: time.Now()})
	dashboard.RecordFollowUp(context.Background(), FollowUpEvent{OwnerID: "s_01", Result: "converted", OccurredAt: time.Now()})
	dashboard.RecordFollowUp(context.Background(), FollowUpEvent{OwnerID: "s_01", Result: "no_reply", OccurredAt: time.Now()})

	perf := dashboard.GetSalesPerformance(context.Background(), "s_01", time.Time{})
	if perf.TotalOrders != 2 {
		t.Errorf("expected 2 orders, got %d", perf.TotalOrders)
	}
	if perf.TotalRevenue != 3000 {
		t.Errorf("expected 3000 revenue, got %f", perf.TotalRevenue)
	}
	if perf.TotalFollowUps != 2 {
		t.Errorf("expected 2 followups, got %d", perf.TotalFollowUps)
	}
	if perf.Conversions != 1 {
		t.Errorf("expected 1 conversion, got %d", perf.Conversions)
	}
}

func TestSalesDashboard_AIProductivity(t *testing.T) {
	journey := NewCustomerJourneyService()
	dashboard := NewSalesDashboard(journey)

	for i := 0; i < 50; i++ {
		dashboard.RecordAIDeal(context.Background(), AIDealEvent{
			OwnerID:     "s_01",
			Replied:     i%2 == 0,
			Transferred: i%10 == 0,
			CostTokens:  1000,
			LatencyMs:   1000,
			OccurredAt:  time.Now(),
		})
	}
	for i := 0; i < 5; i++ {
		dashboard.RecordOrder(context.Background(), OrderEvent{OwnerID: "s_01", Amount: 500, IsAIHandled: true, OrderedAt: time.Now()})
	}
	for i := 0; i < 100; i++ {
		dashboard.RecordFollowUp(context.Background(), FollowUpEvent{OwnerID: "s_01", IsAI: false, Result: "converted", OccurredAt: time.Now()})
	}
	for i := 0; i < 200; i++ {
		dashboard.RecordFollowUp(context.Background(), FollowUpEvent{OwnerID: "s_01", IsAI: false, Result: "no_reply", OccurredAt: time.Now()})
	}

	prod := dashboard.GetAIProductivity(context.Background(), time.Time{})
	if prod.TotalAIDeals != 50 {
		t.Errorf("expected 50 AI deals, got %d", prod.TotalAIDeals)
	}
	if prod.TransferredCount != 5 {
		t.Errorf("expected 5 transfers, got %d", prod.TransferredCount)
	}
	if prod.AISoloDeals != 5 {
		t.Errorf("expected 5 solo deals, got %d", prod.AISoloDeals)
	}
	if prod.TransferRate != 10 {
		t.Errorf("expected 10%% transfer rate, got %f", prod.TransferRate)
	}
}

func TestSalesDashboard_TeamDashboard(t *testing.T) {
	journey := NewCustomerJourneyService()
	dashboard := NewSalesDashboard(journey)
	dashboard.RegisterSales(context.Background(), SalesProfile{SalesID: "s_01", Name: "小张"})

	td := dashboard.GetTeamDashboard(context.Background(), time.Time{})
	if td == nil {
		t.Fatal("team dashboard should not be nil")
	}
	if td.Funnel == nil {
		t.Error("funnel should not be nil")
	}
	if td.AIProductivity == nil {
		t.Error("AI productivity should not be nil")
	}
	if td.Champion == nil {
		t.Error("champion should not be nil")
	}
	if len(td.TopSales) != 1 {
		t.Errorf("expected 1 top sale, got %d", len(td.TopSales))
	}
}

// TestEndToEnd_CompleteLoop 商业产品级端到端：客户从接触到成单 + 仪表盘统计 + 复购预测
// 这个测试模拟真实业务的全流程闭环
func TestEndToEnd_CompleteLoop(t *testing.T) {
	ctx := context.Background()
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	extractor := NewOrderIntentExtractor()
	repurchase := NewRepurchaseEngine()
	dashboard := NewSalesDashboard(journey)

	customers := []struct {
		ID       string
		Intent   string
		Amount   float64
		Product  string
		Conf     float64
		Converts bool
	}{
		{"e2e_01", IntentPriceInquiry, 880, "光子嫩肤", 0.9, true},
		{"e2e_02", IntentPriceInquiry, 880, "光子嫩肤", 0.7, false},
		{"e2e_03", IntentObjectionPrice, 0, "", 0.8, false},
		{"e2e_04", IntentAskProduct, 0, "", 0.85, false},
		{"e2e_05", IntentPurchase, 1500, "水光针", 0.95, true},
		{"e2e_06", IntentChurn, 0, "", 0.9, false},
		{"e2e_07", IntentPurchase, 3000, "私教课", 0.92, true},
		{"e2e_08", IntentPriceInquiry, 200, "瑜伽课", 0.88, true},
		{"e2e_09", IntentSocial, 0, "", 0.6, false},
		{"e2e_10", IntentComplaint, 0, "", 0.85, false},
	}

	for _, c := range customers {
		intentResult := &dto.RecognizeResult{IntentType: c.Intent, Confidence: c.Conf}
		tagger.TagFromSalesResponse(context.Background(), c.ID, &SalesResponse{Intent: intentResult})

		if c.Product != "" {
			extractor.ExtractFromText(context.Background(), c.ID, "客户说"+c.Product)
		}

		if c.Converts {
			_, _ = journey.Transition(ctx, c.ID, StageLead, "ai", "ai", "", nil)
			_, _ = journey.Transition(ctx, c.ID, StageContact, "manual", "s_01", "", nil)
			_, _ = journey.Transition(ctx, c.ID, StageInterested, "ai", "ai", "", nil)
			_, _ = journey.Transition(ctx, c.ID, StageQuoted, "manual", "s_01", "", nil)
			_, _ = journey.Transition(ctx, c.ID, StageWon, "order", "system", "", nil)

			dashboard.RecordOrder(context.Background(), OrderEvent{
				CustomerID:  c.ID,
				OwnerID:     "s_01",
				Amount:      c.Amount,
				ProductName: c.Product,
				IsAIHandled: c.Conf > 0.85,
				OrderedAt:   time.Now(),
			})

			repurchase.RecordPurchase(context.Background(), PurchaseEvent{
				CustomerID:  c.ID,
				Amount:      c.Amount,
				ProductName: c.Product,
				OrderedAt:   time.Now(),
			})
		} else {
			if c.Intent == IntentChurn || c.Intent == IntentComplaint {
				_, _ = journey.Transition(ctx, c.ID, StageLost, "ai", "ai", "", nil)
			}
		}

		dashboard.RecordAIDeal(context.Background(), AIDealEvent{
			CustomerID: c.ID,
			OwnerID:    "s_01",
			Intent:     c.Intent,
			Replied:    true,
			CostTokens: 1000 + int(c.Conf*500),
			LatencyMs:  1500,
			OccurredAt: time.Now(),
		})

		_, _ = followup.Schedule(ctx, c.ID, "s_01", ReminderFirstContact, 1*time.Hour, nil)
	}

	wonCount := len(journey.ListByStage(context.Background(), StageWon))
	if wonCount != 4 {
		t.Errorf("expected 4 won, got %d", wonCount)
	}

	perf := dashboard.GetSalesPerformance(context.Background(), "s_01", time.Time{})
	if perf.TotalOrders != 4 {
		t.Errorf("expected 4 orders, got %d", perf.TotalOrders)
	}
	expectedRev := 880.0 + 1500.0 + 3000.0 + 200.0
	if perf.TotalRevenue != expectedRev {
		t.Errorf("expected %f revenue, got %f", expectedRev, perf.TotalRevenue)
	}

	prod := dashboard.GetAIProductivity(context.Background(), time.Time{})
	if prod.TotalAIDeals != 10 {
		t.Errorf("expected 10 AI deals, got %d", prod.TotalAIDeals)
	}

	funnel := dashboard.FunnelByJourney(context.Background())
	if funnel.TotalWon != 4 {
		t.Errorf("funnel won should be 4, got %d", funnel.TotalWon)
	}

	pending := followup.ListPending(context.Background(), "s_01", 0)
	if len(pending) < 10 {
		t.Errorf("expected >= 10 pending, got %d", len(pending))
	}
}

