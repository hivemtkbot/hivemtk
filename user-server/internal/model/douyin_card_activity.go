package model

import "time"

// DouyinCardActivity 抖音卡片活动记录模型
type DouyinCardActivity struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CardID    uint      `json:"card_id" gorm:"not null;index"`    
	UserID    uint      `json:"user_id" gorm:"not null;index"`    
	Action    string    `json:"action" gorm:"size:20;not null"`   
	Username  string    `json:"username" gorm:"size:100"`         
	IPAddress string    `json:"ip_address" gorm:"size:45"`        
	UserAgent string    `json:"user_agent" gorm:"size:500"`       
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"` 
}

// TableName 返回表名
func (DouyinCardActivity) TableName() string {
	return "douyin_card_activities"
}

