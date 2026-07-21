package model

import "time"

// KuaishouCardActivity 快手卡片活动记录模型
type KuaishouCardActivity struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	CardID       uint      `json:"card_id" gorm:"not null;index"`         // 卡片ID
	UserID       uint      `json:"user_id" gorm:"index"`                  // 用户ID
	Username     string    `json:"username" gorm:"size:100"`              // 用户名
	ActivityType string    `json:"activity_type" gorm:"size:20;not null"` // 活动类型：view, like, share, collect, comment
	IPAddress    string    `json:"ip_address" gorm:"size:45"`             // 用户IP
	UserAgent    string    `json:"user_agent" gorm:"size:500"`            // 用户代理
	ExtraData    string    `json:"extra_data" gorm:"type:text"`           // 额外数据
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`      // 创建时间

	// 关联
	Card KuaishouCard `gorm:"foreignKey:CardID" json:"card,omitempty"`
}

// TableName 返回表名
func (KuaishouCardActivity) TableName() string {
	return "kuaishou_card_activities"
}
