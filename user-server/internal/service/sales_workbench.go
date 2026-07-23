package service

import (
	"fmt"
	"sort"
	"sync"
	"time"
	"context"
)

// ============================================================================
// 商业产品级 销售工作台（Sales Workbench）
// ----------------------------------------------------------------------------
// P1-CLOSE-13：销售工作台首页 —— 一次调用聚合全部"今日要做什么"
//
// 商业市场需求：销售每天打开 SCRM 第一眼看到什么？真实场景：
//   - 微信群里被客户@  → 进 SCRM 工作台 → 应该一眼看到"待回复评论"+"今日跟进"
//   - 主管晨会要报数  → 应该看到"昨日成交"+"本月业绩"+"团队排名"
//   - 销售对账时  → 应该看到"待确认草稿"+"本周成单"
//
// 商业产品级必备字段：
//   1. 待办（草稿 + 跟进 + 评论，按优先级排序）
//   2. 今日业绩（订单、跟进、转化）
//   3. 本月业绩（订单、收入、转化率）
//   4. AI 产能（谈单数、独立成单率、节省成本）
//   5. 销冠排行（团队前 5 名 + 自己排名）
//   6. 客户漏斗（各阶段客户数）
//   7. 关键指标（续约率、客单价、复购率）
//
// 修复前：销售需要点 5+ 个页面才能拼凑"今天该做什么"。
// 修复后：一次调用返回所有，工作台首页直接展示。
// ============================================================================

// WorkbenchTodo 待办事项（销售工作台首页核心）
type WorkbenchTodo struct {
	Type		string		`json:"type"`		// draft / followup / comment
	Priority	int		`json:"priority"`	// 1-5,5 最高
	Title		string		`json:"title"`
	Description	string		`json:"description"`
	TargetID	string		`json:"target_id"`	// 关联 ID
	TargetType	string		`json:"target_type"`	// customer/post/order
	CustomerID	string		`json:"customer_id"`
	DueAt		time.Time	`json:"due_at"`
	CreatedAt	time.Time	`json:"created_at"`
	URL		string		`json:"url"`	// 跳转 URL
}

// WorkbenchToday 今日业绩
type WorkbenchToday struct {
	Date		time.Time	`json:"date"`
	NewOrders	int		`json:"new_orders"`
	NewRevenue	float64		`json:"new_revenue"`
	FollowUps	int		`json:"follow_ups"`
	Conversions	int		`json:"conversions"`
	ConversionRate	float64		`json:"conversion_rate"`
	AIDeals		int		`json:"ai_deals"`
}

// WorkbenchMonth 本月业绩
type WorkbenchMonth struct {
	Month		string	`json:"month"`	// YYYY-MM
	TotalOrders	int	`json:"total_orders"`
	TotalRevenue	float64	`json:"total_revenue"`
	FollowUps	int	`json:"follow_ups"`
	Conversions	int	`json:"conversions"`
	ConversionRate	float64	`json:"conversion_rate"`
	AvgDealAmount	float64	`json:"avg_deal_amount"`
	NewCustomers	int	`json:"new_customers"`
}

// WorkbenchKeyMetrics 关键指标
type WorkbenchKeyMetrics struct {
	RenewalRate		float64	`json:"renewal_rate"`		// 续约率
	AvgDealAmount		float64	`json:"avg_deal_amount"`	// 客单价
	RepurchaseRate		float64	`json:"repurchase_rate"`	// 复购率
	ChurnRate		float64	`json:"churn_rate"`		// 流失率
	AIAssistRate		float64	`json:"ai_assist_rate"`		// AI 辅助率
	ActiveCustomers		int	`json:"active_customers"`	// 活跃客户
	DormantCustomers	int	`json:"dormant_customers"`	// 沉睡客户
}

// WorkbenchOverview 工作台首页综合数据
type WorkbenchOverview struct {
	SalesID		string			`json:"sales_id"`
	Name		string			`json:"name"`
	Team		string			`json:"team"`
	Todos		[]*WorkbenchTodo	`json:"todos"`
	Today		*WorkbenchToday		`json:"today"`
	Month		*WorkbenchMonth		`json:"month"`
	AIProduct	*AIProductivity		`json:"ai_product"`
	Funnel		*JourneyFunnel		`json:"funnel"`
	Leaderboard	[]*SalesPerformance	`json:"leaderboard"`
	MyRank		int			`json:"my_rank"`
	Metrics		*WorkbenchKeyMetrics	`json:"metrics"`
	GeneratedAt	time.Time		`json:"generated_at"`
}

