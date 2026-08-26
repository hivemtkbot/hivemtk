package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoCrawlerVisit AI 引擎爬虫访问记录（v3 竞品对齐 A6：Profound Agent Analytics 对标）
type GeoCrawlerVisit struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserAgent string    `gorm:"type:varchar(255)" json:"user_agent"`
	Path      string    `gorm:"type:varchar(500)" json:"path"`
	Engine    string    `gorm:"type:varchar(50);index" json:"engine"` // GPTBot/PerplexityBot/ClaudeBot/CCBot...
	IP        string    `gorm:"type:varchar(45)" json:"ip,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoCrawlerVisit) TableName() string { return "geo_crawler_visits" }
