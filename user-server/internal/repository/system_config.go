package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SystemConfigRepository 系统配置仓库接口
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context，透传至 r.db.WithContext(ctx)，
// 保证 trace / timeout / cancel 在整条调用链上贯通。
type SystemConfigRepository interface {
	GetConfig(ctx context.Context) (*model.SystemConfig, error)
	SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error)
}

type systemConfigRepo struct {
	db *gorm.DB
}

// NewSystemConfigRepository 创建系统配置仓库实例
func NewSystemConfigRepository() SystemConfigRepository {
	return &systemConfigRepo{db: _db.GetDB()}
}

func (r *systemConfigRepo) GetConfig(ctx context.Context) (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := r.db.WithContext(ctx).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *systemConfigRepo) SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error) {
	err := r.db.WithContext(ctx).FirstOrCreate(&config).Error
	if err != nil {
		return nil, err
	}
	return config, nil
}
