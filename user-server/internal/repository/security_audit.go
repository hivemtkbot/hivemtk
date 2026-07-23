package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SecurityAuditRepository 安全审计仓库接口
//
// 五层架构：Repository 层负责 SecurityAuditResult 的持久化
// Service 层通过此接口访问数据，不直接持有 *gorm.DB
type SecurityAuditRepository interface {
	Create(ctx context.Context, record *model.SecurityAuditResult) error
	UpdateResults(ctx context.Context, id uint, updates map[string]any) error
	GetByID(ctx context.Context, id uint) (*model.SecurityAuditResult, error)
	List(ctx context.Context, page, pageSize int) ([]*model.SecurityAuditResult, int64, error)
}

type securityAuditRepo struct {
	db *gorm.DB
}

// NewSecurityAuditRepository 创建安全审计仓库实例
func NewSecurityAuditRepository() SecurityAuditRepository {
	return &securityAuditRepo{db: db.GetDB()}
}

func (r *securityAuditRepo) Create(ctx context.Context, record *model.SecurityAuditResult) error {
	return r.db.Create(record).Error
}

func (r *securityAuditRepo) UpdateResults(ctx context.Context, id uint, updates map[string]any) error {
	return r.db.Model(&model.SecurityAuditResult{}).
		Where("id = ?", id).
		Updates(updates).Error
}

func (r *securityAuditRepo) GetByID(ctx context.Context, id uint) (*model.SecurityAuditResult, error) {
	var result model.SecurityAuditResult
	if err := r.db.First(&result, id).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *securityAuditRepo) List(ctx context.Context, page, pageSize int) ([]*model.SecurityAuditResult, int64, error) {
	var list []*model.SecurityAuditResult
	var total int64

	r.db.Model(&model.SecurityAuditResult{}).Count(&total)

	if err := r.db.Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error; err != nil {
		return nil, 0, err
	}

	return list, total, nil
}
