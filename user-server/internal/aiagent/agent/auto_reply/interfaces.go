package auto_reply_integration

import (
	"context"
	"time"

	ragcustomerservice "hivemtk-user/internal/aiagent/rag/customer_service"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
)

// AutoReplyRequest 表示自动回复请求
type AutoReplyRequest struct {
	UserID      string         `json:"user_id"`
	SessionID   string         `json:"session_id"`
	Message     string         `json:"message"`
	ContextData map[string]any `json:"context_data,omitempty"`
	Channel     string         `json:"channel"` // 如: "wechat", "website", "app"
	Timeout     time.Duration  `json:"timeout,omitempty"`
}

// AutoReplyResponse 表示自动回复响应
type AutoReplyResponse struct {
	ReplyMessage   string        `json:"reply_message"`
	Confidence     float64       `json:"confidence"`
	Source         string        `json:"source"` // "rule_based", "rag", "fallback"
	IntentDetected string        `json:"intent_detected,omitempty"`
	Entities       []string      `json:"entities,omitempty"`
	ProcessingTime time.Duration `json:"processing_time"`
	Error          error         `json:"error,omitempty"`
}

// AutoReplyIntegrationService 自动回复集成服务接口
type AutoReplyIntegrationService interface {
	// ProcessAutoReply 处理自动回复请求
	ProcessAutoReply(ctx context.Context, req *AutoReplyRequest) (*AutoReplyResponse, error)

	// HealthCheck 检查服务健康状态
	HealthCheck(ctx context.Context) bool

	// GetCapabilities 获取服务支持的功能
	GetCapabilities() []string
}

// RuleBasedMatcher 规则匹配器接口
type RuleBasedMatcher interface {
	// MatchRule 匹配规则
	MatchRule(message string, contextData map[string]any) (bool, string, float64, error)

	// GetFallbackReply 获取备用回复
	GetFallbackReply() string
}

// ReplyProcessor 回复处理器接口
type ReplyProcessor interface {
	// ProcessReply 处理回复
	ProcessReply(ctx context.Context, req *AutoReplyRequest, ragService ragcustomerservice.RagCustomerService,
		retrievalService ragretrieval.RagRetrievalService) (*AutoReplyResponse, error)
}
