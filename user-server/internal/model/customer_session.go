package model

import "time"

// SessionStatus 会话状态
type SessionStatus string

const (
	SessionStatusPending       SessionStatus = "pending"        // 等待处理
	SessionStatusAIHandling    SessionStatus = "ai_handling"    // AI处理中
	SessionStatusHumanHandling SessionStatus = "human_handling" // 人工处理中
	SessionStatusWaiting       SessionStatus = "waiting"        // 等待用户回复
	SessionStatusResolved      SessionStatus = "resolved"       // 已解决
	SessionStatusClosed        SessionStatus = "closed"         // 已关闭
)

// HandlerType 处理者类型
type HandlerType string

const (
	HandlerTypeAI    HandlerType = "ai"
	HandlerTypeHuman HandlerType = "human"
)

// CustomerSession 客服会话
type CustomerSession struct {
	ID              uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID       string        `gorm:"type:varchar(50);uniqueIndex;not null" json:"session_id"`
	Platform        Platform      `gorm:"type:varchar(20)" json:"platform"`
	AccountID       string        `gorm:"type:varchar(50)" json:"account_id"`
	UserID          string        `gorm:"type:varchar(50);index" json:"user_id"`
	UserName        string        `gorm:"type:varchar(100)" json:"user_name"`
	UserAvatar      string        `gorm:"type:varchar(500)" json:"user_avatar"`
	UserPhone       string        `gorm:"type:varchar(20)" json:"user_phone"`
	UserEmail       string        `gorm:"type:varchar(100)" json:"user_email"`
	Status          SessionStatus `gorm:"type:varchar(20);default:'pending'" json:"status"`
	HandlerType     HandlerType   `gorm:"type:varchar(20)" json:"handler_type"`
	AgentID         uint          `gorm:"index" json:"agent_id"`
	AgentName       string        `gorm:"type:varchar(100)" json:"agent_name"`
	Priority        int           `gorm:"default:0" json:"priority"` // 0-普通, 1-重要, 2-紧急
	LastMessage     string        `gorm:"type:text" json:"last_message"`
	LastMessageAt   *time.Time    `json:"last_message_at"`
	LastMessageBy   string        `gorm:"type:varchar(20)" json:"last_message_by"` // user, ai, agent
	MessageCount    int           `gorm:"default:0" json:"message_count"`
	AIReplyCount    int           `gorm:"default:0" json:"ai_reply_count"`
	HumanReplyCount int           `gorm:"default:0" json:"human_reply_count"`
	AvgResponseTime int           `json:"avg_response_time"` // 平均响应时间(秒)
	Rating          int           `json:"rating"`            // 用户评分 1-5
	RatingComment   string        `gorm:"type:text" json:"rating_comment"`
	Tags            string        `gorm:"type:text" json:"tags"` // JSON数组
	CreatedAt       time.Time     `gorm:"autoCreateTime" json:"created_at"`
	ResolvedAt      *time.Time    `json:"resolved_at"`
	ClosedAt        *time.Time    `json:"closed_at"`
}

// TableName 指定表名
func (CustomerSession) TableName() string {
	return "customer_sessions"
}
