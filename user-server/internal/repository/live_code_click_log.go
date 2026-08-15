package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// LiveCodeClickLogRepository 活码点击日志仓储接口
// 提供点击日志写入与按活码/二维码维度的真实聚合能力
type LiveCodeClickLogRepository interface {
	CreateLiveCodeClick(ctx context.Context, log *model.LiveCodeClickLog) error
	CreateQRCodeClick(ctx context.Context, log *model.QRCodeClickLog) error
	CountByLiveCode(ctx context.Context, liveCodeID string) (int64, error)
	CountTodayByLiveCode(ctx context.Context, liveCodeID string) (int64, error)
	CountByQRCode(ctx context.Context, qrCodeID string) (int64, error)
	CountTodayByQRCode(ctx context.Context, qrCodeID string) (int64, error)
}

// liveCodeClickLogRepository 活码点击日志仓储实现
type liveCodeClickLogRepository struct {
	db *gorm.DB
}

// NewLiveCodeClickLogRepository 创建活码点击日志仓储实例
func NewLiveCodeClickLogRepository(db *gorm.DB) LiveCodeClickLogRepository {
	return &liveCodeClickLogRepository{db: db}
}

// CreateLiveCodeClick 写入活码点击日志
func (r *liveCodeClickLogRepository) CreateLiveCodeClick(ctx context.Context, log *model.LiveCodeClickLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// CreateQRCodeClick 写入二维码点击日志
func (r *liveCodeClickLogRepository) CreateQRCodeClick(ctx context.Context, log *model.QRCodeClickLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// CountByLiveCode 统计活码总点击数
func (r *liveCodeClickLogRepository) CountByLiveCode(ctx context.Context, liveCodeID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.LiveCodeClickLog{}).
		Where("live_code_id = ?", liveCodeID).
		Count(&count).Error
	return count, err
}

// CountTodayByLiveCode 统计活码今日点击数
func (r *liveCodeClickLogRepository) CountTodayByLiveCode(ctx context.Context, liveCodeID string) (int64, error) {
	var count int64
	todayStart := time.Now().Truncate(24 * time.Hour)
	err := r.db.WithContext(ctx).Model(&model.LiveCodeClickLog{}).
		Where("live_code_id = ? AND created_at >= ?", liveCodeID, todayStart).
		Count(&count).Error
	return count, err
}

// CountByQRCode 统计二维码总点击数
func (r *liveCodeClickLogRepository) CountByQRCode(ctx context.Context, qrCodeID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.QRCodeClickLog{}).
		Where("qr_code_id = ?", qrCodeID).
		Count(&count).Error
	return count, err
}

// CountTodayByQRCode 统计二维码今日点击数
func (r *liveCodeClickLogRepository) CountTodayByQRCode(ctx context.Context, qrCodeID string) (int64, error) {
	var count int64
	todayStart := time.Now().Truncate(24 * time.Hour)
	err := r.db.WithContext(ctx).Model(&model.QRCodeClickLog{}).
		Where("qr_code_id = ? AND created_at >= ?", qrCodeID, todayStart).
		Count(&count).Error
	return count, err
}

