package model

// SystemConfig 系统配置模型
type SystemConfig struct {
	ID                   uint   `gorm:"primaryKey" json:"id"`
	Name                 string `gorm:"size:255;not null" json:"site_name"`         // 站点名称
	WebsiteURL           string `gorm:"size:255" json:"website_url"`                // 网站URL
	LogoURL              string `gorm:"size:500" json:"logo_url"`                   // 站点 Logo URL
	ThemeColor           string `gorm:"size:20" json:"theme_color"`                 // 主题色
	SEOKeywords          string `gorm:"size:500" json:"seo_keywords"`               // SEO 关键词
	SEODescription       string `gorm:"type:text" json:"seo_description"`           // SEO 描述
	ServicePhone         string `gorm:"size:50" json:"service_phone"`               // 客服电话
	ServiceEmail         string `gorm:"size:100" json:"service_email"`              // 客服邮箱
	ICPRecord            string `gorm:"size:100" json:"icp_record"`                 // ICP 备案号
	PoliceRecord         string `gorm:"size:100" json:"police_record"`              // 公安备案号
	EnableRegister       bool   `gorm:"default:true" json:"enable_register"`        // 启用注册
	EnableEmailMarketing bool   `gorm:"default:true" json:"enable_email_marketing"` // 启用邮件营销
	EnableRAG            bool   `gorm:"default:true" json:"enable_rag"`             // 启用 RAG 智能体
	MaintenanceMode      bool   `gorm:"default:false" json:"maintenance_mode"`      // 维护模式
	// 私域独立部署:不限制用户数(MaxUsers 保留为兼容旧字段,固定 0,不再用于业务限制)
	MaxUsers          int  `gorm:"default:0" json:"max_users"`
	MaxUploadSizeMB   int  `gorm:"default:50" json:"max_upload_size_mb"`    // 最大上传文件大小 (MB)
}

func (*SystemConfig) TableName() string {
	return "system_config"
}
