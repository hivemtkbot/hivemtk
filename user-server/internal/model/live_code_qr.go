package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// LiveCodeQR 活码二维码模型
type LiveCodeQR struct {
	ID         string    `json:"id" gorm:"primaryKey;size:36"`
	LiveCodeID string    `json:"live_code_id" gorm:"size:36;not null"`
	QRType     string    `json:"qr_type" gorm:"size:50;not null"`
	QRContent  string    `json:"qr_content" gorm:"not null"`
	QRTitle    string    `json:"qr_title" gorm:"size:255"`
	ImageURL   string    `json:"image_url" gorm:"size:255"`
	Priority   int       `json:"priority" gorm:"default:1"`
	DailyLimit int       `json:"daily_limit" gorm:"default:200"`
	ExpireDays int       `json:"expire_days" gorm:"default:7"`
	Status     int       `json:"status" gorm:"default:1"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt  time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 返回表名
func (LiveCodeQR) TableName() string {
	return "live_code_qrs"
}

// BeforeCreate GORM钩子，在创建前生成UUID
func (l *LiveCodeQR) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}
