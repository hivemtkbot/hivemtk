package service

import (
	"context"
	"testing"
	"time"

	"marketing/internal/pkg/testutil"

	"marketing/internal/model"
	dbutil "marketing/internal/pkg/utils/db"
)

// ============================================================================
// 商业产品级 销售工作台测试套件（P1-CLOSE-13）
// ----------------------------------------------------------------------------
// 商业产品级业务流：销售登录 SCRM 第一眼看到什么？
//   - 待办清单（草稿 + 跟进 + 评论）
//   - 今日 / 本月业绩
//   - AI 产能
//   - 客户漏斗
//   - 销冠排行
//   - 关键指标
//
// 核心测试维度：
//   A. 待办聚合（3 类源 + 优先级 + 排序）
//   B. 今日 / 本月业绩统计
//   C. AI 产能 / 销冠排行 / 漏斗
//   D. 关键指标
//   E. 快捷入口
//   F. 无依赖场景
// ============================================================================

// setupWorkbenchEnv 完整工作台环境
func setupWorkbenchEnv(t *testing.T) (*CustomerJourneyService, *FollowUpService, *AITagger, *SalesDashboard, *OrderDraftService, *SalesWorkbenchService) {
	// 初始化 PostgreSQL 测试 DB（OrderService 依赖 gorm.DB）
	memDB := testutil.NewTestDB(t,
		&model.Order{},
	)
	dbutil.SetTestDB(memDB)
	journey := NewCustomerJourneyService()
	followup := NewFollowUpService(journey)
	tagger := NewAITagger()
	dashboard := NewSalesDashboard(journey)
	followup.SetDashboard(dashboard)

	draft := NewOrderDraftService(nil)
	draft.SetJourney(journey)
	draft.SetDashboard(dashboard)
	draft.SetFollowUp(followup)
	draft.SetOrderService(NewOrderService())

	workbench := NewSalesWorkbenchService()
	workbench.SetDashboard(dashboard)
	workbench.SetJourney(journey)
	workbench.SetFollowUp(followup)
	workbench.SetDraft(draft)
	workbench.SetTagger(tagger)

	return journey, followup, tagger, dashboard, draft, workbench
}

// ============================================================================
// A. 待办聚合测试
// ============================================================================

// TestWorkbench_Todos_Drafts 待确认草稿出现在待办
func TestWorkbench_Todos_Drafts(t *testing.T) {
	_, _, _, _, draft, workbench := setupWorkbenchEnv(t)
	salesID := "sales_todo_001"
	// 2 个草稿
	draft.CreateManual(&CreateDraftRequest{
		CustomerID: "c1", OwnerID: salesID, ProductName: "光子嫩肤", Quantity: 1, UnitPrice: 1000,
	})
	draft.CreateManual(&CreateDraftRequest{
		CustomerID: "c2", OwnerID: salesID, ProductName: "水光针", Quantity: 1, UnitPrice: 500,
	})

	overview := workbench.GetOverview(salesID)
	if len(overview.Todos) < 2 {
		t.Errorf("待办应包含 2 个草稿，实际: %d", len(overview.Todos))
	}
	draftCount := 0
	for _, td := range overview.Todos {
		if td.Type == "draft" {
			draftCount++
			if td.Priority != 5 {
				t.Errorf("草稿待办优先级应为 5，实际: %d", td.Priority)
			}
		}
	}
	if draftCount != 2 {
		t.Errorf("应有 2 个 draft 待办，实际: %d", draftCount)
	}
	t.Logf("✅ 待办含 %d 个草稿", draftCount)
}

// TestWorkbench_Todos_Followups 待跟进出现在待办
func TestWorkbench_Todos_Followups(t *testing.T) {
	journey, followup, _, _, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_todo_002"
	custID := "c_followup_001"
	_, _ = journey.Transition(context.Background(), custID, StageLead, "test", salesID, "test", nil)

	// 安排一个即将到期的跟进
	_, _ = followup.Schedule(context.Background(), custID, salesID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "首次跟进", Priority: PriorityNormal,
	})

	overview := workbench.GetOverview(salesID)
	followupCount := 0
	for _, td := range overview.Todos {
		if td.Type == "followup" {
			followupCount++
		}
	}
	if followupCount < 1 {
		t.Errorf("应有 1 个跟进待办，实际: %d", followupCount)
	}
	t.Logf("✅ 待办含 %d 个跟进", followupCount)
}

