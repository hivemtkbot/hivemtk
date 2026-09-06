package model

import (
	"time"
)

// XiaohongshuCardActivity 小红书卡片活动模型
type XiaohongshuCardActivity struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CardID       uint      `gorm:"not null;index" json:"card_id"`
	UserID       uint      `gorm:"not null;index" json:"user_id"`
	Username     string    `gorm:"size:100" json:"username"`
	ActivityType string    `gorm:"size:20;not null" json:"activity_type"`
	Content      string    `gorm:"size:500" json:"content"`
	IPAddress    string    `gorm:"size:45" json:"ip_address"`
	UserAgent    string    `gorm:"size:500" json:"user_agent"`
	ExtraData    string    `gorm:"type:text" json:"extra_data"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	Card XiaohongshuCard `gorm:"foreignKey:CardID" json:"card,omitempty"`
}

// TableName 指定表名
func (XiaohongshuCardActivity) TableName() string {
	return "xiaohongshu_card_activities"
}
