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
	// ResetSystemData 清空业务表（保留 system_users/system_config），返回受影响行数近似
	ResetSystemData(ctx context.Context) (int64, error)
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

// ResetSystemData 清空业务表（保留 system_users / system_config）。
// 注意：资产包(AssetBundle)等系统内容表刻意排除，避免误清用户低代码资产。
func (r *systemConfigRepo) ResetSystemData(ctx context.Context) (int64, error) {
	businessModels := []interface{}{
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
		&model.UnifiedMessage{},
		&model.UnifiedReply{},
		&model.Clue{},
		&model.ClueEngagementEvent{},
		&model.ClueScore{},
		&model.CommunityGroup{},
		&model.CommunityMember{},
		&model.CommunityMessage{},
		&model.EmailDraft{},
		&model.EmailJobs{},
		&model.EmailList{},
		&model.EmailSend{},
		&model.EmailSmtp{},
		&model.EmailTrackingEvent{},
		&model.EmailUnsubscribe{},
		&model.Order{},
	}
	var total int64
	for _, m := range businessModels {
		if err := r.db.WithContext(ctx).
			Session(&gorm.Session{AllowGlobalUpdate: true}).
			Where("1 = 1").Delete(m).Error; err != nil {
			return total, err
		}
	}
	return total, nil
}
