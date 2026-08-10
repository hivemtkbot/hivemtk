package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	sysmodel "hivemtk-user/internal/model"
)

// StatsRepository 统计仓库接口
type StatsRepository interface {
	// API日志相关
	CreateAPILog(ctx context.Context, log *sysmodel.APILog) error
	GetAPILogs(ctx context.Context, licenseID string, startTime, endTime time.Time, limit int) ([]*sysmodel.APILog, error)
	GetAPICallCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error)
	GetAPIErrorCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error)
	GetAverageResponseTime(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error)

	// 访问日志相关
	CreateVisitLog(ctx context.Context, log *sysmodel.VisitLog) error
	GetVisitLogs(ctx context.Context, licenseID string, startTime, endTime time.Time, limit int) ([]*sysmodel.VisitLog, error)
	GetVisitCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error)
	GetUniqueVisitors(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error)

	// 每日统计相关
	GetOrCreateDailyStats(ctx context.Context, licenseID string, date string) (*sysmodel.DailyStats, error)
	UpdateDailyStats(ctx context.Context, stats *sysmodel.DailyStats) error
	GetDailyStats(ctx context.Context, licenseID string, startDate, endDate string) ([]*sysmodel.DailyStats, error)
	GetDailyStatsSummary(ctx context.Context, startDate, endDate string) ([]*sysmodel.DailyStats, error)

	// 系统指标相关
	CreateSystemMetrics(ctx context.Context, metrics *sysmodel.SystemMetrics) error
	GetLatestSystemMetrics(ctx context.Context) (*sysmodel.SystemMetrics, error)
	GetSystemMetrics(ctx context.Context, startTime, endTime time.Time, limit int) ([]*sysmodel.SystemMetrics, error)

	// 统计汇总
	GetStatsSummary(ctx context.Context, licenseID string, startTime, endTime time.Time) (map[string]any, error)
}

type statsRepository struct {
	db *gorm.DB
}

func NewStatsRepository(db *gorm.DB) StatsRepository {
	return &statsRepository{db: db}
}

// CreateAPILog 创建API日志
func (r *statsRepository) CreateAPILog(ctx context.Context, log *sysmodel.APILog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetAPILogs 获取API日志
func (r *statsRepository) GetAPILogs(ctx context.Context, licenseID string, startTime, endTime time.Time, limit int) ([]*sysmodel.APILog, error) {
	var logs []*sysmodel.APILog
	query := r.db.WithContext(ctx).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

// GetAPICallCount 获取API调用次数
func (r *statsRepository) GetAPICallCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.APILog{}).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Count(&count).Error
	return count, err
}

// GetAPIErrorCount 获取API错误次数
func (r *statsRepository) GetAPIErrorCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.APILog{}).
		Where("license_id = ? AND created_at >= ? AND created_at <= ? AND status_code >= 400", licenseID, startTime, endTime).
		Count(&count).Error
	return count, err
}

// GetAverageResponseTime 获取平均响应时间
// 注：PostgreSQL AVG() 返回 numeric，扫描到 int64 会因精度溢出失败。
// 使用 float64 中间类型避免 SQLSTATE 22003/类型转换错误。
func (r *statsRepository) GetAverageResponseTime(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error) {
	var avgTime float64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.APILog{}).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Select("COALESCE(AVG(duration), 0)").
		Scan(&avgTime).Error
	return int64(avgTime), err
}

// CreateVisitLog 创建访问日志
func (r *statsRepository) CreateVisitLog(ctx context.Context, log *sysmodel.VisitLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetVisitLogs 获取访问日志
func (r *statsRepository) GetVisitLogs(ctx context.Context, licenseID string, startTime, endTime time.Time, limit int) ([]*sysmodel.VisitLog, error) {
	var logs []*sysmodel.VisitLog
	query := r.db.WithContext(ctx).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&logs).Error
	return logs, err
}

// GetVisitCount 获取访问次数
func (r *statsRepository) GetVisitCount(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.VisitLog{}).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Count(&count).Error
	return count, err
}

// GetUniqueVisitors 获取独立访客数
func (r *statsRepository) GetUniqueVisitors(ctx context.Context, licenseID string, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.VisitLog{}).
		Where("license_id = ? AND created_at >= ? AND created_at <= ?", licenseID, startTime, endTime).
		Distinct("ip_address").
		Count(&count).Error
	return count, err
}

