package model

import (
	"time"

	"gorm.io/gorm"
)

// GeoAlert GEO 告警记录（负面监控命中 / 异常检测结果）
// 用于前端告警列表展示 + 通知渠道触发
type GeoAlert struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Type      string    `gorm:"column:type;size:64;index" json:"type"`         // negative_monitor / sov_drop / entity_anomaly
	Level     string    `gorm:"column:level;size:16;index" json:"level"`       // warning / critical / info
	BrandName string    `gorm:"column:brand_name;size:256;index" json:"brand_name"`
	Query     string    `gorm:"column:query;size:512" json:"query"`
	Engine    string    `gorm:"column:engine;size:64" json:"engine"`
	Snippet   string    `gorm:"column:snippet;type:text" json:"snippet"`
	Details   string    `gorm:"column:details;type:text" json:"details"`        // 完整上下文或 JSON
	Notified  bool      `gorm:"column:notified;default:false" json:"notified"` // 是否已通知
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (GeoAlert) TableName() string { return "geo_alerts" }
