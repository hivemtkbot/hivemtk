// Package repository 提供 AssetBundleVersionLog 的 CRUD 仓储。
//
// 方向9：资产包版本变更日志
// 文档依据：docs/企业级架构优化/资产包模式.md §五
package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AssetBundleVersionLogRepository 资产包版本日志仓储接口
type AssetBundleVersionLogRepository interface {
	Create(ctx context.Context, m *model.AssetBundleVersionLog) error
	List(ctx context.Context, assetID string, limit int) ([]*model.AssetBundleVersionLog, error)
}

// assetBundleVersionLogRepo GORM 实现
type assetBundleVersionLogRepo struct {
	db *gorm.DB
}

// NewAssetBundleVersionLogRepository 构造版本日志仓储
func NewAssetBundleVersionLogRepository(db *gorm.DB) AssetBundleVersionLogRepository {
	return &assetBundleVersionLogRepo{db: db}
}

// Create 新建版本日志
func (r *assetBundleVersionLogRepo) Create(ctx context.Context, m *model.AssetBundleVersionLog) error {
	if m == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// List 列出某资产包的版本日志
func (r *assetBundleVersionLogRepo) List(ctx context.Context, assetID string, limit int) ([]*model.AssetBundleVersionLog, error) {
	var list []*model.AssetBundleVersionLog
	q := r.db.WithContext(ctx).Model(&model.AssetBundleVersionLog{})
	if assetID != "" {
		q = q.Where("asset_id = ?", assetID)
	}
	if limit <= 0 {
		limit = 50
	}
	err := q.Order("created_at DESC").Limit(limit).Find(&list).Error
	return list, err
}
