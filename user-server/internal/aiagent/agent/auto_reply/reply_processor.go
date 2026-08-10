package auto_reply_integration

import (
	"context"
	"fmt"
	"time"

	ragcustomerservice "hivemtk-user/internal/aiagent/rag/customer_service"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
)

// ReplyProcessorImpl 回复处理器实现
type ReplyProcessorImpl struct {
	ruleMatcher RuleBasedMatcher
	timeout     time.Duration
}

// NewReplyProcessorImpl 创建新的回复处理器实例
func NewReplyProcessorImpl(ruleMatcher RuleBasedMatcher, timeout time.Duration) *ReplyProcessorImpl {
	if timeout <= 0 {
		timeout = 10 * time.Second // 默认超时时间
	}

	return &ReplyProcessorImpl{
		ruleMatcher: ruleMatcher,
		timeout:     timeout,
	}
}

// ProcessReply 处理回复
func (rp *ReplyProcessorImpl) ProcessReply(ctx context.Context, req *AutoReplyRequest,
	ragService ragcustomerservice.RagCustomerService,
	retrievalService ragretrieval.RagRetrievalService) (*AutoReplyResponse, error) {
	startTime := time.Now()

	// 首先尝试规则匹配
	match, ruleReply, confidence, err := rp.ruleMatcher.MatchRule(req.Message, req.ContextData)
	if err != nil {
		return &AutoReplyResponse{
			ReplyMessage:   rp.ruleMatcher.GetFallbackReply(),
			Confidence:     0.0,
			Source:         "fallback",
			ProcessingTime: time.Since(startTime),
			Error:          err,
		}, nil
	}

	// 如果规则匹配成功，直接返回规则匹配结果
	if match {
		return &AutoReplyResponse{
			ReplyMessage:   ruleReply,
			Confidence:     confidence,
			Source:         "rule_based",
			ProcessingTime: time.Since(startTime),
		}, nil
	}

	// 如果规则匹配失败，则使用RAG系统
	// 设置超时控制
	ctxWithTimeout, cancel := context.WithTimeout(ctx, rp.timeout)
	defer cancel()

	// 创建会话ID，如果未提供则生成一个
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("session_%d", time.Now().Unix())
	}

	// 准备RAG客服服务的消息对象
	userMessage := ragcustomerservice.Message{
		ID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		SessionID: sessionID,
		Role:      ragcustomerservice.MessageRoleUser,
		Content:   req.Message,
		Timestamp: time.Now(),
		Metadata:  req.ContextData,
	}

	// 尝试获取现有会话或创建新会话
	var session ragcustomerservice.Session
	existingSession, err := ragService.GetSession(ctxWithTimeout, sessionID)
	if err != nil || existingSession == nil {
		// 创建新会话
		config := ragcustomerservice.SessionConfig{
			MaxHistoryLength: 10,
			Timeout:          int(rp.timeout.Seconds()),
			Temperature:      0.7,
			MaxTokens:        1000,
			SystemPrompt:     "你是一个有用的客服助手",
			EnableContextual: true,
			EnableLearning:   false,
			EnableFallback:   true,
		}

		newSession, err := ragService.CreateSession(ctxWithTimeout, req.UserID, req.Channel, "default_kb", config)
		if err != nil {
			// RAG服务失败，返回备用回复
			return &AutoReplyResponse{
				ReplyMessage:   rp.ruleMatcher.GetFallbackReply(),
				Confidence:     0.3, // 备用回复置信度较低
				Source:         "fallback",
				ProcessingTime: time.Since(startTime),
				Error:          err,
			}, nil
		}
		session = *newSession
	} else {
		session = *existingSession
	}

	// 使用RAG客服服务处理消息
	response, err := ragService.ProcessMessage(ctxWithTimeout, session, userMessage)
	if err != nil {
		// RAG服务失败，返回备用回复
		return &AutoReplyResponse{
			ReplyMessage:   rp.ruleMatcher.GetFallbackReply(),
			Confidence:     0.3, // 备用回复置信度较低
			Source:         "fallback",
			ProcessingTime: time.Since(startTime),
			Error:          err,
		}, nil
	}

	// 从RAG响应中提取信息
	confidenceScore := response.Confidence
	intentDetected := response.Intent

	// 从引用中提取实体
	var entities []string
	for _, ref := range response.References {
		entities = append(entities, ref.DocumentID)
	}

	// 返回RAG系统生成的回复
	return &AutoReplyResponse{
		ReplyMessage:   response.Content,
		Confidence:     confidenceScore,
		Source:         response.Source,
		IntentDetected: intentDetected,
		Entities:       entities,
		ProcessingTime: time.Since(startTime),
	}, nil
}
