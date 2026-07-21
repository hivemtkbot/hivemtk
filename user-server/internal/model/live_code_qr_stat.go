package model

import (
	"time"
)

// LiveCodeQRStat 活码二维码统计模型
type LiveCodeQRStat struct {
	ID         int       `json:"id" gorm:"primaryKey;autoIncrement"`
	QRCodeID   string    `json:"qr_code_id" gorm:"size:36;not null"` // 二维码ID
	Date       time.Time `json:"date" gorm:"index"`                  // 访问日期（按天分组）
	ViewCount  int       `json:"view_count" gorm:"default:0"`        // 浏览次数
	ClickCount int       `json:"click_count" gorm:"default:0"`       // 点击次数
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`   // 创建时间
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`   // 更新时间
}

// TableName 返回表名
func (LiveCodeQRStat) TableName() string {
	return "live_code_qr_stats"
}
