package model

import "time"

// QuickReply 快捷回复
type QuickReply struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Category  string    `gorm:"type:varchar(50);index" json:"category"`
	Title     string    `gorm:"type:varchar(100);not null" json:"title"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Channel   string    `gorm:"type:varchar(20)" json:"channel"` // 适用渠道：空=通用 whatsapp/wecom/feishu/telegram/email/sms
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	IsPublic  bool      `gorm:"default:true" json:"is_public"` // 是否公开给所有客服
	CreatedBy uint      `json:"created_by"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (QuickReply) TableName() string {
	return "quick_replies"
}
