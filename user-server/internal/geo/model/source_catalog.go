package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoSourceCatalog GEO 信源目录表（v3 信源分级 G5.1）
//
// 维护品牌相关的外部信源列表（央媒/省市/行业/无效），并给出信源等级
// （A/B/C/D）。爬虫任务定期刷新 last_checked 时间，LookupSourceLevel 供
// CrawlerService 查询给定 URL 的信源等级。
type GeoSourceCatalog struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	SourceURL   string         `gorm:"column:source_url;size:512;uniqueIndex" json:"source_url"`
	Domain      string         `gorm:"column:domain;size:256;index" json:"domain"`
	Level       string         `gorm:"column:level;size:2;index" json:"level"`
	Category    string         `gorm:"column:category;size:64;index" json:"category"`
	Description string         `gorm:"column:description;size:512" json:"description"`
	Verified    bool           `gorm:"column:verified;default:true" json:"verified"`
	LastChecked time.Time      `gorm:"column:last_checked" json:"last_checked"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoSourceCatalog) TableName() string { return "geo_source_catalogs" }
