package model

import (
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type ObsProvider string

const (
	ObsProviderAliyun  ObsProvider = "aliyun"
	ObsProviderQiniu   ObsProvider = "qiniu"
	ObsProviderTencent ObsProvider = "tencent"
	ObsProviderAWS     ObsProvider = "aws"
	ObsProviderLocal   ObsProvider = "local"
)

type ObsStatus string

const (
	ObsStatusActive   ObsStatus = "active"
	ObsStatusInactive ObsStatus = "inactive"
	ObsStatusError    ObsStatus = "error"
)

// ObsConfig 对象存储配置模型
type ObsConfig struct {
	ID         string      `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name       string      `gorm:"type:varchar(100);not null" json:"name"`
	Provider   ObsProvider `gorm:"type:varchar(20);not null" json:"provider"`
	AccessKey  string      `gorm:"type:varchar(255);not null" json:"access_key"`
	SecretKey  string      `gorm:"type:varchar(255);not null" json:"secret_key"`
	Bucket     string      `gorm:"type:varchar(100);not null" json:"bucket"`
	Region     string      `gorm:"type:varchar(50)" json:"region"`
	Endpoint   string      `gorm:"type:varchar(500)" json:"endpoint"`
	Domain     string      `gorm:"type:varchar(500)" json:"domain"`      // 自定义域名
	PathPrefix string      `gorm:"type:varchar(100)" json:"path_prefix"` // 路径前缀

	// 高级配置（JSON格式）
	Config string `gorm:"type:text" json:"config"`

	// 使用限制
	MaxSize  int64 `gorm:"default:104857600" json:"max_size"` // 单个文件最大大小（默认100MB）
	MaxCount int   `gorm:"default:1000" json:"max_count"`     // 最大文件数量

	// 状态
	Status     ObsStatus  `gorm:"type:varchar(20);default:'active'" json:"status"`
	LastError  string     `gorm:"type:text" json:"last_error"`
	LastTestAt *time.Time `gorm:"" json:"last_test_at"`

	// 统计信息
	TotalSize int64 `gorm:"default:0" json:"total_size"`
	FileCount int   `gorm:"default:0" json:"file_count"`

	// 是否默认配置
	IsDefault bool `gorm:"default:false" json:"is_default"`

	// 开源版：已移除 License 关联字段（License 模型已删除）
	// LicenseID string   `gorm:"type:varchar(36)" json:"license_id"`
	// License   *License `gorm:"foreignKey:LicenseID" json:"license,omitempty"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (o *ObsConfig) TableName() string {
	return "obs_config"
}

func (o *ObsConfig) BeforeCreate(tx *gorm.DB) error {
	if o.ID == "" {
		o.ID = uuid.New().String()
	}
	return nil
}

// 获取提供商名称
func (o *ObsConfig) GetProviderName() string {
	switch o.Provider {
	case ObsProviderAliyun:
		return "阿里云OSS"
	case ObsProviderQiniu:
		return "七牛云存储"
	case ObsProviderTencent:
		return "腾讯云COS"
	case ObsProviderAWS:
		return "AWS S3"
	case ObsProviderLocal:
		return "本地存储"
	default:
		return "未知"
	}
}

// 检查是否活跃
func (o *ObsConfig) IsActive() bool {
	return o.Status == ObsStatusActive
}

// 获取完整的上传路径
func (o *ObsConfig) GetFullPath(fileName string) string {
	if o.PathPrefix != "" {
		return o.PathPrefix + "/" + fileName
	}
	return fileName
}

// 获取访问URL
func (o *ObsConfig) GetAccessURL(filePath string) string {
	if o.Domain != "" {
		return o.Domain + "/" + filePath
	}

	// 根据提供商生成默认访问URL
	switch o.Provider {
	case ObsProviderAliyun:
		return "https://" + o.Bucket + "." + o.Region + ".aliyuncs.com/" + filePath
	case ObsProviderQiniu:
		return "http://" + o.Domain + "/" + filePath
	case ObsProviderTencent:
		return "https://" + o.Bucket + ".cos." + o.Region + ".myqcloud.com/" + filePath
	case ObsProviderAWS:
		return "https://" + o.Bucket + ".s3." + o.Region + ".amazonaws.com/" + filePath
	default:
		return filePath
	}
}

// 检查文件大小限制
func (o *ObsConfig) IsFileSizeAllowed(size int64) bool {
	return size <= o.MaxSize
}

// 检查是否达到文件数量限制
func (o *ObsConfig) IsFileCountAllowed() bool {
	return o.FileCount < o.MaxCount
}
