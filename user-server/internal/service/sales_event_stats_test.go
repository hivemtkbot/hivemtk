package service

// H2 回归验证：SalesEventStatsService（DB 权威）替代原 SalesDashboard（内存版）。
// 覆盖：事件写入 → 草稿统计 / 销售业绩 / 排行榜 / AI 产能 / 漏斗迁移后的 journey.Funnel。

import (
	"context"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/repository"
)

func setupStatsEnv(t *testing.T) (*CustomerJourneyService, *SalesEventStatsService) {
	database := testutil.NewTestDB(t, &model.SalesEvent{})
	return NewCustomerJourneyService(),
		NewSalesEventStatsServiceWithRepo(repository.NewSalesEventRepositoryWithDB(database))
}

func TestSalesEventStats_DraftStats(t *testing.T) {
	_, stats := setupStatsEnv(t)
	ctx := context.Background()
	ownerID := "sales_draft_001"

	stats.RecordOrderDraft(ctx, OrderDraftEvent{OwnerID: ownerID, ProductName: "光子嫩肤", Amount: 2280, Action: "created"})
	stats.RecordOrderDraft(ctx, OrderDraftEvent{OwnerID: ownerID, ProductName: "水光针", Amount: 980, Action: "created"})
	stats.RecordOrderDraft(ctx, OrderDraftEvent{OwnerID: ownerID, ProductName: "光子嫩肤", Amount: 2280, Action: "confirmed"})
	stats.RecordOrderDraft(ctx, OrderDraftEvent{OwnerID: "other_sales", Amount: 1, Action: "created"})

	ds := stats.GetDraftStats(ctx, ownerID, time.Time{})
	if ds.Total != 3 {
		t.Errorf("owner 过滤后总数应为 3，实际 %d", ds.Total)
	}
	if ds.ByAction["created"] != 2 || ds.ByAction["confirmed"] != 1 {
		t.Errorf("by_action 错误: %v", ds.ByAction)
	}
	if ds.ConversionRate < 49 || ds.ConversionRate > 51 {
		t.Errorf("转化率应约 50%%，实际 %.2f", ds.ConversionRate)
	}
	if ds.ConfirmedAmount != 2280 {
		t.Errorf("确认金额应为 2280，实际 %.2f", ds.ConfirmedAmount)
	}
}

func TestSalesEventStats_SalesPerformance(t *testing.T) {
	_, stats := setupStatsEnv(t)
	ctx := context.Background()
	ownerID := "sales_perf_001"

	stats.RegisterSales(ctx, SalesProfile{SalesID: ownerID, Name: "小张", Team: "A组"})
	stats.RecordOrder(ctx, OrderEvent{OrderID: "o1", OwnerID: ownerID, CustomerID: "c1", Amount: 50000})
	stats.RecordOrder(ctx, OrderEvent{OrderID: "o2", OwnerID: ownerID, CustomerID: "c2", Amount: 30000})
	stats.RecordFollowUp(ctx, FollowUpEvent{OwnerID: ownerID, Result: "converted"})
	stats.RecordFollowUp(ctx, FollowUpEvent{OwnerID: ownerID, Result: "contacted"})

	perf := stats.GetSalesPerformance(ctx, ownerID, time.Time{})
	if perf.Name != "小张" || perf.Team != "A组" {
		t.Errorf("档案未生效: %+v", perf)
	}
	if perf.TotalOrders != 2 || perf.TotalRevenue != 80000 {
		t.Errorf("订单聚合错误: %+v", perf)
	}
	if perf.AvgDealAmount != 40000 {
		t.Errorf("客单价应为 40000，实际 %.2f", perf.AvgDealAmount)
	}
	if perf.Conversions != 1 || perf.ConversionRate != 50 {
		t.Errorf("转化统计错误: %+v", perf)
	}
}

