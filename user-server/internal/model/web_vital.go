package model

import "time"

// WebVitalRecord 前端性能指标（Web Vitals: CLS/FID/LCP/FCP/TTFB）
type WebVitalRecord struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Metric    string    `gorm:"type:varchar(16);index;not null" json:"metric"`
	Value     float64   `json:"value"`
	Rating    string    `gorm:"type:varchar(16)" json:"rating"`
	Page      string    `gorm:"type:varchar(300)" json:"page"`
	SessionID string    `gorm:"type:varchar(64);index" json:"session_id"`
	UserAgent string    `gorm:"type:varchar(300)" json:"user_agent"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (WebVitalRecord) TableName() string { return "web_vital_records" }
