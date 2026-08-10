package browser

import (
	"context"
	"fmt"
	"time"

	auto_reply_integration "hivemtk-user/internal/aiagent/agent/auto_reply"
	ragcustomerservice "hivemtk-user/internal/aiagent/rag/customer_service"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"

	"hivemtk-user/internal/pkg/utils/logger"
)

type ReplyHandler interface {
	HandleMessage(ctx context.Context, msg Message, platform Platform, userID uint) (*ReplyResult, error)
}

type ReplyResult struct {
	Content    string
	Source     string // rule / rag / fallback
	Confidence float64
	Error      error
}

type IntegrationReplyHandler struct {
	integration      auto_reply_integration.AutoReplyIntegrationService
	ragService       ragcustomerservice.RagCustomerService
	retrievalService ragretrieval.RagRetrievalService
	ruleMatcher      RuleMatcher
	timeout          time.Duration
}

func NewIntegrationReplyHandler(
	integration auto_reply_integration.AutoReplyIntegrationService,
	ragService ragcustomerservice.RagCustomerService,
	retrievalService ragretrieval.RagRetrievalService,
	ruleMatcher RuleMatcher,
) *IntegrationReplyHandler {
	return &IntegrationReplyHandler{
		integration:      integration,
		ragService:       ragService,
		retrievalService: retrievalService,
		ruleMatcher:      ruleMatcher,
		timeout:          15 * time.Second,
	}
}

func (h *IntegrationReplyHandler) HandleMessage(ctx context.Context, msg Message, platform Platform, userID uint) (*ReplyResult, error) {
	startTime := time.Now()

	rule, err := h.ruleMatcher.TestMatching(ctx, string(platform), msg.Content, userID)
	if err == nil && rule != nil {
		return &ReplyResult{
			Content:    rule.ReplyContent,
			Source:     "rule",
			Confidence: 0.8,
		}, nil
	}

	req := &auto_reply_integration.AutoReplyRequest{
		UserID:    fmt.Sprintf("%d", userID),
		SessionID: fmt.Sprintf("%s_%s_%s", platform, msg.SenderID, msg.ChatID),
		Message:   msg.Content,
		Channel:   string(platform),
		ContextData: map[string]any{
			"sender_name": msg.SenderName,
			"sender_id":   msg.SenderID,
			"platform":    string(platform),
			"chat_id":     msg.ChatID,
		},
		Timeout: h.timeout,
	}

	resp, err := h.integration.ProcessAutoReply(ctx, req)
	if err != nil {
		fallback := h.getFallbackReply()
		return &ReplyResult{
			Content:    fallback,
			Source:     "fallback",
			Confidence: 0.0,
			Error:      err,
		}, nil
	}

	logger.Infof("[ReplyHandler] RAG回复: source=%s confidence=%.2f time=%dms",
		resp.Source, resp.Confidence, time.Since(startTime).Milliseconds())

	return &ReplyResult{
		Content:    resp.ReplyMessage,
		Source:     resp.Source,
		Confidence: resp.Confidence,
	}, nil
}

func (h *IntegrationReplyHandler) getFallbackReply() string {
	defaults := []string{
		"感谢您的消息，我们会尽快回复您！",
		"您好，我们收到了您的消息，稍后会为您处理。",
		"谢谢您的咨询，客服会及时为您解答。",
	}
	return defaults[time.Now().UnixNano()%int64(len(defaults))]
}

type SimpleRuleHandler struct {
	matcher RuleMatcher
}

func NewSimpleRuleHandler(matcher RuleMatcher) *SimpleRuleHandler {
	return &SimpleRuleHandler{matcher: matcher}
}

func (h *SimpleRuleHandler) HandleMessage(ctx context.Context, msg Message, platform Platform, userID uint) (*ReplyResult, error) {
	rule, err := h.matcher.TestMatching(ctx, string(platform), msg.Content, userID)
	if err != nil || rule == nil {
		return nil, nil
	}
	return &ReplyResult{
		Content:    rule.ReplyContent,
		Source:     "rule",
		Confidence: 0.8,
	}, nil
}
