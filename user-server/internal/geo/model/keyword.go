package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GeoKeyword GEO 关键词模型
type GeoKeyword struct {
	ID           string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Keyword      string         `gorm:"type:text;not null" json:"keyword"`
	Category     string         `gorm:"type:varchar(100)" json:"category"`
	Source       string         `gorm:"type:varchar(50)" json:"source"`
	SearchVolume int            `gorm:"default:0" json:"search_volume"`
	Difficulty   float64        `gorm:"default:0" json:"difficulty"`
	Intent       string         `gorm:"type:varchar(50)" json:"intent"`
	Cluster      string         `gorm:"type:varchar(100);index" json:"cluster"`
	Status       string         `gorm:"type:varchar(20);default:'active'" json:"status"`

	CreatedAt time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time         `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt   `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoKeyword) TableName() string {
	return "geo_keywords"
}

func (m *GeoKeyword) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GeoKeywordGroup GEO 关键词分组模型
type GeoKeywordGroup struct {
	ID           string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name         string         `gorm:"type:varchar(200);not null" json:"name"`
	Description  string         `gorm:"type:text" json:"description"`
	KeywordCount int            `gorm:"default:0" json:"keyword_count"`

	CreatedAt time.Time         `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time         `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt   `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoKeywordGroup) TableName() string {
	return "geo_keyword_groups"
}

func (m *GeoKeywordGroup) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
