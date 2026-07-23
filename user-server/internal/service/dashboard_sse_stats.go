package service

import (
	"context"
	"math"
	"time"

	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
)

// DashboardSnapshot 实时驾驶舱数据快照
type DashboardSnapshot struct {
	// 时间戳
	GeneratedAt time.Time `json:"generated_at"`

	// 1. 在线会话数
	OnlineSessions  int `json:"online_sessions"`   // 在线会话总数
	AISessions      int `json:"ai_sessions"`       // AI 处理中
	HumanSessions   int `json:"human_sessions"`    // 人工处理中
	WaitingSessions int `json:"waiting_sessions"`  // 等待用户回复
	InFlightReplies int `json:"in_flight_replies"` // 正在生成的回复

	// 2. 拟人度分布（最近 1h）
	HumanizeDistribution HumanizeDistribution `json:"humanize_distribution"`

	// 3. 转化漏斗进度
	Funnel *FunnelProgress `json:"funnel"`

	// 4. LLM 实时指标
	LLMMetrics *LLMRealTimeMetrics `json:"llm_metrics"`
}

// HumanizeDistribution AI 拟人度分布（最近 1h）
type HumanizeDistribution struct {
	WindowHours      int     `json:"window_hours"`
	TotalScored      int     `json:"total_scored"`
	LowScoreCount    int     `json:"low_score_count"`    // < 0.7（需要重生成）
	MediumScoreCount int     `json:"medium_score_count"` // 0.7 - 0.85（边界）
	HighScoreCount   int     `json:"high_score_count"`   // >= 0.85（达标）
	AvgScore         float64 `json:"avg_score"`
	PassRate         float64 `json:"pass_rate"` // >= 0.85 占比
}

// FunnelProgress 转化漏斗进度
type FunnelProgress struct {
	Stages       []FunnelStage `json:"stages"`
	TotalEntered int           `json:"total_entered"`
	TotalWon     int           `json:"total_won"`
	OverallRate  float64       `json:"overall_rate"` // 端到端转化率（won / entered）
}

// FunnelStage 漏斗单个阶段
type FunnelStage struct {
	Name      string  `json:"name"`       // 阶段名
	Code      string  `json:"code"`       // 阶段编码
	Count     int     `json:"count"`      // 当前阶段客户数
	StageRate float64 `json:"stage_rate"` // 占总数比例（%）
	StepRate  float64 `json:"step_rate"`  // 上一步到本步的转化率（%）
}

// LLMRealTimeMetrics LLM 实时指标
type LLMRealTimeMetrics struct {
	ActiveProviders int     `json:"active_providers"` // 启用的 provider 数
	DownProviders   int     `json:"down_providers"`   // 熔断 / down 的 provider 数
	CircuitOpen     int     `json:"circuit_open"`     // 熔断器开启数
	AvgLatencyMs    float64 `json:"avg_latency_ms"`   // 平均延迟
	FailureRate     float64 `json:"failure_rate"`     // 失败率
}

// DashboardStatsService 实时驾驶舱统计服务
//
// 五层架构归属: L2 服务层
// 职责：聚合 customer_sessions / humanize_scores / customer_journey 等表的实时指标，
// 供 controller 层 SSE 流式推送使用。controller 不再直接访问 db。
type DashboardStatsService interface {
	// Available 表示底层 db 是否可用（离线场景返回 false，调用方据此降级）
	Available(ctx context.Context) bool
	// CollectSessionStats 采集会话统计（在线 / AI / 人工 / 等待）
	CollectSessionStats(ctx context.Context, snap *DashboardSnapshot)
	// CollectHumanizeDistribution 采集拟人度分布（最近 1h）
	CollectHumanizeDistribution(ctx context.Context) HumanizeDistribution
	// CollectFunnel 采集转化漏斗（基于 customer_journey，兜底 customer_sessions）
	CollectFunnel(ctx context.Context) *FunnelProgress
}

// dashboardStatsService 默认实现
type dashboardStatsService struct {
	db *gorm.DB
}

// NewDashboardStatsService 创建实时驾驶舱统计服务
func NewDashboardStatsService(db *gorm.DB) DashboardStatsService {
	return &dashboardStatsService{db: db}
}