func TestSalesEventStats_TeamRanking(t *testing.T) {
	_, stats := setupStatsEnv(t)
	ctx := context.Background()

	for _, o := range []struct {
		id     string
		amount float64
	}{
		{"s_01", 50000}, {"s_01", 30000}, {"s_02", 40000}, {"s_03", 5000},
	} {
		stats.RecordOrder(ctx, OrderEvent{OrderID: "ord_" + o.id + string(rune(int(o.amount))), OwnerID: o.id, Amount: o.amount})
	}

	ranking := stats.GetTeamRanking(ctx, time.Time{}, 0)
	if len(ranking) != 3 {
		t.Fatalf("应有 3 名销售，实际 %d", len(ranking))
	}
	if ranking[0].SalesID != "s_01" || ranking[0].Rank != 1 {
		t.Errorf("销冠应为 s_01，实际 %+v", ranking[0])
	}
	if ranking[0].TotalRevenue != 80000 {
		t.Errorf("销冠营收应为 80000，实际 %.2f", ranking[0].TotalRevenue)
	}
	top2 := stats.GetTeamRanking(ctx, time.Time{}, 2)
	if len(top2) != 2 {
		t.Errorf("topN 截取错误: %d", len(top2))
	}
}

func TestSalesEventStats_AIProductivity(t *testing.T) {
	_, stats := setupStatsEnv(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		stats.RecordAIDeal(ctx, AIDealEvent{
			CustomerID:  "c" + string(rune('a'+i)),
			Replied:     true,
			Transferred: i < 2,
			CostTokens:  100,
			LatencyMs:   200,
		})
	}
	for i := 0; i < 4; i++ {
		stats.RecordOrder(ctx, OrderEvent{OrderID: "o" + string(rune('a'+i)), Amount: 1000, IsAIHandled: true})
	}
	stats.RecordFollowUp(ctx, FollowUpEvent{IsAI: false, Result: "converted"})

	prod := stats.GetAIProductivity(ctx, time.Time{})
	if prod.TotalAIDeals != 10 || prod.AIReplied != 10 || prod.TransferredCount != 2 {
		t.Errorf("AI 谈单统计错误: %+v", prod)
	}
	if prod.ReplyRate != 100 || prod.TransferRate != 20 {
		t.Errorf("比率错误: reply=%.0f transfer=%.0f", prod.ReplyRate, prod.TransferRate)
	}
	if prod.AvgCostPerDeal != 100 || prod.AvgLatencyMs != 200 {
		t.Errorf("均值错误: cost=%.0f latency=%.0f", prod.AvgCostPerDeal, prod.AvgLatencyMs)
	}
	if prod.AISoloDeals != 4 || prod.SoloDealAmount != 4000 {
		t.Errorf("AI 独立成单统计错误: %+v", prod)
	}
	if prod.HumanConversionRate != 100 {
		t.Errorf("人工转化率应为 100，实际 %.0f", prod.HumanConversionRate)
	}
}

func TestSalesEventStats_WorkbenchAggregates(t *testing.T) {
	journey, stats := setupStatsEnv(t)
	ctx := context.Background()
	ownerID := "sales_today"

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	stats.RecordOrder(ctx, OrderEvent{OrderID: "today1", OwnerID: ownerID, Amount: 880, OrderedAt: now})
	stats.RecordOrder(ctx, OrderEvent{OrderID: "yesterday", OwnerID: ownerID, Amount: 999, OrderedAt: todayStart.Add(-24 * time.Hour)})
	stats.RecordFollowUp(ctx, FollowUpEvent{OwnerID: ownerID, Result: "converted", OccurredAt: now})

	wb := NewSalesWorkbenchService()
	wb.SetStats(ctx, stats)

	day := wb.aggregateToday(ctx, ownerID, stats, todayStart)
	if day.NewOrders != 1 || day.NewRevenue != 880 {
		t.Errorf("今日聚合错误: %+v", day)
	}
	if day.FollowUps != 1 || day.Conversions != 1 {
		t.Errorf("今日跟进聚合错误: %+v", day)
	}

	funnel := journey.Funnel(ctx)
	if funnel == nil || len(funnel.Stages) == 0 {
		t.Error("journey.Funnel 应返回漏斗（H2 迁移后入口）")
	}
}

func TestSalesEventStats_ChampionProfile(t *testing.T) {
	_, stats := setupStatsEnv(t)
	ctx := context.Background()

	stats.RegisterSales(ctx, SalesProfile{SalesID: "s_top", Name: "销冠", Tags: []string{"高客单"}})
	stats.RecordOrder(ctx, OrderEvent{OrderID: "big1", OwnerID: "s_top", Amount: 200000})
	champion := stats.GetChampionProfile(ctx, time.Time{})
	if len(champion.TopPerformers) == 0 || champion.TopPerformers[0].SalesID != "s_top" {
		t.Errorf("销冠画像错误: %+v", champion)
	}
}
