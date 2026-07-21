package dto

import "time"

// AutoReplyRuleRequest 自动回复规则请求
type AutoReplyRuleRequest struct {
	UserID       uint    `json:"user_id" binding:"required"`
	Platform     string  `json:"platform" binding:"required"`
	Keywords     string  `json:"keywords" binding:"required"`
	ReplyContent string  `json:"reply_content" binding:"required"`
	Frequency    int     `json:"frequency"`
	DailyLimit   int     `json:"daily_limit"`
	StartTime    *string `json:"start_time,omitempty"` // 开始时间 (HH:MM格式)
	EndTime      *string `json:"end_time,omitempty"`   // 结束时间 (HH:MM格式)
	IsActive     bool    `json:"is_active"`
	IsRagEnabled bool    `json:"is_rag_enabled"`
	RagProductID *string `json:"rag_product_id,omitempty"`
}

// AutoReplyRuleListRequest 自动回复规则列表请求
type AutoReplyRuleListRequest struct {
	Platform string `form:"platform"`
	UserID   uint   `form:"user_id"`
	IsActive *bool  `form:"is_active"`
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// AutoReplyRuleResponse 自动回复规则响应
type AutoReplyRuleResponse struct {
	ID           uint      `json:"id"`
	UserID       uint      `json:"user_id"`
	Platform     string    `json:"platform"`
	Keywords     string    `json:"keywords"`
	ReplyContent string    `json:"reply_content"`
	Frequency    int       `json:"frequency"`
	DailyLimit   int       `json:"daily_limit"`
	StartTime    *string   `json:"start_time,omitempty"` // 开始时间 (HH:MM格式)
	EndTime      *string   `json:"end_time,omitempty"`   // 结束时间 (HH:MM格式)
	IsActive     bool      `json:"is_active"`
	IsRagEnabled bool      `json:"is_rag_enabled"`
	RagProductID *uint     `json:"rag_product_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AutoReplyTestRequest 自动回复测试请求
type AutoReplyTestRequest struct {
	Platform  string `json:"platform" binding:"required"`
	Message   string `json:"message" binding:"required"`
	UserID    uint   `json:"user_id"`
	AccountID uint   `json:"account_id"`
}

// AutoReplyTestResponse 自动回复测试响应
type AutoReplyTestResponse struct {
	Matched   bool                   `json:"matched"`
	Rule      *AutoReplyRuleResponse `json:"rule,omitempty"`
	Message   string                 `json:"message"`
	Platform  string                 `json:"platform"`
	Timestamp time.Time              `json:"timestamp"`
}

// AutoReplySimulateRequest 模拟消息请求
type AutoReplySimulateRequest struct {
	Platform  string `json:"platform" binding:"required"`
	Message   string `json:"message" binding:"required"`
	Sender    string `json:"sender"`
	UserID    uint   `json:"user_id"`
	AccountID uint   `json:"account_id"`
}

// AutoReplySimulateResponse 模拟消息响应
type AutoReplySimulateResponse struct {
	LogID     uint      `json:"log_id"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Reply     string    `json:"reply"`
	Timestamp time.Time `json:"timestamp"`
}

// AutoReplyBatchTestRequest 批量测试请求
type AutoReplyBatchTestRequest struct {
	Platform  string   `json:"platform" binding:"required"`
	Messages  []string `json:"messages" binding:"required"`
	UserID    uint     `json:"user_id"`
	AccountID uint     `json:"account_id"`
}

// AutoReplyRateLimitTestRequest 速率限制测试请求
type AutoReplyRateLimitTestRequest struct {
	Platform  string `json:"platform" binding:"required"`
	UserID    uint   `json:"user_id"`
	AccountID uint   `json:"account_id"`
	TestCount int    `json:"test_count"`
}

// AutoReplyRateLimitStatsResponse 速率限制统计响应
type AutoReplyRateLimitStatsResponse struct {
	Platform    string     `json:"platform"`
	UserID      uint       `json:"user_id"`
	AccountID   uint       `json:"account_id"`
	DailySent   int        `json:"daily_sent"`
	DailyLimit  int        `json:"daily_limit"`
	ResetTime   time.Time  `json:"reset_time"`
	LastReplyAt *time.Time `json:"last_reply_at,omitempty"`
}

// AutoReplyConcurrentStatsResponse 并发统计响应
type AutoReplyConcurrentStatsResponse struct {
	Platform       string `json:"platform"`
	UserID         uint   `json:"user_id"`
	ActiveBots     int    `json:"active_bots"`
	MaxConcurrent  int    `json:"max_concurrent"`
	QueueSize      int    `json:"queue_size"`
	TotalProcessed int    `json:"total_processed"`
}