// Available 表示底层 db 是否可用
func (s *dashboardStatsService) Available(ctx context.Context)  bool {
	return s.db != nil
}

// CollectSessionStats 采集会话统计
func (s *dashboardStatsService) CollectSessionStats(ctx context.Context, snap *DashboardSnapshot) {
	// 5 分钟内有消息视为"在线"
	onlineThreshold := time.Now().Add(-5 * time.Minute)

	// 在线会话：status IN active set 且 last_message_at > threshold
	var onlineCount int64
	s.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status IN ?", []model.SessionStatus{
			model.SessionStatusAIHandling,
			model.SessionStatusHumanHandling,
			model.SessionStatusWaiting,
		}).
		Where("last_message_at > ? OR last_message_at IS NULL", onlineThreshold).
		Count(&onlineCount)
	snap.OnlineSessions = int(onlineCount)

	// AI 处理中
	var aiCount int64
	s.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusAIHandling).
		Count(&aiCount)
	snap.AISessions = int(aiCount)

	// 人工处理中
	var humanCount int64
	s.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusHumanHandling).
		Count(&humanCount)
	snap.HumanSessions = int(humanCount)

	// 等待用户回复
	var waitingCount int64
	s.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", model.SessionStatusWaiting).
		Count(&waitingCount)
	snap.WaitingSessions = int(waitingCount)

	// 正在生成的回复 = AI 处理中
	snap.InFlightReplies = snap.AISessions
}

