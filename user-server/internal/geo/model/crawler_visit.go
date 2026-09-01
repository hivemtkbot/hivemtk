package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoCrawlerVisit AI 引擎爬虫访问记录（关键词维度：AI Bot 搜索某关键词时发现并访问页面）
type GeoCrawlerVisit struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Keyword   string    `gorm:"type:varchar(200);index" json:"keyword"`     // 关联 geo_keywords.keyword：这条访问是哪个关键词的搜索触发的
	UserAgent string    `gorm:"type:varchar(255)" json:"user_agent"`
	Path      string    `gorm:"type:varchar(500)" json:"path"`
	Engine    string    `gorm:"type:varchar(50);index" json:"engine"`       // GPTBot/PerplexityBot/ClaudeBot/CCBot...
	IP        string    `gorm:"type:varchar(45)" json:"ip,omitempty"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoCrawlerVisit) TableName() string { return "geo_crawler_visits" }
