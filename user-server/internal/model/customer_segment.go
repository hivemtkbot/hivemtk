package model

import "time"

// CustomerSegment 客户分群（R46：Builder/RfmMatrix"保存分群"此前为假按钮——真持久化落此表）
type CustomerSegment struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	RulesJSON   string    `gorm:"type:text" json:"rules_json"` // 规则树/RFM 快照 JSON
	Trigger     string    `gorm:"type:varchar(50)" json:"trigger"`
	Size        int64     `gorm:"default:0" json:"size"` // 创建时规模估算
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CustomerSegment) TableName() string { return "customer_segments" }

// Macro 会话宏（R48 T4，对标 Chatwoot Macros）
type Macro struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"name"`
	Actions   string    `gorm:"type:text;not null" json:"actions"` // 动作序列 JSON
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Macro) TableName() string { return "macros" }

// SessionAISummary AI 会话摘要（R48 T5）
type SessionAISummary struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID string    `gorm:"type:varchar(120);uniqueIndex;not null" json:"session_id"`
	Summary   string    `gorm:"type:text" json:"summary"`
	Sentiment string    `gorm:"type:varchar(20)" json:"sentiment"`
	Model     string    `gorm:"type:varchar(60)" json:"model"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SessionAISummary) TableName() string { return "session_ai_summaries" }

// WebhookSubscription 出站 Webhook 订阅（R48 T6）
type WebhookSubscription struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	URL       string    `gorm:"type:varchar(500);not null" json:"url"`
	Events    string    `gorm:"type:varchar(500);not null" json:"events"`
	Secret    string    `gorm:"type:varchar(120);not null" json:"secret"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (WebhookSubscription) TableName() string { return "webhook_subscriptions" }

// SavedView 保存的自定义视图（R48 T8）
type SavedView struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"type:varchar(100);not null" json:"name"`
	Route     string    `gorm:"type:varchar(100);not null" json:"route"`
	Filter    string    `gorm:"type:text" json:"filter"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (SavedView) TableName() string { return "saved_views" }

// ReportSubscription 定时邮件报表订阅（R48 T9）
type ReportSubscription struct {
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Email     string     `gorm:"type:varchar(200);not null;uniqueIndex" json:"email"`
	Schedule  string     `gorm:"type:varchar(20);default:'daily'" json:"schedule"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastSent  *time.Time `json:"last_sent,omitempty"`
	CreatedAt time.Time  `gorm:"autoCreateTime" json:"created_at"`
}

func (ReportSubscription) TableName() string { return "report_subscriptions" }
