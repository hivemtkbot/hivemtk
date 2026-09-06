package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SmsUnsubscribeRepository 短信退订仓库接口
type SmsUnsubscribeRepository interface {
	Create(ctx context.Context, record *model.SmsUnsubscribe) error
	Update(ctx context.Context, record *model.SmsUnsubscribe) error
	GetByPhone(ctx context.Context, phone string) (*model.SmsUnsubscribe, error)
	ExistsByPhone(ctx context.Context, phone string) (bool, error)
	DeleteByPhone(ctx context.Context, phone string) error
	List(ctx context.Context, page, limit int, keyword string) ([]*model.SmsUnsubscribe, int64, error)
	ListAll(ctx context.Context) ([]*model.SmsUnsubscribe, error)
}

type smsUnsubscribeRepo struct {
	db *gorm.DB
}

// NewSmsUnsubscribeRepository 创建短信退订仓库实例
func NewSmsUnsubscribeRepository(db *gorm.DB) SmsUnsubscribeRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &smsUnsubscribeRepo{db: db}
}

func (r *smsUnsubscribeRepo) Create(ctx context.Context, record *model.SmsUnsubscribe) error {
	return r.db.Create(record).Error
}

func (r *smsUnsubscribeRepo) Update(ctx context.Context, record *model.SmsUnsubscribe) error {
	return r.db.Save(record).Error
}

func (r *smsUnsubscribeRepo) GetByPhone(ctx context.Context, phone string) (*model.SmsUnsubscribe, error) {
	var record model.SmsUnsubscribe
	err := r.db.Where("phone = ?", phone).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

func (r *smsUnsubscribeRepo) ExistsByPhone(ctx context.Context, phone string) (bool, error) {
	var count int64
	err := r.db.Model(&model.SmsUnsubscribe{}).Where("phone = ?", phone).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *smsUnsubscribeRepo) DeleteByPhone(ctx context.Context, phone string) error {
	return r.db.Where("phone = ?", phone).Delete(&model.SmsUnsubscribe{}).Error
}

func (r *smsUnsubscribeRepo) List(ctx context.Context, page, limit int, keyword string) ([]*model.SmsUnsubscribe, int64, error) {
	var records []*model.SmsUnsubscribe
	var total int64

	query := r.db.Model(&model.SmsUnsubscribe{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("phone LIKE ? OR reason LIKE ? OR keyword_matched LIKE ?", like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("unsubscribed_at DESC").Offset(offset).Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, err
	}
	return records, total, nil
}

func (r *smsUnsubscribeRepo) ListAll(ctx context.Context) ([]*model.SmsUnsubscribe, error) {
	var records []*model.SmsUnsubscribe
	if err := r.db.Order("unsubscribed_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
