package model

import (
	"time"
)

type AutoReplyRule struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"user_id"`
	Platform     string    `gorm:"size:20;index" json:"platform"`
	Keywords     string    `gorm:"column:keyword;type:text" json:"keywords"` // 兼容数据库字段名
	ReplyContent string    `gorm:"type:text" json:"reply_content"`
	Frequency    int       `gorm:"default:60" json:"frequency"`
	DailyLimit   int       `gorm:"default:100" json:"daily_limit"`
	StartTime    *string   `gorm:"size:10" json:"start_time,omitempty"` // 开始时间 (HH:MM格式)
	EndTime      *string   `gorm:"size:10" json:"end_time,omitempty"`   // 结束时间 (HH:MM格式)
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	IsRagEnabled bool      `gorm:"default:false" json:"is_rag_enabled"`
	RagProductID *string   `gorm:"size:64;index" json:"rag_product_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RateLimitTestResult 速率限制测试结果
type RateLimitTestResult struct {
	Platform  string    `json:"platform"`
	UserID    uint      `json:"user_id"`
	AccountID uint      `json:"account_id"`
	TestID    int       `json:"test_id"`
	Allowed   bool      `json:"allowed"`
	ErrorMsg  string    `json:"error_msg"`
	Timestamp time.Time `json:"timestamp"`
}
