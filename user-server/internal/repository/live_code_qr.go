package repository

import (
	"marketing/internal/model"

	"gorm.io/gorm"
)

// LiveCodeQRRepository 活码二维码仓储接口
type LiveCodeQRRepository interface {
	Create(qrCode *model.LiveCodeQR) error
	Update(qrCode *model.LiveCodeQR) error
	Delete(id string) error
	GetByID(id string) (*model.LiveCodeQR, error)
	GetByLiveCodeID(liveCodeID string) ([]*model.LiveCodeQR, error)
	GetAvailableQR(liveCodeID string) (*model.LiveCodeQR, error)
	CreateStat(stat *model.LiveCodeQRStat) error
	GetStats(qrID string) ([]*model.LiveCodeQRStat, error)
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
func (r *liveCodeQRRepository) Create(qrCode *model.LiveCodeQR) error {
	return r.db.Create(qrCode).Error
}

// Update 更新活码二维码
func (r *liveCodeQRRepository) Update(qrCode *model.LiveCodeQR) error {
	return r.db.Save(qrCode).Error
}

// Delete 删除活码二维码
func (r *liveCodeQRRepository) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.LiveCodeQR{}).Error
}

// GetByID 根据ID获取活码二维码
func (r *liveCodeQRRepository) GetByID(id string) (*model.LiveCodeQR, error) {
	var qrCode model.LiveCodeQR
	err := r.db.Where("id = ?", id).First(&qrCode).Error
	if err != nil {
		return nil, err
	}
	return &qrCode, nil
}

// GetByLiveCodeID 根据活码ID获取二维码列表
func (r *liveCodeQRRepository) GetByLiveCodeID(liveCodeID string) ([]*model.LiveCodeQR, error) {
	var qrCodes []*model.LiveCodeQR
	err := r.db.Where("live_code_id = ?", liveCodeID).Order("created_at DESC").Find(&qrCodes).Error
	if err != nil {
		return nil, err
	}
	return qrCodes, nil
}

// GetAvailableQR 获取可用的二维码（状态为启用）
func (r *liveCodeQRRepository) GetAvailableQR(liveCodeID string) (*model.LiveCodeQR, error) {
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
func (r *liveCodeQRRepository) CreateStat(stat *model.LiveCodeQRStat) error {
	return r.db.Create(stat).Error
}

// GetStats 获取二维码访问统计
func (r *liveCodeQRRepository) GetStats(qrID string) ([]*model.LiveCodeQRStat, error) {
	var stats []*model.LiveCodeQRStat
	err := r.db.Where("qr_code_id = ?", qrID).Order("date DESC").Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}
