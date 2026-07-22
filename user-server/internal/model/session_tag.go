package model

import "time"

// SessionTag 会话标签
type SessionTag struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(50);not null" json:"name"`
	Code        string    `gorm:"type:varchar(50);uniqueIndex" json:"code"` // 英文/拼音标识，如 vip
	Group       string    `gorm:"type:varchar(50)" json:"group"`            // 分组：客户类型/意向度
	Color       string    `gorm:"type:varchar(20)" json:"color"`
	Description string    `gorm:"type:varchar(200)" json:"description"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (SessionTag) TableName() string {
	return "session_tags"
}