// SalesWorkbenchService 销售工作台服务
type SalesWorkbenchService struct {
	mu	sync.RWMutex

	// 下游依赖
	dashboard	*SalesDashboard
	journey		*CustomerJourneyService
	followup	*FollowUpService
	draft		*OrderDraftService
	tagger		*AITagger
}

// NewSalesWorkbenchService 创建工作台服务
func NewSalesWorkbenchService() *SalesWorkbenchService {
	return &SalesWorkbenchService{}
}

// SetDashboard 注入销售仪表盘
func (s *SalesWorkbenchService) SetDashboard(ctx context.Context, d *SalesDashboard) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dashboard = d
}

// SetJourney 注入客户旅程
func (s *SalesWorkbenchService) SetJourney(ctx context.Context, j *CustomerJourneyService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journey = j
}

// SetFollowUp 注入跟进服务
func (s *SalesWorkbenchService) SetFollowUp(ctx context.Context, f *FollowUpService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.followup = f
}

// SetDraft 注入订单草稿服务
func (s *SalesWorkbenchService) SetDraft(ctx context.Context, d *OrderDraftService) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draft = d
}

// SetTagger 注入标签服务
func (s *SalesWorkbenchService) SetTagger(ctx context.Context, t *AITagger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tagger = t
}

// GetOverview 获取销售工作台首页综合数据（核心入口）
// 商业产品级：一次调用聚合 7 大模块，前端无需多次请求
func (s *SalesWorkbenchService) GetOverview(ctx context.Context, salesID string) *WorkbenchOverview {
	s.mu.RLock()
	dash := s.dashboard
	journey := s.journey
	followup := s.followup
	draft := s.draft
	tagger := s.tagger
	s.mu.RUnlock()

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	overview := &WorkbenchOverview{
		SalesID:	salesID,
		GeneratedAt:	now,
		Todos:		[]*WorkbenchTodo{},
	}

	// 1. 待办清单（核心）
	overview.Todos = s.aggregateTodos(ctx, salesID, dash, followup, draft)

	// 2. 今日业绩
	overview.Today = s.aggregateToday(ctx, salesID, dash, todayStart)

	// 3. 本月业绩
	overview.Month = s.aggregateMonth(ctx, salesID, dash, monthStart)

	// 4. AI 产能
	if dash != nil {
		overview.AIProduct = dash.GetAIProductivity(monthStart)
	}

	// 5. 客户漏斗（使用 dashboard 的旅程漏斗，纯内存）
	if dash != nil {
		overview.Funnel = dash.FunnelByJourney()
	}

	// 6. 销冠排行
	if dash != nil {
		overview.Leaderboard = dash.GetTeamRanking(monthStart, 5)
		overview.MyRank = s.findMyRank(ctx, overview.Leaderboard, salesID)
	}

	// 7. 关键指标
	overview.Metrics = s.aggregateMetrics(ctx, salesID, dash, journey, tagger, monthStart)

	// 8. 销售档案
	if dash != nil {
		perf := dash.GetSalesPerformance(salesID, time.Time{})
		if perf != nil {
			overview.Name = perf.Name
			overview.Team = perf.Team
		}
	}

	return overview
}

