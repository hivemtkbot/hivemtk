package service

import (
	"context"
	"time"

	opsrepo "marketing/internal/ops/repository"
)

// AIProductivityService AI 产能分析服务
type AIProductivityService struct {
	repo *opsrepo.AIProductivityRepository
}

// NewAIProductivityService 创建服务
func NewAIProductivityService() *AIProductivityService {
	return &AIProductivityService{repo: opsrepo.NewAIProductivityRepository()}
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

	ctx := context.Background()
	rep := &ProductivityReport{
		StartTime:   startTime,
		EndTime:     endTime,
		GeneratedAt: time.Now(),
	}

	// 会话总数
	rep.TotalConversations, _ = s.repo.CountCustomerSessionsByTimeRange(ctx, startTime, endTime)

	// AI 回复数 (从 customer_session_messages)
	rep.AIReplies, _ = s.repo.CountSessionMessagesBySenderType(ctx, startTime, endTime, "ai")

	// 人工回复
	rep.HumanReplies, _ = s.repo.CountSessionMessagesBySenderType(ctx, startTime, endTime, "human")

	totalReplies := rep.AIReplies + rep.HumanReplies
	if totalReplies > 0 {
		rep.AIRatio = float64(rep.AIReplies) / float64(totalReplies) * 100
	}

	// 平均响应时长（粗略：从消息表）
	// 注意：PostgreSQL 不允许在聚合函数(AVG)中直接嵌套窗口函数(LAG)，
	// 必须用子查询先算出相邻消息时间差，再在外层聚合。
	rep.AvgResponseTime, _ = s.repo.GetAvgResponseTime(ctx, startTime, endTime)

	// 转化数（订单）
	rep.TotalConversions, _ = s.repo.CountOrdersByUnixTimeRangeAndStatus(ctx, startTime, endTime, 100)
	if rep.TotalConversations > 0 {
		rep.ConversionRate = float64(rep.TotalConversions) / float64(rep.TotalConversations) * 100
	}

	// LLM Token 与成本（来自 LLM usage 表）
	usage, _ := s.repo.GetLLMUsageStats(ctx, startTime, endTime)
	rep.LLMTokens = usage.Tokens
	rep.LLMCost = usage.Cost

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
	ctx := context.Background()
	start := time.Now().AddDate(0, 0, -days)
	trend := make([]DailyMetric, 0, days)

	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		dayEnd := day.AddDate(0, 0, 1)
		m := DailyMetric{Date: day.Format("2006-01-02")}

		m.Conversations, _ = s.repo.CountCustomerSessionsByDayRange(ctx, day, dayEnd)
		m.AIReplies, _ = s.repo.CountSessionMessagesBySenderTypeAndDayRange(ctx, day, dayEnd, "ai")
		m.Conversions, _ = s.repo.CountOrdersByUnixDayRangeAndStatus(ctx, day, dayEnd, 100)
		m.LLMCost, _ = s.repo.GetLLMUsageCostSum(ctx, day, dayEnd)

		trend = append(trend, m)
	}
	return trend, nil
}
