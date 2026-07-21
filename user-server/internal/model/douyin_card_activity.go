package model

import "time"

// DouyinCardActivity 抖音卡片活动记录模型
type DouyinCardActivity struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CardID    uint      `json:"card_id" gorm:"not null;index"`    // 卡片ID
	UserID    uint      `json:"user_id" gorm:"not null;index"`    // 用户ID
	Action    string    `json:"action" gorm:"size:20;not null"`   // 活动类型：view, like, share, collect, comment
	Username  string    `json:"username" gorm:"size:100"`         // 用户名
	IPAddress string    `json:"ip_address" gorm:"size:45"`        // IP地址
	UserAgent string    `json:"user_agent" gorm:"size:500"`       // 用户代理
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"` // 创建时间
}

// TableName 返回表名
func (DouyinCardActivity) TableName() string {
	return "douyin_card_activities"
}
