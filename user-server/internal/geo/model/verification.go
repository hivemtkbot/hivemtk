package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// GeoVerifyResult GEO 品牌验证结果模型
type GeoVerifyResult struct {
	ID            string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	ArticleID     string         `gorm:"type:varchar(36);index" json:"article_id"`
	Model         string         `gorm:"type:varchar(100)" json:"model"`
	Query         string         `gorm:"type:text" json:"query"`
	Response      string         `gorm:"type:text" json:"response"`
	BrandMentioned bool          `gorm:"default:false" json:"brand_mentioned"`
	MentionCount  int            `gorm:"default:0" json:"mention_count"`
	Sentiment     string         `gorm:"type:varchar(20)" json:"sentiment"`
	Position      string         `gorm:"type:varchar(100)" json:"position"`

	CreatedAt time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (m *GeoVerifyResult) TableName() string {
	return "geo_verify_results"
}

func (m *GeoVerifyResult) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}

// GeoAPICall GEO API 调用记录模型（用于成本统计）
type GeoAPICall struct {
	ID           string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Provider     string    `gorm:"type:varchar(50);index" json:"provider"`
	Model        string    `gorm:"type:varchar(100)" json:"model"`
	InputTokens  int       `gorm:"default:0" json:"input_tokens"`
	OutputTokens int       `gorm:"default:0" json:"output_tokens"`
	CostUSD      float64   `gorm:"default:0" json:"cost_usd"`
	CostCNY      float64   `gorm:"default:0" json:"cost_cny"`
	Purpose      string    `gorm:"type:varchar(200)" json:"purpose"`
	Status       string    `gorm:"type:varchar(20)" json:"status"`
	ErrorMsg     string    `gorm:"type:text" json:"error_msg"`

	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (m *GeoAPICall) TableName() string {
	return "geo_api_calls"
}

func (m *GeoAPICall) BeforeCreate(tx *gorm.DB) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	return nil
}
