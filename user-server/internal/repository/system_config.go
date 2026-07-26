package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SystemConfigRepository 系统配置仓库接口
type SystemConfigRepository interface {
	GetConfig(ctx context.Context) (*model.SystemConfig, error)
	SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error)
	// CountUsers 统计系统用户数
	CountUsers(ctx context.Context) (int64, error)
	// CountAutoReplyLogs 统计自动回复日志数
	CountAutoReplyLogs(ctx context.Context) (int64, error)
	// PingDB 检查数据库连通性
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

func (r *systemConfigRepo) SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error) {
	err := r.db.WithContext(ctx).FirstOrCreate(&config).Error
	if err != nil {
		return nil, err
	}
	return config, nil
}

// CountUsers 统计系统用户数
func (r *systemConfigRepo) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.SystemUser{}).Count(&n).Error
	return n, err
}

// CountAutoReplyLogs 统计自动回复日志数
func (r *systemConfigRepo) CountAutoReplyLogs(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.AutoReplyLog{}).Count(&n).Error
	return n, err
}

// PingDB 检查数据库连通性
func (r *systemConfigRepo) PingDB(ctx context.Context) bool {
	return r.db.WithContext(ctx).Exec("SELECT 1").Error == nil
}
