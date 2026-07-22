package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SystemConfigRepository 系统配置仓库接口
type SystemConfigRepository interface {
	GetConfig() (*model.SystemConfig, error)
	SaveConfig(config *model.SystemConfig) (*model.SystemConfig, error)
}

type systemConfigRepo struct {
	db *gorm.DB
}

// NewSystemConfigRepository 创建系统配置仓库实例
func NewSystemConfigRepository() SystemConfigRepository {
	return &systemConfigRepo{db: _db.GetDB()}
}

func (r *systemConfigRepo) GetConfig() (*model.SystemConfig, error) {
	var config model.SystemConfig
	err := r.db.First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *systemConfigRepo) SaveConfig(config *model.SystemConfig) (*model.SystemConfig, error) {
	err := r.db.FirstOrCreate(&config).Error
	if err != nil {
		return nil, err
	}
	return config, nil
}
