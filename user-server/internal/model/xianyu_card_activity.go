package model

import (
	"time"
)

// XianyuCardActivity 咸鱼卡片活动记录模型
type XianyuCardActivity struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CardID       uint      `json:"card_id" gorm:"index;not null"`      // 卡片ID
	ActivityType string    `json:"activity_type" gorm:"size:50;index"` // 活动类型: view, click, share
	IP           string    `json:"ip" gorm:"size:45;index"`            // IP地址
	UserAgent    string    `json:"user_agent" gorm:"size:500"`         // 用户代理
	Referer      string    `json:"referer" gorm:"size:500"`            // 来源页面
	Country      string    `json:"country" gorm:"size:50"`             // 国家
	Province     string    `json:"province" gorm:"size:50"`            // 省份
	City         string    `json:"city" gorm:"size:50"`                // 城市
	DeviceType   string    `json:"device_type" gorm:"size:20"`         // 设备类型: mobile, pc, tablet
	OS           string    `json:"os" gorm:"size:50"`                  // 操作系统
	Browser      string    `json:"browser" gorm:"size:50"`             // 浏览器
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`   // 创建时间
}

// TableName 返回表名
func (XianyuCardActivity) TableName() string {
	return "xianyu_card_activities"
}
