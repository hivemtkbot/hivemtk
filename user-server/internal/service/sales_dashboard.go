package service
// Deprecated: 纯内存 slice 存储（重启丢数据、slice 无界增长 OOM 风险）。
// 保留仅为 sales_action_trigger 内部引用编译兼容。新报表请使用 dashboard_sse_stats(DB 权威)
// 与 ops 报表套件(custom_report/conversion_funnel/dashboard_screen)。禁止新增调用。

import (
	"context"
	"sort"
	"sync"
	"time"
)


// SalesDashboard 销售仪表盘
type SalesDashboard struct {
	mu sync.RWMutex

	journey *CustomerJourneyService

	orders []OrderEvent

	followups []FollowUpEvent

	aiDeals []AIDealEvent

	drafts []OrderDraftEvent

	salesProfiles map[string]*SalesProfile 
}

// OrderDraftEvent 订单草稿事件（-11）
type OrderDraftEvent struct {
	DraftID     string    `json:"draft_id"`
	CustomerID  string    `json:"customer_id"`
	OwnerID     string    `json:"owner_id"`
	ProductName string    `json:"product_name"`
	Amount      float64   `json:"amount"`
	Action      string    `json:"action"`     
	Source      string    `json:"source"`     
	Confidence  float64   `json:"confidence"` 
	OccurredAt  time.Time `json:"occurred_at"`
}

// OrderEvent 订单事件
type OrderEvent struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	OwnerID     string    `json:"owner_id"`
	Amount      float64   `json:"amount"`
	ProductName string    `json:"product_name"`
	IsAIHandled bool      `json:"is_ai_handled"` 
	OrderedAt   time.Time `json:"ordered_at"`
}

// FollowUpEvent 跟进事件
type FollowUpEvent struct {
	CustomerID string    `json:"customer_id"`
	OwnerID    string    `json:"owner_id"`
	Channel    string    `json:"channel"`
	IsAI       bool      `json:"is_ai"`
	Result     string    `json:"result"` 
	OccurredAt time.Time `json:"occurred_at"`
}