// aggregateTodos 聚合待办（按优先级排序）
func (s *SalesWorkbenchService) aggregateTodos(ctx context.Context, salesID string, dash *SalesDashboard, followup *FollowUpService, draft *OrderDraftService) []*WorkbenchTodo {
	todos := make([]*WorkbenchTodo, 0)
	now := time.Now()

	// 1) 待确认草稿（优先级 5：直接关系收入）
	if draft != nil {
		for _, d := range draft.ListPending(salesID, 0) {
			todos = append(todos, &WorkbenchTodo{
				Type:		"draft",
				Priority:	5,
				Title:		"待确认订单草稿",
				Description:	fmt.Sprintf("%s x %d = ¥%.2f（置信度 %.0f%%）", d.ProductName, d.Quantity, d.TotalAmount, d.Confidence*100),
				TargetID:	d.ID,
				TargetType:	"order_draft",
				CustomerID:	d.CustomerID,
				DueAt:		d.ExpiresAt,
				CreatedAt:	d.CreatedAt,
				URL:		"/dashboard/drafts/" + d.ID,
			})
		}
	}

	// 2) 今日待跟进（优先级 4）
	if followup != nil {
		for _, r := range followup.ListPending(salesID, 0) {
			// 只取今日的
			if r.DueAt.Before(ctx, todayStart(now)) || r.Status != "pending" {
				continue
			}
			priority := 4
			if r.Priority == PriorityUrgent {
				priority = 5
			} else if r.Priority == PriorityLow {
				priority = 3
			}
			todos = append(todos, &WorkbenchTodo{
				Type:		"followup",
				Priority:	priority,
				Title:		"跟进：" + r.Title,
				Description:	r.Description,
				TargetID:	r.ID,
				TargetType:	"reminder",
				CustomerID:	r.CustomerID,
				DueAt:		r.DueAt,
				CreatedAt:	r.CreatedAt,
				URL:		"/dashboard/followups/" + r.ID,
			})
		}
		// 逾期未完成也展示（优先级 5）
		for _, r := range followup.ListOverdue(salesID) {
			todos = append(todos, &WorkbenchTodo{
				Type:		"followup",
				Priority:	5,
				Title:		"【逾期】跟进：" + r.Title,
				Description:	"已逾期 " + now.Sub(r.DueAt).Round(time.Hour).String(),
				TargetID:	r.ID,
				TargetType:	"reminder",
				CustomerID:	r.CustomerID,
				DueAt:		r.DueAt,
				CreatedAt:	r.CreatedAt,
				URL:		"/dashboard/followups/" + r.ID,
			})
		}
	}

	// 排序：优先级降序 + DueAt 升序
	sort.Slice(todos, func(i, j int) bool {
		if todos[i].Priority != todos[j].Priority {
			return todos[i].Priority > todos[j].Priority
		}
		return todos[i].DueAt.Before(todos[j].DueAt)
	})

	// 限制数量（避免一次返回太多）
	if len(todos) > 50 {
		todos = todos[:50]
	}
	return todos
}

// aggregateToday 聚合今日业绩
func (s *SalesWorkbenchService) aggregateToday(ctx context.Context, salesID string, dash *SalesDashboard, todayStart time.Time) *WorkbenchToday {
	now := time.Now()
	day := &WorkbenchToday{Date: todayStart}
	if dash == nil {
		return day
	}
	dash.mu.RLock()
	defer dash.mu.RUnlock()
	for _, o := range dash.orders {
		if o.OwnerID != salesID {
			continue
		}
		if o.OrderedAt.Before(todayStart) {
			continue
		}
		day.NewOrders++
		day.NewRevenue += o.Amount
	}
	for _, f := range dash.followups {
		if f.OwnerID != salesID {
			continue
		}
		if f.OccurredAt.Before(todayStart) {
			continue
		}
		day.FollowUps++
		if f.Result == "converted" {
			day.Conversions++
		}
	}
	for _, ev := range dash.aiDeals {
		if ev.OwnerID != salesID {
			continue
		}
		if ev.OccurredAt.Before(todayStart) {
			continue
		}
		day.AIDeals++
	}
	if day.FollowUps > 0 {
		day.ConversionRate = float64(day.Conversions) / float64(day.FollowUps) * 100
	}
	_ = now
	return day
}

// aggregateMonth 聚合本月业绩
func (s *SalesWorkbenchService) aggregateMonth(ctx context.Context, salesID string, dash *SalesDashboard, monthStart time.Time) *WorkbenchMonth {
	now := time.Now()
	month := &WorkbenchMonth{
		Month: monthStart.Format("2006-01"),
	}
	if dash == nil {
		return month
	}
	dash.mu.RLock()
	defer dash.mu.RUnlock()
	for _, o := range dash.orders {
		if o.OwnerID != salesID {
			continue
		}
		if o.OrderedAt.Before(monthStart) {
			continue
		}
		month.TotalOrders++
		month.TotalRevenue += o.Amount
	}
	for _, f := range dash.followups {
		if f.OwnerID != salesID {
			continue
		}
		if f.OccurredAt.Before(monthStart) {
			continue
		}
		month.FollowUps++
		if f.Result == "converted" {
			month.Conversions++
		}
	}
	if month.TotalOrders > 0 {
		month.AvgDealAmount = month.TotalRevenue / float64(month.TotalOrders)
	}
	if month.FollowUps > 0 {
		month.ConversionRate = float64(month.Conversions) / float64(month.FollowUps) * 100
	}
	_ = now
	return month
}