// TestWorkbench_Todos_PrioritySort 待办按优先级排序
func TestWorkbench_Todos_PrioritySort(t *testing.T) {
	journey, followup, _, _, draft, workbench := setupWorkbenchEnv(t)
	salesID := "sales_sort"
	custID := "c_sort"
	_, _ = journey.Transition(context.Background(), custID, StageLead, "test", salesID, "test", nil)

	// 低优先级跟进
	_, _ = followup.Schedule(context.Background(), custID, salesID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "低优先级", Priority: PriorityLow,
	})
	// 高优先级草稿
	draft.CreateManual(&CreateDraftRequest{
		CustomerID: custID, OwnerID: salesID, ProductName: "P", Quantity: 1, UnitPrice: 1000,
	})

	overview := workbench.GetOverview(salesID)
	if len(overview.Todos) < 2 {
		t.Fatalf("应有至少 2 个待办，实际: %d", len(overview.Todos))
	}
	// 第一个应该是高优先级草稿（priority=5）
	if overview.Todos[0].Priority != 5 {
		t.Errorf("首位待办优先级应为 5，实际: %d (%s)", overview.Todos[0].Priority, overview.Todos[0].Type)
	}
	t.Logf("✅ 待办排序: 首位=%s (优先级=%d)", overview.Todos[0].Type, overview.Todos[0].Priority)
}

// TestWorkbench_Todos_Overdue 逾期跟进优先级最高
func TestWorkbench_Todos_Overdue(t *testing.T) {
	journey, followup, _, _, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_overdue"
	custID := "c_overdue"
	_, _ = journey.Transition(context.Background(), custID, StageLead, "test", salesID, "test", nil)

	// 安排一个已逾期 1 天的跟进
	r, _ := followup.Schedule(context.Background(), custID, salesID, ReminderFirstContact, 24*time.Hour, &ScheduleOptions{
		Title: "已逾期", Priority: PriorityNormal,
	})
	// 手动把 DueAt 调成过去
	followup.mu.Lock()
	followup.reminders[r.ID].DueAt = time.Now().Add(-1 * time.Hour)
	followup.mu.Unlock()

	overview := workbench.GetOverview(salesID)
	found := false
	for _, td := range overview.Todos {
		if td.Type == "followup" && td.Title == "【逾期】跟进：已逾期" {
			found = true
			if td.Priority != 5 {
				t.Errorf("逾期待办应为优先级 5，实际: %d", td.Priority)
			}
		}
	}
	if !found {
		t.Error("应有逾期待办")
	}
	t.Logf("✅ 逾期跟进标为紧急")
}

// ============================================================================
// B. 业绩统计测试
// ============================================================================

// TestWorkbench_Today 今日业绩
func TestWorkbench_Today(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_today"
	// 2 个今日订单
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o1", CustomerID: "c1", OwnerID: salesID, Amount: 1000, ProductName: "P", OrderedAt: time.Now(),
	})
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o2", CustomerID: "c2", OwnerID: salesID, Amount: 500, ProductName: "P2", OrderedAt: time.Now(),
	})
	// 1 个 30 天前订单（不算今日）
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o3", CustomerID: "c3", OwnerID: salesID, Amount: 100, ProductName: "P3",
		OrderedAt: time.Now().AddDate(0, 0, -30),
	})
	// 3 个今日跟进（1 个成交）
	dashboard.RecordFollowUp(FollowUpEvent{
		CustomerID: "c1", OwnerID: salesID, Result: "converted", OccurredAt: time.Now(),
	})
	dashboard.RecordFollowUp(FollowUpEvent{
		CustomerID: "c2", OwnerID: salesID, Result: "no_reply", OccurredAt: time.Now(),
	})
	dashboard.RecordFollowUp(FollowUpEvent{
		CustomerID: "c3", OwnerID: salesID, Result: "no_reply", OccurredAt: time.Now(),
	})

	overview := workbench.GetOverview(salesID)
	if overview.Today.NewOrders != 2 {
		t.Errorf("今日订单应为 2，实际: %d", overview.Today.NewOrders)
	}
	if overview.Today.NewRevenue != 1500 {
		t.Errorf("今日收入应为 1500，实际: %.2f", overview.Today.NewRevenue)
	}
	if overview.Today.FollowUps != 3 {
		t.Errorf("今日跟进数应为 3，实际: %d", overview.Today.FollowUps)
	}
	if overview.Today.Conversions != 1 {
		t.Errorf("今日成交数应为 1，实际: %d", overview.Today.Conversions)
	}
	if overview.Today.ConversionRate < 33 || overview.Today.ConversionRate > 34 {
		t.Errorf("今日转化率应为 33.3，实际: %.2f", overview.Today.ConversionRate)
	}
	t.Logf("✅ 今日业绩: 订单=%d 收入=%.2f 转化率=%.2f%%",
		overview.Today.NewOrders, overview.Today.NewRevenue, overview.Today.ConversionRate)
}

