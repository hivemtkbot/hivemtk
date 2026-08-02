package model

import "time"

// LiveCodeClickLog 活码点击日志（活码维度）
// 记录访客点击活码落地页的原始事件，供 GetStats 聚合真实点击量
type LiveCodeClickLog struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	LiveCodeID string    `json:"live_code_id" gorm:"size:36;not null;index"`
	QRCodeID   string    `json:"qr_code_id" gorm:"size:36;index"`
	UserAgent  string    `json:"user_agent" gorm:"size:500"`
	Referrer   string    `json:"referrer" gorm:"size:500"`
	IPAddress  string    `json:"ip_address" gorm:"size:64"`
	CreatedAt  time.Time `json:"created_at" gorm:"index"`
}

// TableName 返回表名
func (LiveCodeClickLog) TableName() string {
	return "livecode_click_log"
}

// QRCodeClickLog 二维码点击日志（二维码维度）
// 记录访客点击二维码的原始事件，供 GetQRStats 聚合 ViewCount/ClickCount
type QRCodeClickLog struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	QRCodeID   string    `json:"qr_code_id" gorm:"size:36;not null;index"`
	LiveCodeID string    `json:"live_code_id" gorm:"size:36;not null;index"`
	UserAgent  string    `json:"user_agent" gorm:"size:500"`
	Referrer   string    `json:"referrer" gorm:"size:500"`
	IPAddress  string    `json:"ip_address" gorm:"size:64"`
	CreatedAt  time.Time `json:"created_at" gorm:"index"`
}

// TableName 返回表名
func (QRCodeClickLog) TableName() string {
	return "qr_code_click_log"
}
