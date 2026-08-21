package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GeoConfig GEO 全局配置模型（单例：固定 ID=1）
type GeoConfig struct {
	ID               string `gorm:"type:varchar(36);primaryKey" json:"id"`
	BrandName        string `gorm:"type:varchar(200)" json:"brand_name"`
	BrandDescription string `gorm:"type:text" json:"brand_description"`
	Advantages       string `gorm:"type:text" json:"advantages"`
	Competitors      string `gorm:"type:text" json:"competitors"`
	Domain           string `gorm:"type:varchar(500)" json:"domain"`
	Language         string `gorm:"type:varchar(20);default:'zh'" json:"language"`
	DefaultModel     string `gorm:"type:varchar(100)" json:"default_model"`
	VerifyModels     string `gorm:"type:text" json:"verify_models"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoConfig) TableName() string {
	return "geo_config"
}

func (m *GeoConfig) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GeoPlatformAccount GEO 平台账号模型
type GeoPlatformAccount struct {
	ID          string `gorm:"type:varchar(36);primaryKey" json:"id"`
	Platform    string `gorm:"type:varchar(50);index" json:"platform"`
	AccountID   string `gorm:"type:varchar(200)" json:"account_id"`
	AccountName string `gorm:"type:varchar(200)" json:"account_name"`
	Status      string `gorm:"type:varchar(20);default:'active'" json:"status"`
	Config      string `gorm:"type:text" json:"config"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoPlatformAccount) TableName() string {
	return "geo_platform_accounts"
}

func (m *GeoPlatformAccount) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GeoPublishRecord GEO 发布记录模型
type GeoPublishRecord struct {
	ID           string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ArticleID    string    `gorm:"type:varchar(36);index" json:"article_id"`
	Platform     string    `gorm:"type:varchar(50)" json:"platform"`
	AccountID    string    `gorm:"type:varchar(200)" json:"account_id"`
	Status       string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	PublishedURL string    `gorm:"type:varchar(500)" json:"published_url"`
	PublishedAt  time.Time `json:"published_at"`

	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoPublishRecord) TableName() string {
	return "geo_publish_records"
}

func (m *GeoPublishRecord) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