// TestWorkbench_Month 本月业绩
func TestWorkbench_Month(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_month"
	// 5 个本月订单
	for i := 0; i < 5; i++ {
		dashboard.RecordOrder(OrderEvent{
			OrderID: "o" + intToStr(i), CustomerID: "c" + intToStr(i), OwnerID: salesID,
			Amount: 1000, ProductName: "P", OrderedAt: time.Now(),
		})
	}
	// 1 个上月订单
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o_last", CustomerID: "c_last", OwnerID: salesID,
		Amount: 9999, ProductName: "P", OrderedAt: time.Now().AddDate(0, -1, 0),
	})
	overview := workbench.GetOverview(salesID)
	if overview.Month.TotalOrders != 5 {
		t.Errorf("本月订单应为 5，实际: %d", overview.Month.TotalOrders)
	}
	if overview.Month.TotalRevenue != 5000 {
		t.Errorf("本月收入应为 5000，实际: %.2f", overview.Month.TotalRevenue)
	}
	if overview.Month.AvgDealAmount != 1000 {
		t.Errorf("客单价应为 1000，实际: %.2f", overview.Month.AvgDealAmount)
	}
	t.Logf("✅ 本月业绩: 订单=%d 收入=%.2f 客单=%.2f",
		overview.Month.TotalOrders, overview.Month.TotalRevenue, overview.Month.AvgDealAmount)
}

// ============================================================================
// C. AI 产能 / 销冠排行 / 漏斗
// ============================================================================

// TestWorkbench_AIProduct AI 产能
func TestWorkbench_AIProduct(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_ai"
	// AI 处理 3 笔 + 独立成单 1 笔
	for i := 0; i < 3; i++ {
		dashboard.RecordAIDeal(AIDealEvent{
			CustomerID: "c" + intToStr(i), OwnerID: salesID, Intent: "inquiry",
			Replied: true, OccurredAt: time.Now(), CostTokens: 100, LatencyMs: 500,
		})
	}
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o_ai", CustomerID: "c1", OwnerID: salesID,
		Amount: 1000, IsAIHandled: true, OrderedAt: time.Now(),
	})
	overview := workbench.GetOverview(salesID)
	if overview.AIProduct == nil {
		t.Fatal("AI 产能不应为 nil")
	}
	if overview.AIProduct.TotalAIDeals != 3 {
		t.Errorf("AI 处理数应为 3，实际: %d", overview.AIProduct.TotalAIDeals)
	}
	if overview.AIProduct.AISoloDeals != 1 {
		t.Errorf("AI 独立成单应为 1，实际: %d", overview.AIProduct.AISoloDeals)
	}
	if overview.AIProduct.SoloDealAmount != 1000 {
		t.Errorf("AI 成单金额应为 1000，实际: %.2f", overview.AIProduct.SoloDealAmount)
	}
	t.Logf("✅ AI 产能: 处理=%d 独立成单=%d", overview.AIProduct.TotalAIDeals, overview.AIProduct.AISoloDeals)
}

// TestWorkbench_Leaderboard 销冠排行
func TestWorkbench_Leaderboard(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	// 3 个销售，业绩递减
	salesList := []struct {
		id      string
		revenue float64
	}{
		{"sales_top", 10000},
		{"sales_mid", 5000},
		{"sales_low", 1000},
	}
	for _, s := range salesList {
		dashboard.RegisterSales(SalesProfile{SalesID: s.id, Name: s.id})
		dashboard.RecordOrder(OrderEvent{
			OrderID: "o_" + s.id, CustomerID: "c1", OwnerID: s.id,
			Amount: s.revenue, OrderedAt: time.Now(),
		})
	}

	overview := workbench.GetOverview("sales_mid")
	if len(overview.Leaderboard) != 3 {
		t.Errorf("排行榜应有 3 人，实际: %d", len(overview.Leaderboard))
	}
	if overview.Leaderboard[0].SalesID != "sales_top" {
		t.Errorf("第一名应为 sales_top，实际: %s", overview.Leaderboard[0].SalesID)
	}
	if overview.MyRank != 2 {
		t.Errorf("sales_mid 应排第 2，实际: %d", overview.MyRank)
	}
	t.Logf("✅ 排行: #1=%s #2=%s (我是 %d)",
		overview.Leaderboard[0].SalesID, overview.Leaderboard[1].SalesID, overview.MyRank)
}