// aggregateMetrics 关键指标
func (s *SalesWorkbenchService) aggregateMetrics(ctx context.Context, salesID string, dash *SalesDashboard, journey *CustomerJourneyService, tagger *AITagger, since time.Time) *WorkbenchKeyMetrics {
	metrics := &WorkbenchKeyMetrics{}
	if dash == nil {
		return metrics
	}
	dash.mu.RLock()
	defer dash.mu.RUnlock()
	// 复购率 = 复购订单 / 总订单
	totalOrders := 0
	repurchaseOrders := 0
	uniqueCustomers := make(map[string]bool)
	for _, o := range dash.orders {
		if o.OwnerID != salesID {
			continue
		}
		if o.OrderedAt.Before(since) {
			continue
		}
		totalOrders++
		uniqueCustomers[o.CustomerID] = true
		// 简单逻辑：客户已有 2+ 订单视为复购
		count := 0
		for _, oo := range dash.orders {
			if oo.CustomerID == o.CustomerID && oo.OwnerID == salesID {
				count++
			}
		}
		if count >= 2 {
			repurchaseOrders++
		}
	}
	if totalOrders > 0 {
		metrics.RepurchaseRate = float64(repurchaseOrders) / float64(totalOrders) * 100
		metrics.AvgDealAmount = 0	// 月业绩里有
	}
	metrics.ActiveCustomers = len(uniqueCustomers)

	// 沉睡客户数
	if journey != nil {
		journey.mu.RLock()
		dormant := 0
		for _, st := range journey.states {
			if st.CurrentStage == StageSleeping {
				dormant++
			}
		}
		journey.mu.RUnlock()
		metrics.DormantCustomers = dormant
	}

	// AI 辅助率
	aiCount := 0
	totalCount := 0
	for _, f := range dash.followups {
		if f.OwnerID != salesID {
			continue
		}
		if f.OccurredAt.Before(since) {
			continue
		}
		totalCount++
		if f.IsAI {
			aiCount++
		}
	}
	if totalCount > 0 {
		metrics.AIAssistRate = float64(aiCount) / float64(totalCount) * 100
	}
	return metrics
}

// findMyRank 查找我的排名
func (s *SalesWorkbenchService) findMyRank(ctx context.Context, leaderboard []*SalesPerformance, salesID string) int {
	for _, p := range leaderboard {
		if p.SalesID == salesID {
			return p.Rank
		}
	}
	return 0
}

// todayStart 今日开始时间（辅助函数）
func todayStart(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// ============================================================================
// 子页面：工作台各模块独立查询
// ============================================================================

// GetTodosOnly 仅查询待办（前端轮询使用）
func (s *SalesWorkbenchService) GetTodosOnly(ctx context.Context, salesID string) []*WorkbenchTodo {
	s.mu.RLock()
	dash := s.dashboard
	followup := s.followup
	draft := s.draft
	s.mu.RUnlock()
	return s.aggregateTodos(ctx, salesID, dash, followup, draft)
}

// GetQuickActions 销售最常用的快捷入口
// 商业产品级：销售工作台首页"快链"区
func (s *SalesWorkbenchService) GetQuickActions(ctx context.Context, salesID string) []*QuickAction {
	return []*QuickAction{
		{ID: "new_draft", Title: "新建订单", Icon: "edit", URL: "/dashboard/drafts/new", Badge: 0},
		{ID: "ai_assist", Title: "AI 接管客户", Icon: "robot", URL: "/dashboard/ai/transfer", Badge: 0},
		{ID: "followup_today", Title: "今日跟进", Icon: "calendar", URL: "/dashboard/followups/today", Badge: 0},
		{ID: "lead_pool", Title: "线索池", Icon: "inbox", URL: "/dashboard/leads", Badge: 0},
		{ID: "dashboard", Title: "我的业绩", Icon: "chart", URL: "/dashboard/sales/" + salesID, Badge: 0},
	}
}

// QuickAction 快捷入口
type QuickAction struct {
	ID	string	`json:"id"`
	Title	string	`json:"title"`
	Icon	string	`json:"icon"`
	URL	string	`json:"url"`
	Badge	int	`json:"badge"`
}
