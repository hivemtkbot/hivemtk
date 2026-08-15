package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// LiveCodeQRRepository 活码二维码仓储接口
type LiveCodeQRRepository interface {
	Create(ctx context.Context, qrCode *model.LiveCodeQR) error
	Update(ctx context.Context, qrCode *model.LiveCodeQR) error
	Delete(ctx context.Context, id string) error
	GetByID(ctx context.Context, id string) (*model.LiveCodeQR, error)
	GetByLiveCodeID(ctx context.Context, liveCodeID string) ([]*model.LiveCodeQR, error)
	GetAvailableQR(ctx context.Context, liveCodeID string) (*model.LiveCodeQR, error)
	CreateStat(ctx context.Context, stat *model.LiveCodeQRStat) error
	GetStats(ctx context.Context, qrID string) ([]*model.LiveCodeQRStat, error)
	IncrementViewStat(ctx context.Context, qrID string) error
	IncrementClickStat(ctx context.Context, qrID string) error
	SumStats(ctx context.Context, qrID string) (views int64, clicks int64, err error)
	SumLiveCodeStats(ctx context.Context, liveCodeID string) (views int64, clicks int64, err error)
}

// liveCodeQRRepository 活码二维码仓储实现
type liveCodeQRRepository struct {
	db *gorm.DB
}

// NewLiveCodeQRRepository 创建活码二维码仓储实例
func NewLiveCodeQRRepository(db *gorm.DB) LiveCodeQRRepository {
	return &liveCodeQRRepository{db: db}
}

// Create 创建活码二维码
func (r *liveCodeQRRepository) Create(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return r.db.Create(qrCode).Error
}

// Update 更新活码二维码
func (r *liveCodeQRRepository) Update(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return r.db.Save(qrCode).Error
}

// Delete 删除活码二维码
func (r *liveCodeQRRepository) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.LiveCodeQR{}).Error
}

// GetByID 根据ID获取活码二维码
func (r *liveCodeQRRepository) GetByID(ctx context.Context, id string) (*model.LiveCodeQR, error) {
	var qrCode model.LiveCodeQR
	err := r.db.Where("id = ?", id).First(&qrCode).Error
	if err != nil {
		return nil, err
	}
	return &qrCode, nil
}

// GetByLiveCodeID 根据活码ID获取二维码列表
func (r *liveCodeQRRepository) GetByLiveCodeID(ctx context.Context, liveCodeID string) ([]*model.LiveCodeQR, error) {
	var qrCodes []*model.LiveCodeQR
	err := r.db.Where("live_code_id = ?", liveCodeID).Order("created_at DESC").Find(&qrCodes).Error
	if err != nil {
		return nil, err
	}
	return qrCodes, nil
}

// GetAvailableQR 获取可用的二维码（状态为启用）
func (r *liveCodeQRRepository) GetAvailableQR(ctx context.Context, liveCodeID string) (*model.LiveCodeQR, error) {
	var qrCode model.LiveCodeQR
	err := r.db.Where("live_code_id = ? AND status = ?", liveCodeID, 1).
		Order("created_at DESC").
		First(&qrCode).Error
	if err != nil {
		return nil, err
	}
	return &qrCode, nil
}

// CreateStat 创建二维码访问统计
func (r *liveCodeQRRepository) CreateStat(ctx context.Context, stat *model.LiveCodeQRStat) error {
	return r.db.Create(stat).Error
}

// GetStats 获取二维码访问统计
func (r *liveCodeQRRepository) GetStats(ctx context.Context, qrID string) ([]*model.LiveCodeQRStat, error) {
	var stats []*model.LiveCodeQRStat
	err := r.db.Where("qr_code_id = ?", qrID).Order("date DESC").Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// beginDay 返回当天的 00:00:00（本地时区，与 CreateStat 写入的 date 对齐）
func beginOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// IncrementViewStat 累加指定二维码当天的展示次数（按天 upsert）
func (r *liveCodeQRRepository) IncrementViewStat(ctx context.Context, qrID string) error {
	today := beginOfToday()
	var stat model.LiveCodeQRStat
	err := r.db.Where("qr_code_id = ? AND date = ?", qrID, today).First(&stat).Error
	if err == gorm.ErrRecordNotFound {
		stat = model.LiveCodeQRStat{QRCodeID: qrID, Date: today, ViewCount: 1, ClickCount: 0}
		return r.db.Create(&stat).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&stat).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

// IncrementClickStat 累加指定二维码当天的点击次数（按天 upsert）
func (r *liveCodeQRRepository) IncrementClickStat(ctx context.Context, qrID string) error {
	today := beginOfToday()
	var stat model.LiveCodeQRStat
	err := r.db.Where("qr_code_id = ? AND date = ?", qrID, today).First(&stat).Error
	if err == gorm.ErrRecordNotFound {
		stat = model.LiveCodeQRStat{QRCodeID: qrID, Date: today, ViewCount: 0, ClickCount: 1}
		return r.db.Create(&stat).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&stat).UpdateColumn("click_count", gorm.Expr("click_count + 1")).Error
}

// SumStats 汇总单个二维码历史展示/点击总数
func (r *liveCodeQRRepository) SumStats(ctx context.Context, qrID string) (int64, int64, error) {
	var views, clicks int64
	if err := r.db.WithContext(ctx).Model(&model.LiveCodeQRStat{}).
		Where("qr_code_id = ?", qrID).
		Select("COALESCE(SUM(view_count),0), COALESCE(SUM(click_count),0)").
		Row().Scan(&views, &clicks); err != nil {
		return 0, 0, err
	}
	return views, clicks, nil
}

// SumLiveCodeStats 汇总活码下所有二维码历史展示/点击总数
func (r *liveCodeQRRepository) SumLiveCodeStats(ctx context.Context, liveCodeID string) (int64, int64, error) {
	var views, clicks int64
	if err := r.db.WithContext(ctx).Model(&model.LiveCodeQRStat{}).
		Where("qr_code_id IN (?)",
			r.db.WithContext(ctx).Model(&model.LiveCodeQR{}).
				Select("qr_code_id").Where("live_code_id = ?", liveCodeID)).
		Select("COALESCE(SUM(view_count),0), COALESCE(SUM(click_count),0)").
		Row().Scan(&views, &clicks); err != nil {
		return 0, 0, err
	}
	return views, clicks, nil
}