// CollectHumanizeDistribution 采集拟人度分布（最近 1h）
func (s *dashboardStatsService) CollectHumanizeDistribution(ctx context.Context) HumanizeDistribution {
	dist := HumanizeDistribution{WindowHours: 1}
	if s.db == nil {
		return dist
	}
	since := time.Now().Add(-1 * time.Hour)
	// 假设表名：humanize_scores，含 score 字段
	// 不依赖具体 schema：尝试查询，失败时返回空分布
	type scoreRow struct {
		Total int64
		Low   int64
		Mid   int64
		High  int64
		Avg   float64
	}
	var row scoreRow
	// 用 raw SQL 兼容不同表结构
	raw := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE score < 0.7) as low,
			COUNT(*) FILTER (WHERE score >= 0.7 AND score < 0.85) as mid,
			COUNT(*) FILTER (WHERE score >= 0.85) as high,
			COALESCE(AVG(score), 0) as avg
		FROM humanize_scores
		WHERE created_at > ?`
	tx := s.db.WithContext(ctx).Raw(raw, since).Scan(&row)
	if tx.Error != nil {
		logger.Ctx(ctx).Warn().Err(tx.Error).Msg("dashboard_sse: humanize query failed")
		return dist
	}
	dist.TotalScored = int(row.Total)
	dist.LowScoreCount = int(row.Low)
	dist.MediumScoreCount = int(row.Mid)
	dist.HighScoreCount = int(row.High)
	dist.AvgScore = roundToFloat(row.Avg, 3)
	if row.Total > 0 {
		dist.PassRate = roundToFloat(float64(row.High)/float64(row.Total)*100, 2)
	}
	return dist
}

// CollectFunnel 采集转化漏斗（基于 customer_sessions 的 customer_id 在客户旅程中的阶段）
//
// 数据源：customer_sessions 中 customer_id 在 customer_journey 表的阶段分布
// 失败时返回基于 session 状态的简化漏斗
func (s *dashboardStatsService) CollectFunnel(ctx context.Context) *FunnelProgress {
	funnel := &FunnelProgress{
		Stages: []FunnelStage{},
	}
	if s.db == nil {
		return funnel
	}

	// 尝试查询 customer_journey 表（如果存在）
	type stageRow struct {
		Stage string
		Count int64
	}
	var rows []stageRow
	tx := s.db.WithContext(ctx).Raw(`
		SELECT stage, COUNT(*) as count
		FROM customer_journey
		WHERE created_at > ?
		GROUP BY stage`, time.Now().AddDate(0, 0, -30)).Scan(&rows)

	if tx.Error != nil || len(rows) == 0 {
		// 兜底：使用 customer_sessions 状态作为简化的 5 阶段漏斗
		return s.collectFunnelFromSessions(ctx)
	}

	stageOrder := []struct {
		Name string
		Code string
	}{
		{"陌生人", "stranger"},
		{"线索", "lead"},
		{"接触", "contact"},
		{"报价", "quote"},
		{"成单", "won"},
	}
	stageCount := make(map[string]int64)
	for _, r := range rows {
		stageCount[r.Stage] = r.Count
	}
	totalEntered := int64(0)
	totalWon := int64(0)
	prevCount := int64(0)
	for i, st := range stageOrder {
		cnt := stageCount[st.Code]
		if i == 0 {
			totalEntered = cnt
		}
		if st.Code == "won" {
			totalWon = cnt
		}
		stepRate := 0.0
		if i > 0 && prevCount > 0 {
			stepRate = float64(cnt) / float64(prevCount) * 100
		}
		stageRate := 0.0
		if totalEntered > 0 {
			stageRate = float64(cnt) / float64(totalEntered) * 100
		}
		funnel.Stages = append(funnel.Stages, FunnelStage{
			Name:      st.Name,
			Code:      st.Code,
			Count:     int(cnt),
			StageRate: roundToFloat(stageRate, 2),
			StepRate:  roundToFloat(stepRate, 2),
		})
		prevCount = cnt
	}
	funnel.TotalEntered = int(totalEntered)
	funnel.TotalWon = int(totalWon)
	if totalEntered > 0 {
		funnel.OverallRate = roundToFloat(float64(totalWon)/float64(totalEntered)*100, 2)
	}
	return funnel
}

// collectFunnelFromSessions 兜底：基于 customer_sessions 状态聚合
func (s *dashboardStatsService) collectFunnelFromSessions(ctx context.Context) *FunnelProgress {
	funnel := &FunnelProgress{
		Stages: []FunnelStage{},
	}
	if s.db == nil {
		return funnel
	}
	type statusCount struct {
		Status string
		Count  int64
	}
	var rows []statusCount
	s.db.WithContext(ctx).Raw(`
		SELECT status, COUNT(DISTINCT user_id) as count
		FROM customer_sessions
		WHERE created_at > ?
		GROUP BY status`, time.Now().AddDate(0, 0, -30)).Scan(&rows)
	statusMap := make(map[string]int64)
	for _, r := range rows {
		statusMap[r.Status] = r.Count
	}
	// 5 阶段映射（状态 → 阶段）
	stageMapping := []struct {
		Stage   FunnelStage
		Sources []string
	}{
		{FunnelStage{Name: "陌生人", Code: "stranger"}, []string{string(model.SessionStatusPending)}},
		{FunnelStage{Name: "接触", Code: "contact"}, []string{string(model.SessionStatusAIHandling), string(model.SessionStatusHumanHandling)}},
		{FunnelStage{Name: "等待", Code: "waiting"}, []string{string(model.SessionStatusWaiting)}},
		{FunnelStage{Name: "解决", Code: "resolved"}, []string{string(model.SessionStatusResolved)}},
		{FunnelStage{Name: "关闭", Code: "closed"}, []string{string(model.SessionStatusClosed)}},
	}
	var totalEntered int64
	prevCount := int64(0)
	for i, m := range stageMapping {
		cnt := int64(0)
		for _, src := range m.Sources {
			cnt += statusMap[src]
		}
		if i == 0 {
			totalEntered = cnt
		}
		stepRate := 0.0
		if i > 0 && prevCount > 0 {
			stepRate = float64(cnt) / float64(prevCount) * 100
		}
		stageRate := 0.0
		if totalEntered > 0 {
			stageRate = float64(cnt) / float64(totalEntered) * 100
		}
		stage := m.Stage
		stage.Count = int(cnt)
		stage.StageRate = roundToFloat(stageRate, 2)
		stage.StepRate = roundToFloat(stepRate, 2)
		funnel.Stages = append(funnel.Stages, stage)
		prevCount = cnt
	}
	funnel.TotalEntered = int(totalEntered)
	funnel.TotalWon = int(statusMap[string(model.SessionStatusResolved)])
	if totalEntered > 0 {
		funnel.OverallRate = roundToFloat(float64(funnel.TotalWon)/float64(totalEntered)*100, 2)
	}
	return funnel
}

// roundToFloat 浮点保留 n 位小数
func roundToFloat(v float64, n int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	mult := math.Pow(10, float64(n))
	return math.Round(v*mult) / mult
}
