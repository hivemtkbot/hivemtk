package model

import (
	"time"
)

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

// SessionMessage 会话消息
type SessionMessage struct {
	ID           uint        `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID    string      `gorm:"type:varchar(50);index;not null" json:"session_id"`
	Content      string      `gorm:"type:text" json:"content"`
	ContentType  MessageType `gorm:"type:varchar(20)" json:"content_type"`
	MediaURL     string      `gorm:"type:varchar(500)" json:"media_url"`
	SenderType   string      `gorm:"type:varchar(20);not null" json:"sender_type"` // user, ai, agent
	SenderID     string      `gorm:"type:varchar(50)" json:"sender_id"`
	SenderName   string      `gorm:"type:varchar(100)" json:"sender_name"`
	SenderAvatar string      `gorm:"type:varchar(500)" json:"sender_avatar"`
	AIConfidence float64     `gorm:"type:decimal(5,2)" json:"ai_confidence"`
	AISource     string      `gorm:"type:varchar(20)" json:"ai_source"` // rule, rag, llm
	IsRead       bool        `gorm:"default:false" json:"is_read"`
	ReadAt       *time.Time  `json:"read_at"`
	// DeliveredAt 投递给访客 WebSocket 的时间（nil 表示未投递，用于离线消息）
	DeliveredAt *time.Time `gorm:"index" json:"delivered_at"`
	CreatedAt   time.Time  `gorm:"autoCreateTime;index" json:"created_at"`
}

// TableName 指定表名
func (SessionMessage) TableName() string {
	return "session_messages"
}

// AgentStatus 客服状态
type AgentStatus struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	AgentID         uint       `gorm:"uniqueIndex;not null" json:"agent_id"`
	AgentName       string     `gorm:"type:varchar(100)" json:"agent_name"`
	Status          string     `gorm:"type:varchar(20);default:'offline'" json:"status"` // online, busy, away, offline
	MaxSessions     int        `gorm:"default:5" json:"max_sessions"`
	ActiveSessions  int        `gorm:"default:0" json:"active_sessions"`
	TodaySessions   int        `gorm:"default:0" json:"today_sessions"`
	TodayMessages   int        `gorm:"default:0" json:"today_messages"`
	AvgResponseTime int        `gorm:"default:0" json:"avg_response_time"` // 平均响应时间(秒)
	OnlineAt        *time.Time `json:"online_at"`
	OfflineAt       *time.Time `json:"offline_at"`
	LastActiveAt    *time.Time `json:"last_active_at"`
	UpdatedAt       time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName 指定表名
func (AgentStatus) TableName() string {
	return "agent_statuses"
}

// AISuggestion AI建议
type AISuggestion struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID  string     `gorm:"type:varchar(50);index" json:"session_id"`
	MessageID  uint       `json:"message_id"`
	Suggestion string     `gorm:"type:text" json:"suggestion"`
	Confidence float64    `gorm:"type:decimal(5,2)" json:"confidence"`
	Source     string     `gorm:"type:varchar(20)" json:"source"` // rule, rag, llm
	IsUsed     bool       `gorm:"default:false" json:"is_used"`
	UsedBy     uint       `json:"used_by"` // 使用的客服ID
	UsedAt     *time.Time `json:"used_at"`
	CreatedAt  time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (AISuggestion) TableName() string {
	return "ai_suggestions"
}

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
