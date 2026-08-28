package model

import "time"

// QuickReplyFolder 快捷回复文件夹（客服工作台分组侧栏）
type QuickReplyFolder struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(50);not null;uniqueIndex" json:"name"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (QuickReplyFolder) TableName() string { return "quick_reply_folders" }
