package model

import (
	"time"
)

// XianyuCardActivity 闲鱼卡片活动记录模型
type XianyuCardActivity struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CardID       uint      `json:"card_id" gorm:"index;not null"`      
	ActivityType string    `json:"activity_type" gorm:"size:50;index"` 
	IP           string    `json:"ip" gorm:"size:45;index"`            
	UserAgent    string    `json:"user_agent" gorm:"size:500"`         
	Referer      string    `json:"referer" gorm:"size:500"`            
	Country      string    `json:"country" gorm:"size:50"`             
	Province     string    `json:"province" gorm:"size:50"`            
	City         string    `json:"city" gorm:"size:50"`                
	DeviceType   string    `json:"device_type" gorm:"size:20"`         
	OS           string    `json:"os" gorm:"size:50"`                  
	Browser      string    `json:"browser" gorm:"size:50"`             
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`   
}

// TableName 返回表名
func (XianyuCardActivity) TableName() string {
	return "xianyu_card_activities"
}

