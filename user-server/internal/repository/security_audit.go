package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// SecurityAuditRepository 安全审计仓储（OPT-ARC-01）
//
// 与其它 repository 保持一致的形态：
//   - 无参构造（NewSecurityAuditRepository）
//   - SetDB 注入 db（兼容测试与多租户）
//   - GetDB 返回当前 db（供 service 层 withDB 统一包装）
type SecurityAuditRepository struct {
	db *gorm.DB
}

// NewSecurityAuditRepository 创建安全审计仓储
func NewSecurityAuditRepository() *SecurityAuditRepository {
	return &SecurityAuditRepository{}
}

// SetDB 注入 db
func (r *SecurityAuditRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// GetDB 获取 db（内部 / service 层 withDB 使用）
func (r *SecurityAuditRepository) GetDB(ctx context.Context) *gorm.DB {
	return r.db
}

// Create 写入一条审计记录
func (r *SecurityAuditRepository) Create(ctx context.Context, a *model.SecurityAudit) error {
	return r.db.WithContext(ctx).Create(a).Error
}

// List 分页查询审计记录
func (r *SecurityAuditRepository) List(ctx context.Context, page, pageSize int) ([]model.SecurityAudit, int64, error) {
	var total int64
	if err := r.db.WithContext(ctx).Model(&model.SecurityAudit{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.SecurityAudit
	offset := (page - 1) * pageSize
	if err := r.db.WithContext(ctx).Order("id DESC").Offset(offset).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

// GetByID 获取审计详情（含 items）
func (r *SecurityAuditRepository) GetByID(ctx context.Context, id uint) (*model.SecurityAudit, error) {
	var a model.SecurityAudit
	if err := r.db.WithContext(ctx).Preload("Items").First(&a, id).Error; err != nil {
		return nil, err
	}
	return &a, nil
}
