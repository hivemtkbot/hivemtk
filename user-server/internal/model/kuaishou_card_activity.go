package model

import "time"

// KuaishouCardActivity 快手卡片活动记录模型
type KuaishouCardActivity struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CardID       uint      `json:"card_id" gorm:"not null;index"`         
	UserID       uint      `json:"user_id" gorm:"index"`                  
	Username     string    `json:"username" gorm:"size:100"`              
	ActivityType string    `json:"activity_type" gorm:"size:20;not null"` 
	IPAddress    string    `json:"ip_address" gorm:"size:45"`             
	UserAgent    string    `json:"user_agent" gorm:"size:500"`            
	ExtraData    string    `json:"extra_data" gorm:"type:text"`           
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`      

	Card KuaishouCard `gorm:"foreignKey:CardID" json:"card,omitempty"`
}

// TableName 返回表名
func (KuaishouCardActivity) TableName() string {
	return "kuaishou_card_activities"
}

