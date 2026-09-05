// Package service - Agent 绩效归因（人工+AI混合，G5）
//
// 统计 ai_agents 的 performance：
//   - 自动解决率（AI 处理会话数/总会话数）
//   - 人工接手率
//   - 平均解决时间
//   - CSAT
//
// 数据源：customer_sessions（agent_id + handler_type=ai|human） + csat_surveys（score）
package service

import (
	"context"
	"fmt"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
)

// AgentAttributionService Agent 绩效归因服务
type AgentAttributionService struct {
	db *gorm.DB
}

// NewAgentAttributionService 创建服务实例
func NewAgentAttributionService() *AgentAttributionService {
	return &AgentAttributionService{
		db: repository.GetDB(),
	}
}

// AgentPerformance 单个 Agent 的绩效指标
type AgentPerformance struct {
	AgentID            uint      `json:"agent_id"`
	AgentName          string    `json:"agent_name"`
	TotalSessions      int64     `json:"total_sessions"`
	AIResolvedCount    int64     `json:"ai_resolved_count"`
	HumanTakeoverCount int64     `json:"human_takeover_count"`
	AutoResolveRate    float64   `json:"auto_resolve_rate"`
	HumanTakeoverRate  float64   `json:"human_takeover_rate"`
	AvgResolveSeconds  float64   `json:"avg_resolve_seconds"`
	AvgCSAT            float64   `json:"avg_csat"`
	CSATRespondedCount int64     `json:"csat_responded_count"`
	PeriodStart        time.Time `json:"period_start"`
	PeriodEnd          time.Time `json:"period_end"`
}

// PerformanceQuery 查询参数
type PerformanceQuery struct {
	AgentID    uint       `form:"agent_id"`
	PeriodDays int        `form:"period_days"`
	StartTime  *time.Time `form:"start_time"`
	EndTime    *time.Time `form:"end_time"`
}

// GetPerformance 获取指定时间窗口内的 Agent 绩效
func (s *AgentAttributionService) GetPerformance(ctx context.Context, q *PerformanceQuery) ([]*AgentPerformance, error) {
	if q == nil {
		q = &PerformanceQuery{PeriodDays: 7}
	}
	if q.PeriodDays <= 0 {
		q.PeriodDays = 7
	}

	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -q.PeriodDays)
	if q.StartTime != nil {
		startTime = *q.StartTime
	}
	if q.EndTime != nil {
		endTime = *q.EndTime
	}

	type aggRow struct {
		AgentID       uint    `gorm:"column:agent_id"`
		AgentName     string  `gorm:"column:agent_name"`
		Total         int64   `gorm:"column:total"`
		AIResolved    int64   `gorm:"column:ai_resolved"`
		HumanTakeover int64   `gorm:"column:human_takeover"`
		AvgResolveSec float64 `gorm:"column:avg_resolve_sec"`
	}

	var rows []aggRow
	query := s.db.WithContext(ctx).
		Model(&model.CustomerSession{}).
		Select(`
			agent_id,
			agent_name,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE handler_type = 'ai') as ai_resolved,
			COUNT(*) FILTER (WHERE handler_type = 'human') as human_takeover,
			AVG(EXTRACT(EPOCH FROM (COALESCE(resolved_at, updated_at) - created_at))) as avg_resolve_sec
		`).
		Where("created_at BETWEEN ? AND ? AND agent_id > 0", startTime, endTime).
		Group("agent_id, agent_name")

	if q.AgentID > 0 {
		query = query.Where("agent_id = ?", q.AgentID)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("ATTRIB_001: 会话聚合查询失败: %w", err)
	}

	type csatRow struct {
		AgentID   uint    `gorm:"column:agent_id"`
		AvgScore  float64 `gorm:"column:avg_score"`
		Responded int64   `gorm:"column:responded"`
	}
	var csatRows []csatRow
	csatQuery := s.db.WithContext(ctx).
		Table("csat_surveys cs").
		Select(`
			cs.agent_id,
			AVG(cs.score) as avg_score,
			COUNT(*) as responded
		`).
		Joins("JOIN customer_sessions sess ON sess.session_id = cs.session_id").
		Where("cs.status = 'responded' AND cs.score > 0 AND sess.created_at BETWEEN ? AND ?", startTime, endTime).
		Group("cs.agent_id")
	if err := csatQuery.Scan(&csatRows).Error; err != nil {

		csatRows = nil
	}

	csatMap := make(map[uint]csatRow, len(csatRows))
	for _, r := range csatRows {
		csatMap[r.AgentID] = r
	}

	result := make([]*AgentPerformance, 0, len(rows))
	for _, row := range rows {
		perf := &AgentPerformance{
			AgentID:            row.AgentID,
			AgentName:          row.AgentName,
			TotalSessions:      row.Total,
			AIResolvedCount:    row.AIResolved,
			HumanTakeoverCount: row.HumanTakeover,
			AvgResolveSeconds:  row.AvgResolveSec,
			PeriodStart:        startTime,
			PeriodEnd:          endTime,
		}
		if row.Total > 0 {
			perf.AutoResolveRate = float64(row.AIResolved) / float64(row.Total)
			perf.HumanTakeoverRate = float64(row.HumanTakeover) / float64(row.Total)
		}
		if csat, ok := csatMap[row.AgentID]; ok {
			perf.AvgCSAT = csat.AvgScore
			perf.CSATRespondedCount = csat.Responded
		}
		result = append(result, perf)
	}
	return result, nil
}
