package service

import (
	contentmodel "marketing/internal/content/model"
	"marketing/internal/model"
	db "marketing/internal/pkg/utils/db"
	"marketing/internal/platform"
	"marketing/internal/repository"

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
func (s *SystemConfigService) GetConfig() (*model.SystemConfig, error) {
	config, err := s.repo.GetConfig()
	if err != nil {
		// 没有数据时返回默认配置
		return s.defaultConfig(), nil
	}
	return config, nil
}

// SaveConfig 保存系统配置
func (s *SystemConfigService) SaveConfig(config *model.SystemConfig) (*model.SystemConfig, error) {
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
	return s.repo.SaveConfig(config)
}

// SaveBasicConfig 仅更新应用基础配置（名称、站点 URL），由 service 组装 model 实体
func (s *SystemConfigService) SaveBasicConfig(appName, websiteURL string) (*model.SystemConfig, error) {
	return s.SaveConfig(&model.SystemConfig{
		Name:       appName,
		WebsiteURL: websiteURL,
	})
}

// ResetSystem 重置系统数据
func (s *SystemConfigService) ResetSystem() error {
	// 停止所有后台任务
	platform.StopAllTasks()

	// 获取数据库连接
	d := db.GetDB()

	// 首先确保所有表都存在
	d.AutoMigrate(
		// 开源版：移除 &model.License{}（License 模型删除，授权流程下线）
		&model.SystemUser{},
		&contentmodel.Material{},
		&model.ObsConfig{},
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
		&model.EmailDraft{},
		&model.EmailJobs{},
		&model.EmailList{},
		&model.EmailSend{},
		&model.LiveCode{},
		&model.LiveCodeQR{},
		&model.LiveCodeQRStat{},
		&model.ShortLink{},
		&model.ShortLinkAccess{},
		&model.Order{},
		&model.DailyStats{},
		&model.APILog{},
		&model.VisitLog{},
		&model.SystemMetrics{},
		&model.PaymentConfig{},
	)

	// 删除（或清空）核心业务数据表，使用事务确保原子性
	err := d.Transaction(func(tx *gorm.DB) error {
		tables := []any{
			// 开源版：移除 &model.License{}
			&model.SystemUser{},
			&contentmodel.Material{},
			&model.ObsConfig{},
			&model.AutoReplyAccount{},
			&model.AutoReplyRule{},
			&model.AutoReplyLog{},
			&model.EmailDraft{},
			&model.EmailJobs{},
			&model.EmailList{},
			&model.EmailSend{},
			&model.LiveCode{},
			&model.LiveCodeQR{},
			&model.LiveCodeQRStat{},
			&model.ShortLink{},
			&model.ShortLinkAccess{},
			&model.Order{},
			&model.DailyStats{},
			&model.APILog{},
			&model.VisitLog{},
			&model.SystemMetrics{},
			&model.PaymentConfig{},
		}

		// 使用 GORM 全量删除
		for _, t := range tables {
			if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(t).Error; err != nil {
				// 忽略单表错误，继续清理
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	// 重置系统配置为默认
	if _, err := s.repo.SaveConfig(s.defaultConfig()); err != nil {
		// 忽略配置保存错误
	}

	// 重新启动后台任务
	if err := platform.InitSync(); err != nil {
		return err
	}

	return nil
}

// GetUsageStats 统计用量信息（用户数、请求数近似值）
// 供 app-config 上报使用，避免 controller 直连数据库
func (s *SystemConfigService) GetUsageStats() (userCount int64, requestCount int64) {
	d := db.GetDB()
	if d == nil {
		return 0, 0
	}
	if err := d.Model(&model.User{}).Count(&userCount).Error; err != nil {
		userCount = 0
	}
	// 以自动回复日志数作为请求数近似值
	if err := d.Model(&model.AutoReplyLog{}).Count(&requestCount).Error; err != nil {
		requestCount = 0
	}
	return userCount, requestCount
}

// PingDB 检查数据库连通性，供健康检查使用
func (s *SystemConfigService) PingDB() bool {
	d := db.GetDB()
	if d == nil {
		return false
	}
	sqlDB, err := d.DB()
	if err != nil {
		return false
	}
	return sqlDB.Ping() == nil
}

// defaultConfig 返回默认配置
func (s *SystemConfigService) defaultConfig() *model.SystemConfig {
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