// AIDealEvent AI 谈单事件
type AIDealEvent struct {
	CustomerID  string    `json:"customer_id"`
	OwnerID     string    `json:"owner_id"`
	Intent      string    `json:"intent"`
	Replied     bool      `json:"replied"`     
	Transferred bool      `json:"transferred"` 
	CostTokens  int       `json:"cost_tokens"`
	LatencyMs   int       `json:"latency_ms"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// SalesProfile 销售档案
type SalesProfile struct {
	SalesID  string    `json:"sales_id"`
	Name     string    `json:"name"`
	Team     string    `json:"team"`
	JoinedAt time.Time `json:"joined_at"`
	Tags     []string  `json:"tags"` 
}

// NewSalesDashboard 创建销售仪表盘
func NewSalesDashboard(journey *CustomerJourneyService) *SalesDashboard {
	return &SalesDashboard{
		journey:       journey,
		orders:        make([]OrderEvent, 0),
		followups:     make([]FollowUpEvent, 0),
		aiDeals:       make([]AIDealEvent, 0),
		drafts:        make([]OrderDraftEvent, 0),
		salesProfiles: make(map[string]*SalesProfile),
	}
}

// RegisterSales 注册销售档案
func (d *SalesDashboard) RegisterSales(ctx context.Context, profile SalesProfile) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if profile.JoinedAt.IsZero() {
		profile.JoinedAt = time.Now()
	}
	d.salesProfiles[profile.SalesID] = &profile
}

// RecordOrder 记录订单
func (d *SalesDashboard) RecordOrder(ctx context.Context, ev OrderEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ev.OrderedAt.IsZero() {
		ev.OrderedAt = time.Now()
	}
	d.orders = append(d.orders, ev)
}

// RecordFollowUp 记录跟进
func (d *SalesDashboard) RecordFollowUp(ctx context.Context, ev FollowUpEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	d.followups = append(d.followups, ev)
}

// RecordAIDeal 记录 AI 谈单
func (d *SalesDashboard) RecordAIDeal(ctx context.Context, ev AIDealEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	d.aiDeals = append(d.aiDeals, ev)
}

// RecordOrderDraft 记录订单草稿事件（-11）
// 商业产品级业务流：销售每天接触 50+ 客户，订单草稿的"创建/确认/取消/过期"
// 4 个动作都是仪表盘需要追踪的关键指标。
func (d *SalesDashboard) RecordOrderDraft(ctx context.Context, ev OrderDraftEvent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = time.Now()
	}
	d.drafts = append(d.drafts, ev)
}

// GetDraftStats 获取草稿统计（按 ownerID + 动作聚合）
// 商业产品级：销售管理者需要看到每个销售的"草稿转化率"（确认/创建）。
// 这是销售效率的核心指标：草稿越多但确认越少 = 销售不积极。
// 转化率 = 确认数 / 创建数（更准确反映草稿到成单的转化）
func (d *SalesDashboard) GetDraftStats(ctx context.Context, ownerID string, since time.Time) *DraftStats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	stats := &DraftStats{
		OwnerID:     ownerID,
		ByAction:    make(map[string]int),
		ByProduct:   make(map[string]int),
		GeneratedAt: time.Now(),
	}
	totalAmount := 0.0
	for _, ev := range d.drafts {
		if ownerID != "" && ev.OwnerID != ownerID {
			continue
		}
		if !since.IsZero() && ev.OccurredAt.Before(since) {
			continue
		}
		stats.Total++
		stats.ByAction[ev.Action]++
		stats.ByProduct[ev.ProductName]++
		totalAmount += ev.Amount
		if ev.Action == "confirmed" {
			stats.ConfirmedAmount += ev.Amount
		}
	}
	created := stats.ByAction["created"]
	confirmed := stats.ByAction["confirmed"]
	if created > 0 {
		stats.ConversionRate = float64(confirmed) / float64(created) * 100
	}
	if stats.Total > 0 {
		stats.AvgAmount = totalAmount / float64(stats.Total)
	}
	return stats
}

// DraftStats 草稿统计
type DraftStats struct {
	OwnerID         string         `json:"owner_id"`
	Total           int            `json:"total"`
	ByAction        map[string]int `json:"by_action"`        
	ByProduct       map[string]int `json:"by_product"`       
	ConversionRate  float64        `json:"conversion_rate"`  
	AvgAmount       float64        `json:"avg_amount"`       
	ConfirmedAmount float64        `json:"confirmed_amount"` 
	GeneratedAt     time.Time      `json:"generated_at"`
}


// FunnelByJourney 基于客户旅程的转化漏斗
// 商业逻辑：每阶段的客户数 + 阶段间转化率 + 端到端转化率
type JourneyFunnel struct {
	Stages       []JourneyFunnelStage `json:"stages"`
	TotalEntered int                  `json:"total_entered"`   
	TotalWon     int                  `json:"total_won"`       
	EndToEndRate float64              `json:"end_to_end_rate"` 
	AvgDwellDays float64              `json:"avg_dwell_days"`  
	GeneratedAt  time.Time            `json:"generated_at"`
}

// JourneyFunnelStage 漏斗阶段
type JourneyFunnelStage struct {
	Stage        JourneyStage   `json:"stage"`
	Label        string         `json:"label"`
	Customers    int            `json:"customers"`
	StageRate    float64        `json:"stage_rate"` 
	StepRate     float64        `json:"step_rate"`  
	AvgDwellDays float64        `json:"avg_dwell_days"`
	OwnerLoad    map[string]int `json:"owner_load"` 
}

// FunnelByJourney 构建旅程漏斗
func (d *SalesDashboard) FunnelByJourney(ctx context.Context) *JourneyFunnel {
	funnel := &JourneyFunnel{
		Stages:      make([]JourneyFunnelStage, 0, len(AllStages)),
		GeneratedAt: time.Now(),
	}
	if d.journey == nil {
		return funnel
	}

	stageCounts := make(map[JourneyStage]int)
	stageDwell := make(map[JourneyStage][]float64)
	ownerLoad := make(map[JourneyStage]map[string]int)

	d.journey.mu.RLock()
	for _, state := range d.journey.states {
		stageCounts[state.CurrentStage]++
		dwell := time.Since(state.StageSince).Hours() / 24
		stageDwell[state.CurrentStage] = append(stageDwell[state.CurrentStage], dwell)
		if ownerLoad[state.CurrentStage] == nil {
			ownerLoad[state.CurrentStage] = make(map[string]int)
		}
		owner := state.Metadata["owner_id"]
		if owner == "" {
			owner = "unassigned"
		}
		ownerLoad[state.CurrentStage][owner]++
	}
	d.journey.mu.RUnlock()

	funnel.TotalEntered = stageCounts[StageStranger] + stageCounts[StageLead] + stageCounts[StageContact]
	funnel.TotalWon = stageCounts[StageWon]
	if funnel.TotalEntered > 0 {
		funnel.EndToEndRate = float64(funnel.TotalWon) / float64(funnel.TotalEntered)
	}

	orderedStages := []JourneyStage{
		StageStranger, StageLead, StageContact, StageInterested,
		StageQuoted, StageWon,
	}
	for _, st := range orderedStages {
		count := stageCounts[st]
		dwells := stageDwell[st]
		avgDwell := 0.0
		if len(dwells) > 0 {
			sum := 0.0
			for _, d := range dwells {
				sum += d
			}
			avgDwell = sum / float64(len(dwells))
		}
		meta := StageMetas[st]
		label := string(st)
		if meta != nil && meta.Label != "" {
			label = meta.Label
		}
		funnel.Stages = append(funnel.Stages, JourneyFunnelStage{
			Stage:        st,
			Label:        label,
			Customers:    count,
			AvgDwellDays: avgDwell,
			OwnerLoad:    ownerLoad[st],
		})
	}
	for i := range funnel.Stages {
		funnel.Stages[i].StageRate = float64(funnel.Stages[i].Customers) / float64(funnel.TotalEntered) * 100
		if i == 0 {
			funnel.Stages[i].StepRate = 100
		} else if funnel.Stages[i-1].Customers > 0 {
			funnel.Stages[i].StepRate = float64(funnel.Stages[i].Customers) / float64(funnel.Stages[i-1].Customers) * 100
		}
	}
	return funnel
}


// SalesPerformance 销售业绩
type SalesPerformance struct {
	SalesID        string    `json:"sales_id"`
	Name           string    `json:"name"`
	Team           string    `json:"team"`
	TotalOrders    int       `json:"total_orders"`
	TotalRevenue   float64   `json:"total_revenue"`
	TotalFollowUps int       `json:"total_follow_ups"`
	Conversions    int       `json:"conversions"`     
	ConversionRate float64   `json:"conversion_rate"` 
	AvgDealAmount  float64   `json:"avg_deal_amount"`
	ActiveDays     int       `json:"active_days"`
	Rank           int       `json:"rank"`
	GeneratedAt    time.Time `json:"generated_at"`
}

// GetSalesPerformance 获取销售个人业绩
func (d *SalesDashboard) GetSalesPerformance(ctx context.Context, salesID string, since time.Time) *SalesPerformance {
	d.mu.RLock()
	defer d.mu.RUnlock()

	perf := &SalesPerformance{
		SalesID:     salesID,
		GeneratedAt: time.Now(),
	}
	if profile, ok := d.salesProfiles[salesID]; ok {
		perf.Name = profile.Name
		perf.Team = profile.Team
		perf.ActiveDays = int(time.Since(profile.JoinedAt).Hours() / 24)
	}

	activeDays := make(map[string]bool)
	for _, o := range d.orders {
		if o.OwnerID != salesID {
			continue
		}
		if !since.IsZero() && o.OrderedAt.Before(since) {
			continue
		}
		perf.TotalOrders++
		perf.TotalRevenue += o.Amount
		activeDays[o.OrderedAt.Format("2006-01-02")] = true
	}
	for _, f := range d.followups {
		if f.OwnerID != salesID {
			continue
		}
		if !since.IsZero() && f.OccurredAt.Before(since) {
			continue
		}
		perf.TotalFollowUps++
		if f.Result == "converted" {
			perf.Conversions++
		}
		activeDays[f.OccurredAt.Format("2006-01-02")] = true
	}
	if perf.TotalOrders > 0 {
		perf.AvgDealAmount = perf.TotalRevenue / float64(perf.TotalOrders)
	}
	if perf.TotalFollowUps > 0 {
		perf.ConversionRate = float64(perf.Conversions) / float64(perf.TotalFollowUps) * 100
	}
	return perf
}


// GetTeamRanking 获取销售团队排行榜
// 商业逻辑：按成单金额排序，前 10% 标记为"销冠"，后 10% 标记为"待改进"
func (d *SalesDashboard) GetTeamRanking(ctx context.Context, since time.Time, topN int) []*SalesPerformance {
	d.mu.RLock()
	salesIDs := make([]string, 0, len(d.salesProfiles))
	for id := range d.salesProfiles {
		salesIDs = append(salesIDs, id)
	}
	d.mu.RUnlock()

	performances := make([]*SalesPerformance, 0, len(salesIDs))
	for _, sid := range salesIDs {
		performances = append(performances, d.GetSalesPerformance(context.Background(), sid, since))
	}
	sort.Slice(performances, func(i, j int) bool {
		return performances[i].TotalRevenue > performances[j].TotalRevenue
	})
	for i, p := range performances {
		p.Rank = i + 1
	}
	if topN > 0 && len(performances) > topN {
		performances = performances[:topN]
	}
	return performances
}


// AIProductivity AI 产能分析
type AIProductivity struct {
	TotalAIDeals        int       `json:"total_ai_deals"`
	AIReplied           int       `json:"ai_replied"`
	ReplyRate           float64   `json:"reply_rate"`        
	TransferredCount    int       `json:"transferred_count"` 
	TransferRate        float64   `json:"transfer_rate"`     
	AISoloDeals         int       `json:"ai_solo_deals"`     
	SoloDealAmount      float64   `json:"solo_deal_amount"`
	AISoloRate          float64   `json:"ai_solo_rate"` 
	TotalCostTokens     int       `json:"total_cost_tokens"`
	AvgCostPerDeal      float64   `json:"avg_cost_per_deal"`
	AvgLatencyMs        float64   `json:"avg_latency_ms"`
	AIConversionRate    float64   `json:"ai_conversion_rate"`    
	HumanConversionRate float64   `json:"human_conversion_rate"` 
	ProductivityGain    float64   `json:"productivity_gain"`     
	GeneratedAt         time.Time `json:"generated_at"`
}

// GetAIProductivity 获取 AI 产能
func (d *SalesDashboard) GetAIProductivity(ctx context.Context, since time.Time) *AIProductivity {
	d.mu.RLock()
	defer d.mu.RUnlock()

	prod := &AIProductivity{
		GeneratedAt: time.Now(),
	}
	aiConverted := 0
	humanFollowups := 0
	humanConverted := 0
	latencySum := 0
	for _, ev := range d.aiDeals {
		if !since.IsZero() && ev.OccurredAt.Before(since) {
			continue
		}
		prod.TotalAIDeals++
		if ev.Replied {
			prod.AIReplied++
		}
		if ev.Transferred {
			prod.TransferredCount++
		}
		prod.TotalCostTokens += ev.CostTokens
		latencySum += ev.LatencyMs
	}
	for _, o := range d.orders {
		if !since.IsZero() && o.OrderedAt.Before(since) {
			continue
		}
		if o.IsAIHandled {
			prod.AISoloDeals++
			prod.SoloDealAmount += o.Amount
		}
	}
	for _, f := range d.followups {
		if !since.IsZero() && f.OccurredAt.Before(since) {
			continue
		}
		if !f.IsAI {
			humanFollowups++
			if f.Result == "converted" {
				humanConverted++
			}
		}
	}
	if prod.TotalAIDeals > 0 {
		prod.ReplyRate = float64(prod.AIReplied) / float64(prod.TotalAIDeals) * 100
		prod.TransferRate = float64(prod.TransferredCount) / float64(prod.TotalAIDeals) * 100
		prod.AvgCostPerDeal = float64(prod.TotalCostTokens) / float64(prod.TotalAIDeals)
		prod.AvgLatencyMs = float64(latencySum) / float64(prod.TotalAIDeals)
	}
	if prod.TransferredCount > 0 {
		prod.AIConversionRate = float64(aiConverted) / float64(prod.TransferredCount) * 100
	}
	if humanFollowups > 0 {
		prod.HumanConversionRate = float64(humanConverted) / float64(humanFollowups) * 100
	}
	if humanFollowups > 0 {
		prod.ProductivityGain = float64(prod.AISoloDeals) / float64(humanFollowups)
	}
	return prod
}


// ChampionProfile 销冠画像
type ChampionProfile struct {
	TopPerformers     []*SalesPerformance `json:"top_performers"` 
	CommonTags        []string            `json:"common_tags"`    
	AvgConversionRate float64             `json:"avg_conversion_rate"`
	AvgDealAmount     float64             `json:"avg_deal_amount"`
	RecommendedSOPs   []string            `json:"recommended_sops"` 
	Insights          []string            `json:"insights"`         
	GeneratedAt       time.Time           `json:"generated_at"`
}

// GetChampionProfile 获取销冠画像
// 商业逻辑：top 10% 销售的能力特征 + 行为模式 + 最佳实践
func (d *SalesDashboard) GetChampionProfile(ctx context.Context, since time.Time) *ChampionProfile {
	all := d.GetTeamRanking(context.Background(), since, 0)
	if len(all) == 0 {
		return &ChampionProfile{
			GeneratedAt: time.Now(),
			Insights:    []string{"暂无销售数据"},
		}
	}
	cutoff := len(all) / 10
	if cutoff < 1 {
		cutoff = 1
	}
	if cutoff > 10 {
		cutoff = 10
	}
	topPerformers := all[:cutoff]
	profile := &ChampionProfile{
		TopPerformers: topPerformers,
		GeneratedAt:   time.Now(),
	}

	sumConv := 0.0
	sumDeal := 0.0
	tagCount := make(map[string]int)
	for _, p := range topPerformers {
		sumConv += p.ConversionRate
		sumDeal += p.AvgDealAmount
		if prof, ok := d.salesProfiles[p.SalesID]; ok {
			for _, t := range prof.Tags {
				tagCount[t]++
			}
		}
	}
	if len(topPerformers) > 0 {
		profile.AvgConversionRate = sumConv / float64(len(topPerformers))
		profile.AvgDealAmount = sumDeal / float64(len(topPerformers))
	}
	half := len(topPerformers) / 2
	for tag, count := range tagCount {
		if count >= half {
			profile.CommonTags = append(profile.CommonTags, tag)
		}
	}
	sort.Strings(profile.CommonTags)

	if profile.AvgConversionRate > 30 {
		profile.Insights = append(profile.Insights, "销冠团队转化率 > 30%，远高于行业平均 5-10%")
	}
	if profile.AvgDealAmount > 0 {
		profile.Insights = append(profile.Insights, "销冠平均客单价显著高于团队，建议沉淀高客单 SOP")
	}
	if len(profile.CommonTags) > 0 {
		profile.Insights = append(profile.Insights, "销冠共性能力："+joinTags(profile.CommonTags))
	}
	profile.RecommendedSOPs = []string{
		"high_value_intro",   
		"objection_handling", 
		"closing_techniques", 
	}
	return profile
}

func joinTags(tags []string) string {
	out := ""
	for i, t := range tags {
		if i > 0 {
			out += "、"
		}
		out += t
	}
	return out
}


// TeamDashboard 团队综合仪表盘
type TeamDashboard struct {
	Funnel         *JourneyFunnel      `json:"funnel"`
	TopSales       []*SalesPerformance `json:"top_sales"`
	AIProductivity *AIProductivity     `json:"ai_productivity"`
	Champion       *ChampionProfile    `json:"champion"`
	PeriodStart    time.Time           `json:"period_start"`
	PeriodEnd      time.Time           `json:"period_end"`
	TotalCustomers int                 `json:"total_customers"`
	GeneratedAt    time.Time           `json:"generated_at"`
}

// GetTeamDashboard 获取团队综合仪表盘
func (d *SalesDashboard) GetTeamDashboard(ctx context.Context, since time.Time) *TeamDashboard {
	now := time.Now()
	if since.IsZero() {
		since = now.AddDate(0, 0, -30)
	}
	return &TeamDashboard{
		Funnel:         d.FunnelByJourney(context.Background()),
		TopSales:       d.GetTeamRanking(context.Background(), since, 5),
		AIProductivity: d.GetAIProductivity(context.Background(), since),
		Champion:       d.GetChampionProfile(context.Background(), since),
		PeriodStart:    since,
		PeriodEnd:      now,
		GeneratedAt:    now,
	}
}

