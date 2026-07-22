package repository

import (
	"time"

	"gorm.io/gorm"
	"marketing/internal/model"
)

// SalesPersonaRepository 销冠能力画像聚合查询仓储
// 注意：本仓储的查询为 PostgreSQL 原生 SQL（含窗口函数/EXTRACT EPOCH），
// 仅在 PostgreSQL 上能产生业务正确结果。
type SalesPersonaRepository struct {
	db *gorm.DB
}

// NewSalesPersonaRepository 创建销冠画像仓储
func NewSalesPersonaRepository(db *gorm.DB) *SalesPersonaRepository {
	return &SalesPersonaRepository{db: db}
}

// AvgHumanResponseSec 统计近 30 天人工座席的平均响应时长（秒）
// 来自 customer_session_messages 表，按 session_id 分区取相邻消息时间差
func (r *SalesPersonaRepository) AvgHumanResponseSec() (float64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var avg float64
	sub := r.db.Table("session_messages").
		Select("created_at - LAG(created_at) OVER (PARTITION BY session_id ORDER BY created_at) AS diff").
		Where("sender_type = ?", "human").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -30))
	err := r.db.Table("(?) AS t", sub).
		Select("COALESCE(AVG(EXTRACT(EPOCH FROM diff)), 0)").
		Scan(&avg).Error
	if err != nil {
		return 0, err
	}
	return avg, nil
}

// SessionOrderCount 近 30 天某员工的会话数与全局订单数
func (r *SalesPersonaRepository) SessionOrderCount(staffID uint) (sessions int64, orders int64, err error) {
	if r == nil || r.db == nil {
		return 0, 0, nil
	}
	if err = r.db.Table("customer_sessions").
		Select("COUNT(*) as sessions").
		Where("agent_id = ? AND created_at >= ?", staffID, time.Now().AddDate(0, 0, -30)).
		Scan(&sessions).Error; err != nil {
		return 0, 0, err
	}
	if err = r.db.Table("order").
		Select("COUNT(*) as orders").
		Where("create_time >= ?", time.Now().AddDate(0, 0, -30).Unix()).
		Scan(&orders).Error; err != nil {
		return 0, 0, err
	}
	return sessions, orders, nil
}

// CountSopExecutionsByStaff 统计某员工的 SOP 执行次数
func (r *SalesPersonaRepository) CountSopExecutionsByStaff(staffID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.Table("sop_executions AS se").
		Joins("JOIN customer_sessions AS cs ON se.session_id = cs.session_id").
		Where("cs.agent_id = ?", staffID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountRagQueryLogsByUser 统计某员工的知识库查询次数
func (r *SalesPersonaRepository) CountRagQueryLogsByUser(staffID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.Table("rag_sessions").
		Where("user_id = ?", staffID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountObjectionRecordsByStaff 统计某员工的异议处理记录数
func (r *SalesPersonaRepository) CountObjectionRecordsByStaff(staffID uint) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	var count int64
	if err := r.db.Table("sop_exec_events AS se").
		Joins("JOIN sop_executions AS sx ON se.execution_id = sx.id").
		Joins("JOIN customer_sessions AS cs ON sx.session_id = cs.session_id").
		Where("se.node_type = ?", "objection").
		Where("cs.agent_id = ?", staffID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// SatisfactionStats 某员工客户满意度统计
type SatisfactionStats struct {
	Avg   float64
	Count int64
}

// SatisfactionByStaff 统计某员工的客户满意度（平均评分 + 评分样本数）
func (r *SalesPersonaRepository) SatisfactionByStaff(staffID uint) (*SatisfactionStats, error) {
	if r == nil || r.db == nil {
		return &SatisfactionStats{}, nil
	}
	var stats SatisfactionStats
	if err := r.db.Model(&model.CustomerSession{}).
		Select("AVG(rating) as avg, COUNT(rating) as count").
		Where("agent_id = ? AND rating > 0", staffID).
		Scan(&stats).Error; err != nil {
		return &SatisfactionStats{}, err
	}
	return &stats, nil
}

// SessionCloseStats 某员工的会话关单统计
type SessionCloseStats struct {
	Total  int64
	Closed int64
}

// SessionCloseByStaff 统计某员工的总会话数与已关单会话数
func (r *SalesPersonaRepository) SessionCloseByStaff(staffID uint) (*SessionCloseStats, error) {
	if r == nil || r.db == nil {
		return &SessionCloseStats{}, nil
	}
	var stats SessionCloseStats
	if err := r.db.Model(&model.CustomerSession{}).
		Where("agent_id = ?", staffID).
		Count(&stats.Total).Error; err != nil {
		return &SessionCloseStats{}, err
	}
	if err := r.db.Model(&model.CustomerSession{}).
		Where("agent_id = ? AND status = ?", staffID, "closed").
		Count(&stats.Closed).Error; err != nil {
		return &SessionCloseStats{}, err
	}
	return &stats, nil
}

// StaffSessionCount 员工会话数排行
type StaffSessionCount struct {
	AgentID uint  `gorm:"column:agent_id"`
	Count   int64 `gorm:"column:count"`
}

// ListTopStaffsBySession 列出近 30 天会话数 Top 50 的员工
func (r *SalesPersonaRepository) ListTopStaffsBySession() ([]StaffSessionCount, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []StaffSessionCount
	err := r.db.Model(&model.CustomerSession{}).
		Select("agent_id, COUNT(*) as count").
		Where("agent_id > 0").
		Group("agent_id").
		Order("count DESC").
		Limit(50).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}
