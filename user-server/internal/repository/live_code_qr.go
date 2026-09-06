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

type liveCodeQRRepository struct {
	db *gorm.DB
}

// NewLiveCodeQRRepository 创建活码二维码仓储实例
func NewLiveCodeQRRepository(db *gorm.DB) LiveCodeQRRepository {
	return &liveCodeQRRepository{db: db}
}

func (r *liveCodeQRRepository) Create(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return r.db.Create(qrCode).Error
}

func (r *liveCodeQRRepository) Update(ctx context.Context, qrCode *model.LiveCodeQR) error {
	return r.db.Save(qrCode).Error
}

func (r *liveCodeQRRepository) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.LiveCodeQR{}).Error
}

func (r *liveCodeQRRepository) GetByID(ctx context.Context, id string) (*model.LiveCodeQR, error) {
	var qrCode model.LiveCodeQR
	err := r.db.Where("id = ?", id).First(&qrCode).Error
	if err != nil {
		return nil, err
	}
	return &qrCode, nil
}

func (r *liveCodeQRRepository) GetByLiveCodeID(ctx context.Context, liveCodeID string) ([]*model.LiveCodeQR, error) {
	var qrCodes []*model.LiveCodeQR
	err := r.db.Where("live_code_id = ?", liveCodeID).Order("created_at DESC").Find(&qrCodes).Error
	if err != nil {
		return nil, err
	}
	return qrCodes, nil
}

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

func (r *liveCodeQRRepository) CreateStat(ctx context.Context, stat *model.LiveCodeQRStat) error {
	return r.db.Create(stat).Error
}

func (r *liveCodeQRRepository) GetStats(ctx context.Context, qrID string) ([]*model.LiveCodeQRStat, error) {
	var stats []*model.LiveCodeQRStat
	err := r.db.Where("qr_code_id = ?", qrID).Order("date DESC").Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func beginOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

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
