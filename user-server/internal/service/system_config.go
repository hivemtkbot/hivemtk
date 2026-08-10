package service

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/repository"

	"context"

	"gorm.io/gorm"
)

// SystemConfigService 系统配置服务
type SystemConfigService struct {
	repo repository.SystemConfigRepository
}

// NewSystemConfigService 创建系统配置服务实例
func NewSystemConfigService() *SystemConfigService {
	return &SystemConfigService{repo: repository.NewSystemConfigRepository()}
}

// GetConfig 获取系统配置
func (s *SystemConfigService) GetConfig(ctx context.Context) (*model.SystemConfig, error) {
	config, err := s.repo.GetConfig(ctx)
	if err != nil {
		// 没有数据时返回默认配置
		return s.defaultConfig(ctx), nil
	}
	return config, nil
}

// SaveConfig 保存系统配置
func (s *SystemConfigService) SaveConfig(ctx context.Context, config *model.SystemConfig) (*model.SystemConfig, error) {
	if config == nil {
		return nil, gorm.ErrInvalidData
	}
	// 私域部署:MaxUsers 兼容旧字段,固定 0 表示不限制用户数
	if config.MaxUsers < 0 {
		config.MaxUsers = 0
	}
	// 兜底:上传文件大小
	if config.MaxUploadSizeMB <= 0 {
		config.MaxUploadSizeMB = 50
	}
	// 兜底:主题色
	if config.ThemeColor == "" {
		config.ThemeColor = "#409EFF"
	}
	return s.repo.SaveConfig(ctx, config)
}

// SaveBasicConfig 仅更新应用基础配置（名称、站点 URL），由 service 组装 model 实体
func (s *SystemConfigService) SaveBasicConfig(ctx context.Context, appName, websiteURL string) (*model.SystemConfig, error) {
	return s.SaveConfig(ctx, &model.SystemConfig{
		Name:       appName,
		WebsiteURL: websiteURL,
	})
}

// GetUsageStats 统计用量信息（用户数、请求数近似值）
// 供 app-config 上报使用，避免 controller 直连数据库
func (s *SystemConfigService) GetUsageStats(ctx context.Context) (userCount int64, requestCount int64) {
	if n, err := s.repo.CountUsers(ctx); err == nil {
		userCount = n
	}
	if n, err := s.repo.CountAutoReplyLogs(ctx); err == nil {
		requestCount = n
	}
	return userCount, requestCount
}

// PingDB 检查数据库连通性，供健康检查使用
func (s *SystemConfigService) PingDB(ctx context.Context) bool {
	return s.repo.PingDB(ctx)
}

// defaultConfig 返回默认配置
func (s *SystemConfigService) defaultConfig(ctx context.Context) *model.SystemConfig {
	return &model.SystemConfig{
		Name:                 "",
		WebsiteURL:           "",
		LogoURL:              "",
		ThemeColor:           "#409EFF",
		SEOKeywords:          "",
		SEODescription:       "",
		ServicePhone:         "",
		ServiceEmail:         "",
		ICPRecord:            "",
		PoliceRecord:         "",
		EnableRegister:       true,
		EnableEmailMarketing: true,
		EnableRAG:            true,
		MaintenanceMode:      false,
		// 私域独立部署:不限制用户数(MaxUsers 保留为兼容字段,固定 0)
		MaxUsers:          0,
		MaxUploadSizeMB:   50,
		AutoReplyHeadless: true,
	}
}
