package model

import (
	"time"
)

type AutoReplyLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"index" json:"user_id"`
	AccountID     uint      `gorm:"index" json:"account_id"`
	RuleID        uint      `gorm:"index" json:"rule_id"`
	Platform      string    `gorm:"size:20;index" json:"platform"`
	TargetContent string    `gorm:"type:text" json:"target_content"`
	ReplyContent  string    `gorm:"type:text" json:"reply_content"`
	Status        string    `gorm:"size:20" json:"status"`
	ErrorMsg      string    `gorm:"type:text" json:"error_msg"`
	CreatedAt     time.Time `json:"created_at"`
}
