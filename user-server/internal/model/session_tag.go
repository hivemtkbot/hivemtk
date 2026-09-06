package model

import "time"

// SessionTag 会话标签
type SessionTag struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"type:varchar(50);not null" json:"name"`
	Code          string    `gorm:"type:varchar(50);uniqueIndex" json:"code"`
	Group         string    `gorm:"type:varchar(50)" json:"group"`
	Color         string    `gorm:"type:varchar(20)" json:"color"`
	Description   string    `gorm:"type:varchar(200)" json:"description"`
	SortOrder     int       `gorm:"default:0" json:"sort_order"`
	RuleCondition string    `gorm:"type:varchar(500)" json:"rule_condition"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (SessionTag) TableName() string {
	return "session_tags"
}
