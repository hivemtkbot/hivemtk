package service

import (
	"time"

	"gorm.io/gorm"
	sysmodel "marketing/internal/model"
	sysrepo "marketing/internal/repository"
)

// AIProductivityService AI 产能分析服务
type AIProductivityService struct {
	db *gorm.DB
}

// NewAIProductivityService 创建服务
func NewAIProductivityService() *AIProductivityService {
	return &AIProductivityService{db: sysrepo.GetDB()}
}

// ProductivityReport AI 产能报告
type ProductivityReport struct {
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	TotalConversations int64     `json:"total_conversations"`
	AIReplies          int64     `json:"ai_replies"`
	HumanReplies       int64     `json:"human_replies"`
	AIRatio            float64   `json:"ai_ratio"`          // AI 回复占比
	AvgResponseTime    float64   `json:"avg_response_time"` // 平均响应时长(秒)
	ConversionRate     float64   `json:"conversion_rate"`   // 转化率
	TotalConversions   int64     `json:"total_conversions"`
	LLMTokens          int64     `json:"llm_tokens"`
	LLMCost            float64   `json:"llm_cost"`
	GeneratedAt        time.Time `json:"generated_at"`
}

// BuildReport 构建产能报告
func (s *AIProductivityService) BuildReport(startTime, endTime time.Time) (*ProductivityReport, error) {
	if startTime.IsZero() {
		startTime = time.Now().AddDate(0, 0, -30)
	}
	if endTime.IsZero() {
		endTime = time.Now()
	}

	rep := &ProductivityReport{
		StartTime:   startTime,
		EndTime:     endTime,
		GeneratedAt: time.Now(),
	}

	// 会话总数
	s.db.Model(&sysmodel.CustomerSession{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&rep.TotalConversations)

	// AI 回复数 (从 customer_session_messages)
	var aiCount int64
	s.db.Table("session_messages").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Where("sender_type = ?", "ai").
		Count(&aiCount)
	rep.AIReplies = aiCount

	// 人工回复
	var humanCount int64
	s.db.Table("session_messages").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Where("sender_type = ?", "human").
		Count(&humanCount)
	rep.HumanReplies = humanCount

	totalReplies := rep.AIReplies + rep.HumanReplies
	if totalReplies > 0 {
		rep.AIRatio = float64(rep.AIReplies) / float64(totalReplies) * 100
	}

	// 平均响应时长（粗略：从消息表）
	// 注意：PostgreSQL 不允许在聚合函数(AVG)中直接嵌套窗口函数(LAG)，
	// 必须用子查询先算出相邻消息时间差，再在外层聚合。
	type rtRow struct {
		Avg float64
	}
	var rt rtRow
	s.db.Raw(`SELECT AVG(EXTRACT(EPOCH FROM diff)) AS avg FROM (
		SELECT created_at - LAG(created_at) OVER (PARTITION BY session_id ORDER BY created_at) AS diff
		FROM session_messages
		WHERE created_at BETWEEN ? AND ?
	) t WHERE diff IS NOT NULL`, startTime, endTime).Scan(&rt)
	rep.AvgResponseTime = rt.Avg

	// 转化数（订单）
	var convCount int64
	s.db.Model(&sysmodel.Order{}).
		Where("create_time BETWEEN ? AND ?", startTime.Unix(), endTime.Unix()).
		Where("status = ?", 100).
		Count(&convCount)
	rep.TotalConversions = convCount
	if rep.TotalConversations > 0 {
		rep.ConversionRate = float64(convCount) / float64(rep.TotalConversations) * 100
	}

	// LLM Token 与成本（来自 LLM usage 表）
	type usageRow struct {
		Tokens int64
		Cost   float64
	}
	var ur usageRow
	s.db.Table("llm_usage_records").
		Select("COALESCE(SUM(total_tokens), 0) as tokens, COALESCE(SUM(cost), 0) as cost").
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Scan(&ur)
	rep.LLMTokens = ur.Tokens
	rep.LLMCost = ur.Cost

	return rep, nil
}

// SalesPersonaItem 销冠能力画像项
type SalesPersonaItem struct {
	Tag    string  `json:"tag"`
	Score  float64 `json:"score"`
	Sample int64   `json:"sample"`
}

// DailyMetric 每日指标
type DailyMetric struct {
	Date          string  `json:"date"`
	Conversations int64   `json:"conversations"`
	AIReplies     int64   `json:"ai_replies"`
	Conversions   int64   `json:"conversions"`
	LLMCost       float64 `json:"llm_cost"`
}

// DailyTrend 日趋势
func (s *AIProductivityService) DailyTrend(days int) ([]DailyMetric, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	start := time.Now().AddDate(0, 0, -days)
	trend := make([]DailyMetric, 0, days)

	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		dayEnd := day.AddDate(0, 0, 1)
		m := DailyMetric{Date: day.Format("2006-01-02")}

		s.db.Model(&sysmodel.CustomerSession{}).
			Where("created_at >= ? AND created_at < ?", day, dayEnd).
			Count(&m.Conversations)

		s.db.Table("session_messages").
			Where("created_at >= ? AND created_at < ?", day, dayEnd).
			Where("sender_type = ?", "ai").
			Count(&m.AIReplies)

		s.db.Model(&sysmodel.Order{}).
			Where("create_time >= ? AND create_time < ?", day.Unix(), dayEnd.Unix()).
			Where("status = ?", 100).
			Count(&m.Conversions)

		var costs []float64
		s.db.Table("llm_usage_records").
			Select("COALESCE(SUM(cost), 0)").
			Where("created_at >= ? AND created_at < ?", day, dayEnd).
			Scan(&costs)
		if len(costs) > 0 {
			m.LLMCost = costs[0]
		}

		trend = append(trend, m)
	}
	return trend, nil
}
