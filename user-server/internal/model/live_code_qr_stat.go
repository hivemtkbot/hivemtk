package model

import (
	"time"
)

// LiveCodeQRStat 活码二维码统计模型
type LiveCodeQRStat struct {
	ID         int       `json:"id" gorm:"primaryKey;autoIncrement"`
	QRCodeID   string    `json:"qr_code_id" gorm:"size:36;not null"`
	Date       time.Time `json:"date" gorm:"index"`
	ViewCount  int       `json:"view_count" gorm:"default:0"`
	ClickCount int       `json:"click_count" gorm:"default:0"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回表名
func (LiveCodeQRStat) TableName() string {
	return "live_code_qr_stats"
}