// GetOrCreateDailyStats 获取或创建每日统计
func (r *statsRepository) GetOrCreateDailyStats(ctx context.Context, licenseID string, date string) (*sysmodel.DailyStats, error) {
	var stats sysmodel.DailyStats
	err := r.db.WithContext(ctx).
		Where("license_id = ? AND date = ?", licenseID, date).
		First(&stats).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// 创建新的每日统计
		stats = sysmodel.DailyStats{
			LicenseID:       licenseID,
			Date:            date,
			APICalls:        0,
			Visits:          0,
			UniqueVisitors:  0,
			ErrorCount:      0,
			AvgResponseTime: 0,
		}
		err = r.db.WithContext(ctx).Create(&stats).Error
	}

	return &stats, err
}

// UpdateDailyStats 更新每日统计
func (r *statsRepository) UpdateDailyStats(ctx context.Context, stats *sysmodel.DailyStats) error {
	return r.db.WithContext(ctx).Save(stats).Error
}

// GetDailyStats 获取每日统计
func (r *statsRepository) GetDailyStats(ctx context.Context, licenseID string, startDate, endDate string) ([]*sysmodel.DailyStats, error) {
	var stats []*sysmodel.DailyStats
	err := r.db.WithContext(ctx).
		Where("license_id = ? AND date >= ? AND date <= ?", licenseID, startDate, endDate).
		Order("date ASC").
		Find(&stats).Error
	return stats, err
}

// GetDailyStatsSummary 获取每日统计汇总
func (r *statsRepository) GetDailyStatsSummary(ctx context.Context, startDate, endDate string) ([]*sysmodel.DailyStats, error) {
	var stats []*sysmodel.DailyStats
	err := r.db.WithContext(ctx).
		Where("date >= ? AND date <= ?", startDate, endDate).
		Order("date ASC").
		Find(&stats).Error
	return stats, err
}

// CreateSystemMetrics 创建系统指标
func (r *statsRepository) CreateSystemMetrics(ctx context.Context, metrics *sysmodel.SystemMetrics) error {
	return r.db.WithContext(ctx).Create(metrics).Error
}

// GetLatestSystemMetrics 获取最新的系统指标
func (r *statsRepository) GetLatestSystemMetrics(ctx context.Context) (*sysmodel.SystemMetrics, error) {
	var metrics sysmodel.SystemMetrics
	err := r.db.WithContext(ctx).
		Order("created_at DESC").
		First(&metrics).Error
	return &metrics, err
}

// GetSystemMetrics 获取系统指标
func (r *statsRepository) GetSystemMetrics(ctx context.Context, startTime, endTime time.Time, limit int) ([]*sysmodel.SystemMetrics, error) {
	var metrics []*sysmodel.SystemMetrics
	query := r.db.WithContext(ctx).
		Where("created_at >= ? AND created_at <= ?", startTime, endTime).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&metrics).Error
	return metrics, err
}

// GetStatsSummary 获取统计汇总
func (r *statsRepository) GetStatsSummary(ctx context.Context, licenseID string, startTime, endTime time.Time) (map[string]any, error) {
	result := make(map[string]any)

	// API调用统计
	apiCalls, err := r.GetAPICallCount(ctx, licenseID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result["api_calls"] = apiCalls

	// API错误统计
	apiErrors, err := r.GetAPIErrorCount(ctx, licenseID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result["api_errors"] = apiErrors

	// 平均响应时间
	avgResponseTime, err := r.GetAverageResponseTime(ctx, licenseID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result["avg_response_time"] = avgResponseTime

	// 访问统计
	visits, err := r.GetVisitCount(ctx, licenseID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result["visits"] = visits

	// 独立访客统计
	uniqueVisitors, err := r.GetUniqueVisitors(ctx, licenseID, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result["unique_visitors"] = uniqueVisitors

	// 错误率
	errorRate := float64(0)
	if apiCalls > 0 {
		errorRate = float64(apiErrors) / float64(apiCalls) * 100
	}
	result["error_rate"] = fmt.Sprintf("%.2f%%", errorRate)

	return result, nil
}
