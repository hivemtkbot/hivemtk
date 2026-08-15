package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// DashboardStatsRow 仪表盘会话统计聚合行
type DashboardStatsRow struct {
	OnlineSessions int64
	AISessions     int64
	HumanSessions  int64
	WaitingCount   int64
}

// HumanizeScoreRow 拟人度评分聚合行
type HumanizeScoreRow struct {
	Total int64
	Low   int64
	Mid   int64
	High  int64
	Avg   float64
}

// FunnelStageRow 漏斗阶段聚合行
type FunnelStageRow struct {
	Stage string
	Count int64
}

// FunnelStatusRow 漏斗兜底（按 session 状态聚合）行
type FunnelStatusRow struct {
	Status string
	Count  int64
}

// DashboardStatsRepository 实时驾驶舱统计仓库接口
//
// 配套 service.DashboardStatsService，封装 customer_sessions / humanize_scores /
// customer_journey 表的实时统计查询。所有 Raw SQL 在此层封装，service 不再直接持有 *gorm.DB。
type DashboardStatsRepository interface {
	Available(ctx context.Context) bool
	CountSessionsByStatus(ctx context.Context, statuses []model.SessionStatus, onlineThreshold time.Time) (int64, error)
	CountSessionsBySingleStatus(ctx context.Context, status model.SessionStatus) (int64, error)
	QueryHumanizeDistribution(ctx context.Context, since time.Time) (*HumanizeScoreRow, error)
	QueryJourneyFunnel(ctx context.Context, since time.Time) ([]FunnelStageRow, error)
	QuerySessionFunnel(ctx context.Context, since time.Time) ([]FunnelStatusRow, error)
}

type dashboardStatsRepo struct {
	db *gorm.DB
}

// NewDashboardStatsRepository 创建实时驾驶舱统计仓库
func NewDashboardStatsRepository(db *gorm.DB) DashboardStatsRepository {
	return &dashboardStatsRepo{db: db}
}

// Available 底层 db 是否可用
func (r *dashboardStatsRepo) Available(ctx context.Context) bool {
	return r.db != nil
}

// CountSessionsByStatus 按 status 集合 + 在线阈值统计会话数
//
// 与原实现一致：status IN (...) AND (last_message_at > threshold OR last_message_at IS NULL)
func (r *dashboardStatsRepo) CountSessionsByStatus(ctx context.Context, statuses []model.SessionStatus, onlineThreshold time.Time) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status IN ?", statuses).
		Where("last_message_at > ? OR last_message_at IS NULL", onlineThreshold).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// CountSessionsBySingleStatus 按单一 status 统计会话数
func (r *dashboardStatsRepo) CountSessionsBySingleStatus(ctx context.Context, status model.SessionStatus) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("status = ?", status).
		Count(&n).Error
	if err != nil {
		return 0, err
	}
	return n, nil
}

// QueryHumanizeDistribution 查询拟人度分布（最近 N 小时）
//
// Raw SQL 与原 service 实现完全一致，封装到 repository 层。
func (r *dashboardStatsRepo) QueryHumanizeDistribution(ctx context.Context, since time.Time) (*HumanizeScoreRow, error) {
	raw := `
		SELECT
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE score < 0.7) as low,
			COUNT(*) FILTER (WHERE score >= 0.7 AND score < 0.85) as mid,
			COUNT(*) FILTER (WHERE score >= 0.85) as high,
			COALESCE(AVG(score), 0) as avg
		FROM humanize_scores
		WHERE created_at > ?`
	var row HumanizeScoreRow
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// QueryJourneyFunnel 查询 customer_journey 表的阶段分布
//
// Raw SQL 与原 service 实现完全一致。
func (r *dashboardStatsRepo) QueryJourneyFunnel(ctx context.Context, since time.Time) ([]FunnelStageRow, error) {
	raw := `
		SELECT stage, COUNT(*) as count
		FROM customer_journey
		WHERE created_at > ?
		GROUP BY stage`
	var rows []FunnelStageRow
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// QuerySessionFunnel 兜底：按 customer_sessions 状态聚合
//
// Raw SQL 与原 service 实现完全一致：COUNT(DISTINCT user_id)。
func (r *dashboardStatsRepo) QuerySessionFunnel(ctx context.Context, since time.Time) ([]FunnelStatusRow, error) {
	raw := `
		SELECT status, COUNT(DISTINCT user_id) as count
		FROM customer_sessions
		WHERE created_at > ?
		GROUP BY status`
	var rows []FunnelStatusRow
	if err := r.db.WithContext(ctx).Raw(raw, since).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