// TestWorkbench_Funnel 客户漏斗
func TestWorkbench_Funnel(t *testing.T) {
	journey, _, _, _, _, workbench := setupWorkbenchEnv(t)
	// 制造漏斗：10 个 stranger，5 个 interested，2 个 won
	for i := 0; i < 10; i++ {
		_, _ = journey.Transition(context.Background(), "c_funnel_"+intToStr(i),
			StageStranger, "test", "s", "test", nil)
	}
	for i := 0; i < 5; i++ {
		_, _ = journey.Transition(context.Background(), "c_funnel_"+intToStr(i),
			StageInterested, "test", "s", "test", nil)
	}
	for i := 0; i < 2; i++ {
		_, _ = journey.Transition(context.Background(), "c_funnel_"+intToStr(i),
			StageWon, "test", "s", "test", nil)
	}
	overview := workbench.GetOverview("s")
	if overview.Funnel == nil {
		t.Fatal("漏斗不应为 nil")
	}
	if len(overview.Funnel.Stages) == 0 {
		t.Error("漏斗应有阶段")
	}
	t.Logf("✅ 漏斗: %d 个阶段", len(overview.Funnel.Stages))
}

// ============================================================================
// D. 关键指标
// ============================================================================

// TestWorkbench_Metrics 关键指标
func TestWorkbench_Metrics(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_metrics"
	// 1 个客户 3 笔订单 = 复购
	for i := 0; i < 3; i++ {
		dashboard.RecordOrder(OrderEvent{
			OrderID: "o" + intToStr(i), CustomerID: "c_loyal", OwnerID: salesID,
			Amount: 100, OrderedAt: time.Now(),
		})
	}
	// 1 个新客户 1 笔
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o_new", CustomerID: "c_new", OwnerID: salesID,
		Amount: 200, OrderedAt: time.Now(),
	})
	// AI 辅助跟进
	dashboard.RecordFollowUp(FollowUpEvent{
		CustomerID: "c_loyal", OwnerID: salesID, IsAI: true, Result: "success", OccurredAt: time.Now(),
	})
	dashboard.RecordFollowUp(FollowUpEvent{
		CustomerID: "c_new", OwnerID: salesID, IsAI: false, Result: "no_reply", OccurredAt: time.Now(),
	})
	overview := workbench.GetOverview(salesID)
	if overview.Metrics == nil {
		t.Fatal("关键指标不应为 nil")
	}
	if overview.Metrics.RepurchaseRate < 70 {
		t.Errorf("复购率应≥70%%（3 笔中有 2 笔是复购），实际: %.2f", overview.Metrics.RepurchaseRate)
	}
	if overview.Metrics.AIAssistRate < 40 {
		t.Errorf("AI 辅助率应≥40%%，实际: %.2f", overview.Metrics.AIAssistRate)
	}
	if overview.Metrics.ActiveCustomers != 2 {
		t.Errorf("活跃客户应为 2，实际: %d", overview.Metrics.ActiveCustomers)
	}
	t.Logf("✅ 关键指标: 复购=%.0f%% AI辅助=%.0f%% 活跃=%d",
		overview.Metrics.RepurchaseRate, overview.Metrics.AIAssistRate, overview.Metrics.ActiveCustomers)
}

// ============================================================================
// E. 快捷入口
// ============================================================================

// TestWorkbench_QuickActions 快捷入口
func TestWorkbench_QuickActions(t *testing.T) {
	_, _, _, _, _, workbench := setupWorkbenchEnv(t)
	actions := workbench.GetQuickActions("s1")
	if len(actions) == 0 {
		t.Fatal("应有快捷入口")
	}
	hasNewDraft := false
	for _, a := range actions {
		if a.ID == "new_draft" {
			hasNewDraft = true
			if a.URL == "" {
				t.Error("新建订单 URL 不应为空")
			}
		}
	}
	if !hasNewDraft {
		t.Error("应有 new_draft 入口")
	}
	t.Logf("✅ 快捷入口: %d 个", len(actions))
}

// ============================================================================
// F. 其他
// ============================================================================

