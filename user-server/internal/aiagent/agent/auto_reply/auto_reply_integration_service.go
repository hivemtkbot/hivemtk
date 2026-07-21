package auto_reply_integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	ragcustomerservice "marketing/internal/aiagent/rag/customer_service"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
)

// AutoReplyIntegrationServiceImpl 自动回复集成服务实现
type AutoReplyIntegrationServiceImpl struct {
	ragService       ragcustomerservice.RagCustomerService
	retrievalService ragretrieval.RagRetrievalService
	replyProcessor   ReplyProcessor
	capabilities     []string
	mutex            sync.RWMutex
}

// NewAutoReplyIntegrationService 创建新的自动回复集成服务实例
func NewAutoReplyIntegrationService(
	ragService ragcustomerservice.RagCustomerService,
	retrievalService ragretrieval.RagRetrievalService,
	ruleMatcher RuleBasedMatcher,
	timeout time.Duration,
) *AutoReplyIntegrationServiceImpl {

	service := &AutoReplyIntegrationServiceImpl{
		ragService:       ragService,
		retrievalService: retrievalService,
		replyProcessor:   NewReplyProcessorImpl(ruleMatcher, timeout), // 这里使用了在同一包中定义的函数
		capabilities: []string{
			"rule_based_matching",
			"rag_enhanced_responses",
			"context_aware_processing",
			"intent_detection",
			"fallback_handling",
			"multi_channel_support",
		},
	}

	return service
}

// ProcessAutoReply 处理自动回复请求
func (s *AutoReplyIntegrationServiceImpl) ProcessAutoReply(ctx context.Context, req *AutoReplyRequest) (*AutoReplyResponse, error) {
	// 验证请求参数
	if req == nil {
		return &AutoReplyResponse{
			ReplyMessage:   "请求参数不能为空",
			Confidence:     0.0,
			Source:         "fallback",
			ProcessingTime: 0,
			Error:          fmt.Errorf("request is nil"),
		}, nil
	}

	if req.Message == "" {
		return &AutoReplyResponse{
			ReplyMessage:   "消息内容不能为空",
			Confidence:     0.0,
			Source:         "fallback",
			ProcessingTime: 0,
			Error:          fmt.Errorf("message is empty"),
		}, nil
	}

	// 使用回复处理器处理请求
	return s.replyProcessor.ProcessReply(ctx, req, s.ragService, s.retrievalService)
}

// HealthCheck 检查服务健康状态
func (s *AutoReplyIntegrationServiceImpl) HealthCheck(ctx context.Context) bool {
	// 检查依赖服务的状态
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// 尝试与RAG服务通信
	testMessage := ragcustomerservice.Message{
		ID:        "test_msg_1",
		SessionID: "health_check_session",
		Role:      ragcustomerservice.MessageRoleUser,
		Content:   "健康检查",
		Timestamp: time.Now(),
	}

	testSession := ragcustomerservice.Session{
		ID:        "health_check_session",
		UserID:    "system",
		Platform:  "system",
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config: ragcustomerservice.SessionConfig{
			MaxHistoryLength: 10,
			Timeout:          300,
			Temperature:      0.7,
			MaxTokens:        1000,
			SystemPrompt:     "你是一个有用的客服助手",
			EnableContextual: true,
			EnableLearning:   false,
			EnableFallback:   true,
		},
	}

	// 异步检查RAG服务
	ragHealthy := make(chan bool, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ragHealthy <- false
			}
		}()

		_, err := s.ragService.ProcessMessage(ctxWithTimeout, testSession, testMessage)
		ragHealthy <- err == nil
	}()

	select {
	case ragStatus := <-ragHealthy:
		return ragStatus
	case <-ctx.Done():
		return false
	}
}

// GetCapabilities 获取服务支持的功能
func (s *AutoReplyIntegrationServiceImpl) GetCapabilities() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 返回副本以防止外部修改
	caps := make([]string, len(s.capabilities))
	copy(caps, s.capabilities)
	return caps
}

// GetRagService 获取RAG服务实例
func (s *AutoReplyIntegrationServiceImpl) GetRagService() ragcustomerservice.RagCustomerService {
	return s.ragService
}

// GetRetrievalService 获取检索服务实例
func (s *AutoReplyIntegrationServiceImpl) GetRetrievalService() ragretrieval.RagRetrievalService {
	return s.retrievalService
}
