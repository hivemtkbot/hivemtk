package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
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
	Domain     string      `gorm:"type:varchar(500)" json:"domain"`      
	PathPrefix string      `gorm:"type:varchar(100)" json:"path_prefix"` 

	Config string `gorm:"type:text" json:"config"`

	MaxSize  int64 `gorm:"default:104857600" json:"max_size"` 
	MaxCount int   `gorm:"default:1000" json:"max_count"`     

	Status     ObsStatus  `gorm:"type:varchar(20);default:'active'" json:"status"`
	LastError  string     `gorm:"type:text" json:"last_error"`
	LastTestAt *time.Time `gorm:"" json:"last_test_at"`

	TotalSize int64 `gorm:"default:0" json:"total_size"`
	FileCount int   `gorm:"default:0" json:"file_count"`

	IsDefault bool `gorm:"default:false" json:"is_default"`


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

