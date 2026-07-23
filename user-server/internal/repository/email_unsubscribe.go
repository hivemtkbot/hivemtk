package repository

import (
	"context"
	"errors"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// EmailUnsubscribeRepository 邮件退订仓库接口
type EmailUnsubscribeRepository interface {
	Create(ctx context.Context, record *model.EmailUnsubscribe) error
	Update(ctx context.Context, record *model.EmailUnsubscribe) error
	GetByEmail(ctx context.Context, email string) (*model.EmailUnsubscribe, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	DeleteByEmail(ctx context.Context, email string) error
	List(ctx context.Context, page, limit int, keyword string) ([]*model.EmailUnsubscribe, int64, error)
	ListAll(ctx context.Context) ([]*model.EmailUnsubscribe, error)
}

type emailUnsubscribeRepo struct {
	db *gorm.DB
}

// NewEmailUnsubscribeRepository 创建邮件退订仓库实例
func NewEmailUnsubscribeRepository(db *gorm.DB) EmailUnsubscribeRepository {
	if db == nil {
		db = _db.GetDB()
	}
	return &emailUnsubscribeRepo{db: db}
}

// Create 创建退订记录（email 唯一，重复时返回错误）
func (r *emailUnsubscribeRepo) Create(ctx context.Context, record *model.EmailUnsubscribe) error {
	return r.db.Create(record).Error
}

// Update 更新退订记录
func (r *emailUnsubscribeRepo) Update(ctx context.Context, record *model.EmailUnsubscribe) error {
	return r.db.Save(record).Error
}

// GetByEmail 根据邮箱查询退订记录
func (r *emailUnsubscribeRepo) GetByEmail(ctx context.Context, email string) (*model.EmailUnsubscribe, error) {
	var record model.EmailUnsubscribe
	err := r.db.Where("email = ?", email).First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ExistsByEmail 判断邮箱是否已退订
func (r *emailUnsubscribeRepo) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := r.db.Model(&model.EmailUnsubscribe{}).Where("email = ?", email).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteByEmail 根据邮箱删除退订记录（重新订阅）
func (r *emailUnsubscribeRepo) DeleteByEmail(ctx context.Context, email string) error {
	return r.db.Where("email = ?", email).Delete(&model.EmailUnsubscribe{}).Error
}

// List 分页查询退订名单
func (r *emailUnsubscribeRepo) List(ctx context.Context, page, limit int, keyword string) ([]*model.EmailUnsubscribe, int64, error) {
	var records []*model.EmailUnsubscribe
	var total int64

	query := r.db.Model(&model.EmailUnsubscribe{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("email LIKE ? OR reason LIKE ?", like, like)
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

// ListAll 查询全部退订名单（导出使用）
func (r *emailUnsubscribeRepo) ListAll(ctx context.Context) ([]*model.EmailUnsubscribe, error) {
	var records []*model.EmailUnsubscribe
	if err := r.db.Order("unsubscribed_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}
