package model

import (
	"time"
)

type CardAccess struct {
	ID         uint      `json:"id" gorm:"primaryKey"`
	CardID     uint      `json:"card_id" gorm:"index;not null"`
	CardType   string    `json:"card_type" gorm:"size:50;index"`
	IPAddress  string    `json:"ip_address" gorm:"size:64"`
	UserAgent  string    `json:"user_agent" gorm:"size:500"`
	Referer    string    `json:"referer" gorm:"size:500"`
	Platform   string    `json:"platform" gorm:"size:50"`
	DeviceType string    `json:"device_type" gorm:"size:20"`
	Browser    string    `json:"browser" gorm:"size:50"`
	OS         string    `json:"os" gorm:"size:50"`
	AccessTime time.Time `json:"access_time" gorm:"autoCreateTime"`
}

func (CardAccess) TableName() string {
	return "card_accesses"
}

type DailyCardUVStats struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CardID    uint      `json:"card_id" gorm:"index;not null"`
	CardType  string    `json:"card_type" gorm:"size:50;index"`
	Date      string    `json:"date" gorm:"size:10;index"`
	UVCount   int       `json:"uv_count" gorm:"default:0"`
	PVCount   int       `json:"pv_count" gorm:"default:0"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (DailyCardUVStats) TableName() string {
	return "daily_card_uv_stats"
}
