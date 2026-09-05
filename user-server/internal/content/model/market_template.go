package model

import (
	"time"
)

// MarketTemplate 模板市场模板
type MarketTemplate struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"type:varchar(100);not null" json:"name"`
	Description   string    `gorm:"type:varchar(500)" json:"description"`
	Category      string    `gorm:"type:varchar(50);index" json:"category"`
	Type          string    `gorm:"type:varchar(20);index" json:"type"`
	Content       string    `json:"content"`
	Preview       string    `gorm:"type:text" json:"preview"`
	Author        string    `gorm:"type:varchar(50)" json:"author"`
	DownloadCount int       `gorm:"default:0" json:"download_count"`
	Rating        float64   `gorm:"type:decimal(3,2);default:0" json:"rating"`
	IsOfficial    bool      `gorm:"default:false" json:"is_official"`
	IsFree        bool      `gorm:"default:true" json:"is_free"`
	Price         int64     `gorm:"type:bigint;default:0" json:"price"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (MarketTemplate) TableName() string {
	return "market_templates"
}

// MarketTemplateDownload 模板下载记录
type MarketTemplateDownload struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TemplateID   uint      `gorm:"index;not null" json:"template_id"`
	TemplateType string    `gorm:"type:varchar(20)" json:"template_type"`
	DownloadedAt time.Time `gorm:"autoCreateTime" json:"downloaded_at"`
}

// TableName 指定表名
func (MarketTemplateDownload) TableName() string {
	return "market_template_downloads"
}
