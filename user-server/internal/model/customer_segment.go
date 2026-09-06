package model

import "time"

// CustomerSegment 客户分群（R46：Builder/RfmMatrix"保存分群"此前为假按钮——真持久化落此表）
type CustomerSegment struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Description string    `gorm:"type:varchar(500)" json:"description"`
	RulesJSON   string    `gorm:"type:text" json:"rules_json"`
	Trigger     string    `gorm:"type:varchar(50)" json:"trigger"`
	Size        int64     `gorm:"default:0" json:"size"`
	CreatedAt   time.Time `gorm:"autoCreateTime;index" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (CustomerSegment) TableName() string { return "customer_segments" }

// Macro 会话宏（R48 T4，对标 Chatwoot Macros）
type Macro struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"type:varchar(100);not null;uniqueIndex" json:"name"`
	Actions   string    `gorm:"type:text;not null" json:"actions"`
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

// AutomationRule 轻量自动化规则（R53 B，Chatwoot automation_rules 精简版）
type AutomationRule struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name         string    `gorm:"type:varchar(100);not null" json:"name"`
	Event        string    `gorm:"type:varchar(40);not null;index" json:"event"`
	Conditions   string    `gorm:"type:text" json:"conditions"`
	Actions      string    `gorm:"type:text;not null" json:"actions"`
	DelayMinutes int       `gorm:"default:0" json:"delay_minutes"`
	Priority     int       `gorm:"default:0" json:"priority"`
	Enabled      bool      `gorm:"default:true;index" json:"enabled"`
	RunCount     int64     `gorm:"default:0" json:"run_count"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (AutomationRule) TableName() string { return "automation_rules" }

// RulePendingExecution 自动化规则延迟执行队列（R53 B）
type RulePendingExecution struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleID    uint      `gorm:"index;not null" json:"rule_id"`
	SessionID string    `gorm:"type:varchar(120);not null" json:"session_id"`
	ExecuteAt time.Time `gorm:"index;not null" json:"execute_at"`
	Status    string    `gorm:"type:varchar(20);default:'pending'" json:"status"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (RulePendingExecution) TableName() string { return "rule_pending_executions" }