// TestWorkbench_NilDependencies 无依赖场景
func TestWorkbench_NilDependencies(t *testing.T) {
	workbench := NewSalesWorkbenchService()
	overview := workbench.GetOverview("s1")
	if overview == nil {
		t.Fatal("应返回概览（即使无依赖）")
	}
	if overview.Todos == nil {
		t.Error("Todos 不应为 nil")
	}
	if overview.Today == nil {
		t.Error("Today 不应为 nil")
	}
	if overview.Month == nil {
		t.Error("Month 不应为 nil")
	}
	if overview.Metrics == nil {
		t.Error("Metrics 不应为 nil")
	}
	t.Logf("✅ 无依赖场景")
}

// TestWorkbench_TodosOnly 仅查询待办
func TestWorkbench_TodosOnly(t *testing.T) {
	_, _, _, _, draft, workbench := setupWorkbenchEnv(t)
	salesID := "sales_todos_only"
	draft.CreateManual(&CreateDraftRequest{
		CustomerID: "c1", OwnerID: salesID, ProductName: "P", Quantity: 1, UnitPrice: 100,
	})
	todos := workbench.GetTodosOnly(salesID)
	if len(todos) < 1 {
		t.Error("应有 1 个待办")
	}
	t.Logf("✅ 待办独立查询: %d", len(todos))
}

// TestWorkbench_FullLoop 完整闭环：草稿 → 待办 → 跟进 → 业绩
// 商业产品级：销售登录工作台 → 看到草稿 → 跟进 → 业绩更新
func TestWorkbench_FullLoop(t *testing.T) {
	journey, followup, _, _, draft, workbench := setupWorkbenchEnv(t)
	salesID := "sales_full"
	custID := "c_full"

	// 1. AI 提取意向 → 创建草稿
	intent := OrderIntent{
		CustomerID: custID, ProductName: "光子嫩肤", UnitPrice: 1000, Quantity: 1, Confidence: 0.9,
	}
	draft.CreateFromIntent(&intent, salesID)

	// 2. 客户旅程推到 lead（用于生成跟进）
	_, _ = journey.Transition(context.Background(), custID, StageLead, "test", salesID, "test", nil)
	_, _ = followup.Schedule(context.Background(), custID, salesID, ReminderFirstContact, 1*time.Hour, &ScheduleOptions{
		Title: "首次跟进", Priority: PriorityNormal,
	})

	// 3. 销售工作台首页
	overview := workbench.GetOverview(salesID)
	if len(overview.Todos) < 1 {
		t.Errorf("应有至少 1 个待办，实际: %d", len(overview.Todos))
	}

	// 4. 销售确认草稿 → 创建订单
	pending := draft.ListPending(salesID, 1)
	if len(pending) > 0 {
		d, _ := draft.Confirm(pending[0].ID, salesID)
		if d != nil {
			t.Logf("✅ 草稿确认: 订单=%s", d.OrderID)
		}
	}

	// 5. 销售完成跟进
	pendings := followup.ListPending(salesID, 0)
	if len(pendings) > 0 {
		_ = followup.CompleteWithResult(context.Background(), pendings[0].ID, FollowUpResultConverted, "客户已成交")
	}

	// 6. 客户旅程推到 won
	state := journey.GetState(custID)
	if state.CurrentStage != StageWon {
		t.Errorf("客户旅程应为 won，实际: %s", state.CurrentStage)
	}

	// 7. 业绩更新
	overview2 := workbench.GetOverview(salesID)
	if overview2.Today.NewOrders < 1 {
		t.Errorf("今日订单应≥1，实际: %d", overview2.Today.NewOrders)
	}
	t.Logf("✅ 完整闭环: 评论→草稿→待办→确认→订单→跟进→业绩")
}

// TestWorkbench_ConcurrentGetOverview 并发获取工作台
func TestWorkbench_ConcurrentGetOverview(t *testing.T) {
	_, _, _, dashboard, _, workbench := setupWorkbenchEnv(t)
	salesID := "sales_concurrent"
	// 添加一些数据
	dashboard.RecordOrder(OrderEvent{
		OrderID: "o1", CustomerID: "c1", OwnerID: salesID, Amount: 1000, OrderedAt: time.Now(),
	})
	const n = 20
	done := make(chan bool, n)
	for i := 0; i < n; i++ {
		go func() {
			_ = workbench.GetOverview(salesID)
			done <- true
		}()
	}
	for i := 0; i < n; i++ {
		<-done
	}
	t.Logf("✅ 并发查询: %d 次", n)
}
