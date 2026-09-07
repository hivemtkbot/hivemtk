package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// SystemConfigRepository 系统配置仓库接口
type SystemConfigRepository interface {
	GetConfig(ctx context.Context) (*model.SystemConfig, error)
	SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error)
	CountUsers(ctx context.Context) (int64, error)
	PingDB(ctx context.Context) bool
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

// SaveConfig 保存系统配置。
// 系统配置是单例表：已存在（First 命中）时必须用 Save 原地更新——
// 原 FirstOrCreate 在已存在时不写入任何传入字段，前端保存永远是假动作。
func (r *systemConfigRepo) SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error) {
	var existing model.SystemConfig
	err := r.db.WithContext(ctx).First(&existing).Error
	switch {
	case err == nil:
		config.ID = existing.ID
		if err := r.db.WithContext(ctx).Save(config).Error; err != nil {
			return nil, err
		}
		return config, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := r.db.WithContext(ctx).Create(config).Error; err != nil {
			return nil, err
		}
		return config, nil
	default:
		return nil, err
	}
}

func (r *systemConfigRepo) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Count(&n).Error
	return n, err
}

func (r *systemConfigRepo) PingDB(ctx context.Context) bool {
	return r.db.WithContext(ctx).Exec("SELECT 1").Error == nil
}
