package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	sysmodel "marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
)

// FunnelSourceStat 漏斗来源统计行
type FunnelSourceStat struct {
	Source string
	Count  int64
}

// ConversionFunnelRepository 转化漏斗分析仓储
type ConversionFunnelRepository struct {
	db *gorm.DB
}

// NewConversionFunnelRepository 创建转化漏斗仓储实例
func NewConversionFunnelRepository() *ConversionFunnelRepository {
	return &ConversionFunnelRepository{db: _db.GetDB()}
}

// CountCustomerEventsByTimeRange 统计时间范围内的客户事件数（访问阶段）
func (r *ConversionFunnelRepository) CountCustomerEventsByTimeRange(ctx context.Context, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.CustomerEvent{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&count).Error
	return count, err
}

// CountCluesByUnixTimeRange 统计时间范围内的线索数（按 unix 时间戳过滤）
func (r *ConversionFunnelRepository) CountCluesByUnixTimeRange(ctx context.Context, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.Clue{}).
		Where("create_time >= ? AND create_time <= ?", startTime.Unix(), endTime.Unix()).
		Count(&count).Error
	return count, err
}

// CountIntentRecords 统计时间范围内的意向记录数（按 intent_type 过滤）
func (r *ConversionFunnelRepository) CountIntentRecords(ctx context.Context, startTime, endTime time.Time, intentTypes []string) (int64, error) {
	var count int64
	q := r.db.WithContext(ctx).
		Table("intent_records").
		Where("created_at BETWEEN ? AND ?", startTime, endTime)
	if len(intentTypes) > 0 {
		q = q.Where("intent_type IN ?", intentTypes)
	}
	err := q.Count(&count).Error
	return count, err
}

// CountCustomerSessionsByTimeRange 统计时间范围内的会话数
func (r *ConversionFunnelRepository) CountCustomerSessionsByTimeRange(ctx context.Context, startTime, endTime time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&sysmodel.CustomerSession{}).
		Where("created_at BETWEEN ? AND ?", startTime, endTime).
		Count(&count).Error
	return count, err
}

// GetClueSourceStats 线索来源分布（按 account 分组，取 Top 10）
func (r *ConversionFunnelRepository) GetClueSourceStats(ctx context.Context, startTime, endTime time.Time) ([]FunnelSourceStat, error) {
	var rows []FunnelSourceStat
	err := r.db.WithContext(ctx).
		Model(&sysmodel.Clue{}).
		Select("account as source, COUNT(*) as count").
		Where("create_time >= ? AND create_time <= ?", startTime.Unix(), endTime.Unix()).
		Group("account").
		Order("count DESC").
		Limit(10).
		Scan(&rows).Error
	return rows, err
}
