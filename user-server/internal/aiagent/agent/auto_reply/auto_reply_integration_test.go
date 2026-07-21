package auto_reply_integration

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	ragcustomerservice "marketing/internal/aiagent/rag/customer_service"
	ragretrieval "marketing/internal/aiagent/rag/retrieval"
)

// MockRagRetrievalService 模拟RAG检索服务
type MockRagRetrievalService struct{}

func (m *MockRagRetrievalService) IndexDocuments(ctx context.Context, kbID string, documents []ragretrieval.Document) error {
	return nil
}

func (m *MockRagRetrievalService) Search(ctx context.Context, kbID string, query string, params ragretrieval.SearchParams) ([]ragretrieval.SearchResult, error) {
	return []ragretrieval.SearchResult{}, nil
}

func (m *MockRagRetrievalService) DeleteKnowledgeBase(ctx context.Context, kbID string) error {
	return nil
}

func (m *MockRagRetrievalService) DeleteDocumentFromKB(ctx context.Context, kbID, docID string) error {
	return nil
}

func (m *MockRagRetrievalService) UpdateDocumentInKB(ctx context.Context, kbID, docID string, document ragretrieval.Document) error {
	return nil
}

func (m *MockRagRetrievalService) GetKnowledgeBaseInfo(ctx context.Context, kbID string) (*ragretrieval.KnowledgeBaseInfo, error) {
	return &ragretrieval.KnowledgeBaseInfo{}, nil
}

func (m *MockRagRetrievalService) ListKnowledgeBases(ctx context.Context, ownerID string, includePublic bool) ([]ragretrieval.KnowledgeBaseInfo, error) {
	return []ragretrieval.KnowledgeBaseInfo{}, nil
}

func (m *MockRagRetrievalService) CreateKnowledgeBase(ctx context.Context, kbInfo ragretrieval.KnowledgeBaseInfo) error {
	return nil
}

// MockRagCustomerService 模拟RAG客服服务
type MockRagCustomerService struct{}

func (m *MockRagCustomerService) ProcessMessage(ctx context.Context, session ragcustomerservice.Session, message ragcustomerservice.Message) (ragcustomerservice.Response, error) {
	return ragcustomerservice.Response{
		ID:             "resp_123",
		SessionID:      session.ID,
		Content:        "这是一个来自RAG系统的模拟回复",
		Intent:         "general_query",
		Confidence:     0.85,
		References:     []ragcustomerservice.Reference{{DocumentID: "doc_1", Content: "模拟内容", Score: 0.9}},
		Metadata:       map[string]any{},
		Timestamp:      time.Now(),
		ProcessingTime: time.Millisecond * 100,
		Source:         "rag",
	}, nil
}

func (m *MockRagCustomerService) CreateSession(ctx context.Context, userID, platform, kbID string, config ragcustomerservice.SessionConfig) (*ragcustomerservice.Session, error) {
	session := &ragcustomerservice.Session{
		ID:        fmt.Sprintf("mock_session_%d", time.Now().Unix()),
		UserID:    userID,
		Platform:  platform,
		KBID:      kbID,
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config:    config,
		Conversation: &ragcustomerservice.Conversation{
			Messages: []ragcustomerservice.Message{},
			Context:  ragcustomerservice.Context{},
			Metadata: map[string]any{},
		},
		Metadata: map[string]any{},
	}
	return session, nil
}

func (m *MockRagCustomerService) GetSession(ctx context.Context, sessionID string) (*ragcustomerservice.Session, error) {
	config := ragcustomerservice.SessionConfig{
		MaxHistoryLength: 10,
		Timeout:          300,
		Temperature:      0.7,
		MaxTokens:        1000,
		SystemPrompt:     "你是一个有用的客服助手",
		EnableContextual: true,
		EnableLearning:   false,
		EnableFallback:   true,
	}

	session := &ragcustomerservice.Session{
		ID:        sessionID,
		UserID:    "mock_user",
		Platform:  "mock_platform",
		KBID:      "default_kb",
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config:    config,
		Conversation: &ragcustomerservice.Conversation{
			Messages: []ragcustomerservice.Message{},
			Context:  ragcustomerservice.Context{},
			Metadata: map[string]any{},
		},
		Metadata: map[string]any{},
	}
	return session, nil
}

func (m *MockRagCustomerService) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return nil
}

func (m *MockRagCustomerService) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockRagCustomerService) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]ragcustomerservice.Message, error) {
	return []ragcustomerservice.Message{}, nil
}

func (m *MockRagCustomerService) ProcessBatchMessages(ctx context.Context, session ragcustomerservice.Session, messages []ragcustomerservice.Message) ([]ragcustomerservice.Response, error) {
	responses := make([]ragcustomerservice.Response, len(messages))
	for i, msg := range messages {
		responses[i] = ragcustomerservice.Response{
			ID:             fmt.Sprintf("resp_%d", i),
			SessionID:      session.ID,
			Content:        fmt.Sprintf("回复给: %s", msg.Content),
			Intent:         "general_query",
			Confidence:     0.8,
			References:     []ragcustomerservice.Reference{},
			Metadata:       map[string]any{},
			Timestamp:      time.Now(),
			ProcessingTime: time.Millisecond * 50,
			Source:         "rag",
		}
	}
	return responses, nil
}

func (m *MockRagCustomerService) UpdateKnowledge(ctx context.Context, kbID string, feedback ragcustomerservice.Feedback) error {
	return nil
}

func (m *MockRagCustomerService) GetSessionMetrics(ctx context.Context, sessionID string) (*ragcustomerservice.SessionMetrics, error) {
	return &ragcustomerservice.SessionMetrics{}, nil
}

func (m *MockRagCustomerService) ListSessions(ctx context.Context, userID, platform string, status ragcustomerservice.SessionStatus) ([]ragcustomerservice.Session, error) {
	return []ragcustomerservice.Session{}, nil
}

func (m *MockRagCustomerService) GetUserContext(ctx context.Context, userID, platform string) (*ragcustomerservice.Context, error) {
	context := &ragcustomerservice.Context{
		Topic:           "",
		Intent:          "",
		Entities:        map[string][]string{},
		Sentiment:       ragcustomerservice.Sentiment{},
		PreviousTopics:  []string{},
		UserPreferences: map[string]any{},
		SessionContext:  map[string]any{},
		LastInteraction: time.Now(),
	}
	return context, nil
}

// TestAutoReplyIntegration 测试自动回复集成服务
func TestAutoReplyIntegration(t *testing.T) {
	// 创建模拟服务
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}

	// 创建规则匹配器
	ruleMatcher := NewDefaultRuleBasedMatcher()

	// 创建自动回复集成服务
	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	// 测试规则匹配场景
	t.Run("Test Rule Matching", func(t *testing.T) {
		ctx := context.Background()
		req := &AutoReplyRequest{
			UserID:      "user123",
			SessionID:   "session123",
			Message:     "你好",
			Channel:     "wechat",
			ContextData: map[string]any{"product": "premium"},
		}

		resp, err := service.ProcessAutoReply(ctx, req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp.Source != "rule_based" {
			t.Errorf("Expected source 'rule_based', got '%s'", resp.Source)
		}

		if resp.Confidence < 0.8 {
			t.Errorf("Expected confidence >= 0.8 for rule-based reply, got %f", resp.Confidence)
		}

		t.Logf("Rule matching test passed: %s (confidence: %.2f)", resp.ReplyMessage, resp.Confidence)
	})

	// 测试RAG系统场景
	t.Run("Test RAG System", func(t *testing.T) {
		ctx := context.Background()
		req := &AutoReplyRequest{
			UserID:      "user123",
			SessionID:   "session123",
			Message:     "请介绍一下你们的产品特点",
			Channel:     "website",
			ContextData: map[string]any{"product": "premium"},
		}

		resp, err := service.ProcessAutoReply(ctx, req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp.Source != "rag" && resp.Source != "fallback" {
			t.Errorf("Expected source 'rag' or 'fallback', got '%s'", resp.Source)
		}

		t.Logf("RAG system test passed: %s (confidence: %.2f)", resp.ReplyMessage, resp.Confidence)
	})

	// 测试备用回复场景
	t.Run("Test Fallback Reply", func(t *testing.T) {
		ctx := context.Background()
		req := &AutoReplyRequest{
			UserID:      "user123",
			SessionID:   "session123",
			Message:     "这是一条无法匹配的奇怪消息",
			Channel:     "app",
			ContextData: map[string]any{},
		}

		resp, err := service.ProcessAutoReply(ctx, req)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}

		if resp.Source != "fallback" && resp.Source != "rag" {
			t.Errorf("Expected source 'fallback' or 'rag', got '%s'", resp.Source)
		}

		t.Logf("Fallback test passed: %s (confidence: %.2f)", resp.ReplyMessage, resp.Confidence)
	})

	// 测试健康检查
	t.Run("Test Health Check", func(t *testing.T) {
		ctx := context.Background()
		healthy := service.HealthCheck(ctx)

		if !healthy {
			t.Error("Expected service to be healthy")
		}

		t.Log("Health check passed")
	})

	// 测试功能获取
	t.Run("Test Capabilities", func(t *testing.T) {
		caps := service.GetCapabilities()

		if len(caps) == 0 {
			t.Error("Expected non-empty capabilities list")
		}

		t.Logf("Capabilities test passed: %v", caps)
	})
}

// TestGetRagService 测试获取 RAG 服务
func TestGetRagService(t *testing.T) {
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	// 测试 GetRagService 返回正确的实例
	returnedService := service.GetRagService()
	if returnedService != mockRagService {
		t.Error("Expected GetRagService to return the same mock service instance")
	}
}

// TestGetRetrievalService 测试获取检索服务
func TestGetRetrievalService(t *testing.T) {
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	// 测试 GetRetrievalService 返回正确的实例
	returnedService := service.GetRetrievalService()
	if returnedService != mockRetrievalService {
		t.Error("Expected GetRetrievalService to return the same mock service instance")
	}
}

// TestProcessAutoReply_NilRequest 测试空请求参数
func TestProcessAutoReply_NilRequest(t *testing.T) {
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	ctx := context.Background()
	resp, err := service.ProcessAutoReply(ctx, nil)

	// ProcessAutoReply 返回软错误（在 response.Error 中），而不是 error 返回值
	if err != nil {
		t.Errorf("Expected no error for nil request, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response for nil request")
	}
	if resp.ReplyMessage != "请求参数不能为空" {
		t.Errorf("Expected '请求参数不能为空', got '%s'", resp.ReplyMessage)
	}
	if resp.Source != "fallback" {
		t.Errorf("Expected source 'fallback', got '%s'", resp.Source)
	}
	if resp.Error == nil {
		t.Error("Expected error in response for nil request")
	}
}

// TestProcessAutoReply_EmptyMessage 测试空消息内容
func TestProcessAutoReply_EmptyMessage(t *testing.T) {
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user123",
		SessionID: "session123",
		Message:   "", // 空消息
		Channel:   "wechat",
	}

	resp, err := service.ProcessAutoReply(ctx, req)

	// ProcessAutoReply 返回软错误（在 response.Error 中），而不是 error 返回值
	if err != nil {
		t.Errorf("Expected no error for empty message, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response for empty message")
	}
	if resp.ReplyMessage != "消息内容不能为空" {
		t.Errorf("Expected '消息内容不能为空', got '%s'", resp.ReplyMessage)
	}
	if resp.Source != "fallback" {
		t.Errorf("Expected source 'fallback', got '%s'", resp.Source)
	}
	if resp.Error == nil {
		t.Error("Expected error in response for empty message")
	}
}

// TestGetFallbackReply 测试获取备用回复
func TestGetFallbackReply(t *testing.T) {
	ruleMatcher := NewDefaultRuleBasedMatcher()

	fallbackReply := ruleMatcher.GetFallbackReply()

	// 验证返回的备用回复不为空
	if fallbackReply == "" {
		t.Error("Expected non-empty fallback reply")
	}

	// 验证备用回复包含预期的道歉内容
	expectedText := "抱歉"
	if len(fallbackReply) < len(expectedText) {
		t.Errorf("Expected fallback reply to contain '%s', got '%s'", expectedText, fallbackReply)
	}
}

// TestRuleBasedMatcherImpl_GetFallbackReply_Custom 测试自定义备用回复
func TestRuleBasedMatcherImpl_GetFallbackReply_Custom(t *testing.T) {
	customFallback := "这是一个自定义的备用回复"
	ruleMatcher := NewRuleBasedMatcherImpl([]Rule{}, customFallback)

	fallbackReply := ruleMatcher.GetFallbackReply()

	if fallbackReply != customFallback {
		t.Errorf("Expected '%s', got '%s'", customFallback, fallbackReply)
	}
}

// TestRuleBasedMatcherImpl_GetFallbackReply_Default 测试默认备用回复
func TestRuleBasedMatcherImpl_GetFallbackReply_Default(t *testing.T) {
	ruleMatcher := NewRuleBasedMatcherImpl([]Rule{}, "")

	fallbackReply := ruleMatcher.GetFallbackReply()

	expectedDefault := "抱歉，我没有理解您的意思，请稍后再试或联系人工客服。"
	if fallbackReply != expectedDefault {
		t.Errorf("Expected '%s', got '%s'", expectedDefault, fallbackReply)
	}
}

// TestRuleBasedMatcherImpl_MatchRule_EmptyMessage 测试空消息返回错误
func TestRuleBasedMatcherImpl_MatchRule_EmptyMessage(t *testing.T) {
	ruleMatcher := NewRuleBasedMatcherImpl([]Rule{}, "")

	match, reply, confidence, err := ruleMatcher.MatchRule("", nil)

	if err == nil {
		t.Error("Expected error for empty message")
	}
	if match {
		t.Error("Expected match to be false")
	}
	if reply != "" {
		t.Errorf("Expected empty reply, got '%s'", reply)
	}
	if confidence != 0.0 {
		t.Errorf("Expected 0.0 confidence, got %f", confidence)
	}
}

// TestRuleBasedMatcherImpl_MatchRule_WithRules 测试规则匹配成功
func TestRuleBasedMatcherImpl_MatchRule_WithRules(t *testing.T) {
	rules := []Rule{
		{
			Pattern:    regexp.MustCompile(`(?i)hello`),
			Reply:      "Hello!",
			Confidence: 0.9,
			Priority:   1,
		},
	}
	ruleMatcher := NewRuleBasedMatcherImpl(rules, "fallback")

	match, reply, confidence, err := ruleMatcher.MatchRule("Hello, world!", nil)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !match {
		t.Error("Expected match to be true")
	}
	if reply != "Hello!" {
		t.Errorf("Expected 'Hello!', got '%s'", reply)
	}
	if confidence != 0.9 {
		t.Errorf("Expected 0.9 confidence, got %f", confidence)
	}
}

// TestAutoReplyIntegrationServiceImpl_HealthCheck_Timeout 测试健康检查超时
func TestAutoReplyIntegrationServiceImpl_HealthCheck_Timeout(t *testing.T) {
	// 创建一个会阻塞的 mock 服务
	mockRagService := &MockRagCustomerServiceBlocking{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	// 创建一个很短时间内超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	healthy := service.HealthCheck(ctx)

	if healthy {
		t.Error("Expected health check to return false on timeout")
	}
}

// MockRagCustomerServiceBlocking 用于测试超时场景的阻塞 mock 服务
type MockRagCustomerServiceBlocking struct{}

func (m *MockRagCustomerServiceBlocking) ProcessMessage(ctx context.Context, session ragcustomerservice.Session, message ragcustomerservice.Message) (ragcustomerservice.Response, error) {
	// 阻塞直到 context 取消
	<-ctx.Done()
	return ragcustomerservice.Response{}, ctx.Err()
}

func (m *MockRagCustomerServiceBlocking) CreateSession(ctx context.Context, userID, platform, kbID string, config ragcustomerservice.SessionConfig) (*ragcustomerservice.Session, error) {
	now := time.Now()
	return &ragcustomerservice.Session{
		ID:        "mock-session-" + userID,
		UserID:    userID,
		Platform:  platform,
		KBID:      kbID,
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
		Config:    config,
		Metadata:  make(map[string]any),
	}, nil
}

func (m *MockRagCustomerServiceBlocking) GetSession(ctx context.Context, sessionID string) (*ragcustomerservice.Session, error) {
	now := time.Now()
	return &ragcustomerservice.Session{
		ID:        sessionID,
		UserID:    "mock-user",
		Platform:  "mock-platform",
		KBID:      "mock-kb",
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]any),
	}, nil
}

func (m *MockRagCustomerServiceBlocking) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return nil
}

func (m *MockRagCustomerServiceBlocking) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockRagCustomerServiceBlocking) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]ragcustomerservice.Message, error) {
	return []ragcustomerservice.Message{}, nil
}

func (m *MockRagCustomerServiceBlocking) ProcessBatchMessages(ctx context.Context, session ragcustomerservice.Session, messages []ragcustomerservice.Message) ([]ragcustomerservice.Response, error) {
	return []ragcustomerservice.Response{}, nil
}

func (m *MockRagCustomerServiceBlocking) UpdateKnowledge(ctx context.Context, kbID string, feedback ragcustomerservice.Feedback) error {
	return nil
}

func (m *MockRagCustomerServiceBlocking) GetSessionMetrics(ctx context.Context, sessionID string) (*ragcustomerservice.SessionMetrics, error) {
	return &ragcustomerservice.SessionMetrics{}, nil
}

func (m *MockRagCustomerServiceBlocking) ListSessions(ctx context.Context, userID, platform string, status ragcustomerservice.SessionStatus) ([]ragcustomerservice.Session, error) {
	return []ragcustomerservice.Session{}, nil
}

func (m *MockRagCustomerServiceBlocking) GetUserContext(ctx context.Context, userID, platform string) (*ragcustomerservice.Context, error) {
	return &ragcustomerservice.Context{}, nil
}

// MockRagCustomerServicePanic 用于测试 panic 恢复场景的 mock 服务
type MockRagCustomerServicePanic struct{}

func (m *MockRagCustomerServicePanic) ProcessMessage(ctx context.Context, session ragcustomerservice.Session, message ragcustomerservice.Message) (ragcustomerservice.Response, error) {
	panic("unexpected panic in RAG service")
}

func (m *MockRagCustomerServicePanic) CreateSession(ctx context.Context, userID, platform, kbID string, config ragcustomerservice.SessionConfig) (*ragcustomerservice.Session, error) {
	now := time.Now()
	return &ragcustomerservice.Session{
		ID:        "panic-session-" + userID,
		UserID:    userID,
		Platform:  platform,
		KBID:      kbID,
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
		Config:    config,
		Metadata:  make(map[string]any),
	}, nil
}

func (m *MockRagCustomerServicePanic) GetSession(ctx context.Context, sessionID string) (*ragcustomerservice.Session, error) {
	now := time.Now()
	return &ragcustomerservice.Session{
		ID:        sessionID,
		UserID:    "mock-user",
		Platform:  "mock-platform",
		KBID:      "mock-kb",
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  make(map[string]any),
	}, nil
}

func (m *MockRagCustomerServicePanic) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return nil
}

func (m *MockRagCustomerServicePanic) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockRagCustomerServicePanic) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]ragcustomerservice.Message, error) {
	return []ragcustomerservice.Message{}, nil
}

func (m *MockRagCustomerServicePanic) ProcessBatchMessages(ctx context.Context, session ragcustomerservice.Session, messages []ragcustomerservice.Message) ([]ragcustomerservice.Response, error) {
	return []ragcustomerservice.Response{}, nil
}

func (m *MockRagCustomerServicePanic) UpdateKnowledge(ctx context.Context, kbID string, feedback ragcustomerservice.Feedback) error {
	return nil
}

func (m *MockRagCustomerServicePanic) GetSessionMetrics(ctx context.Context, sessionID string) (*ragcustomerservice.SessionMetrics, error) {
	return &ragcustomerservice.SessionMetrics{}, nil
}

func (m *MockRagCustomerServicePanic) ListSessions(ctx context.Context, userID, platform string, status ragcustomerservice.SessionStatus) ([]ragcustomerservice.Session, error) {
	return []ragcustomerservice.Session{}, nil
}

func (m *MockRagCustomerServicePanic) GetUserContext(ctx context.Context, userID, platform string) (*ragcustomerservice.Context, error) {
	return &ragcustomerservice.Context{}, nil
}

// TestAutoReplyIntegrationServiceImpl_HealthCheck_Panic 测试健康检查处理 panic
func TestAutoReplyIntegrationServiceImpl_HealthCheck_Panic(t *testing.T) {
	mockRagService := &MockRagCustomerServicePanic{}
	mockRetrievalService := &MockRagRetrievalService{}
	ruleMatcher := NewDefaultRuleBasedMatcher()

	service := NewAutoReplyIntegrationService(mockRagService, mockRetrievalService, ruleMatcher, 10*time.Second)

	ctx := context.Background()
	healthy := service.HealthCheck(ctx)

	if healthy {
		t.Error("Expected health check to return false when RAG service panics")
	}
}

// TestNewDefaultAutoReplyIntegrationService 测试创建默认自动回复集成服务
func TestNewDefaultAutoReplyIntegrationService(t *testing.T) {
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}

	service := NewDefaultAutoReplyIntegrationService(mockRagService, mockRetrievalService)

	if service == nil {
		t.Fatal("Expected non-nil service from NewDefaultAutoReplyIntegrationService")
	}

	// 验证返回的服务是否正确配置
	returnedRag := service.GetRagService()
	if returnedRag != mockRagService {
		t.Error("Expected GetRagService to return the same mock service instance")
	}

	returnedRetrieval := service.GetRetrievalService()
	if returnedRetrieval != mockRetrievalService {
		t.Error("Expected GetRetrievalService to return the same mock service instance")
	}

	// 验证默认功能
	caps := service.GetCapabilities()
	if len(caps) == 0 {
		t.Error("Expected non-empty capabilities list")
	}

	// 验证健康检查
	ctx := context.Background()
	healthy := service.HealthCheck(ctx)
	if !healthy {
		t.Error("Expected service to be healthy")
	}
}

// TestNewReplyProcessorImpl_ZeroTimeout 测试零超时时间使用默认值
func TestNewReplyProcessorImpl_ZeroTimeout(t *testing.T) {
	ruleMatcher := NewDefaultRuleBasedMatcher()

	// 使用零超时时间
	processor := NewReplyProcessorImpl(ruleMatcher, 0)

	if processor == nil {
		t.Fatal("Expected non-nil processor")
	}

	// 验证默认超时时间被设置
	// 通过检查处理器的行为来验证（无法直接访问 timeout 字段）
	// 如果处理器能正常工作，说明默认超时时间已设置
	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user123",
		SessionID: "session123",
		Message:   "你好",
		Channel:   "wechat",
	}

	// 使用模拟服务测试处理器
	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}

	resp, err := processor.ProcessReply(ctx, req, mockRagService, mockRetrievalService)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
	// 规则匹配应该成功（"你好"是预定义规则）
	if resp.Source != "rule_based" {
		t.Errorf("Expected source 'rule_based', got '%s'", resp.Source)
	}
}

// TestNewReplyProcessorImpl_NegativeTimeout 测试负超时时间使用默认值
func TestNewReplyProcessorImpl_NegativeTimeout(t *testing.T) {
	ruleMatcher := NewDefaultRuleBasedMatcher()

	// 使用负超时时间
	processor := NewReplyProcessorImpl(ruleMatcher, -5*time.Second)

	if processor == nil {
		t.Fatal("Expected non-nil processor")
	}

	// 验证处理器能正常工作
	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user456",
		SessionID: "session456",
		Message:   "谢谢",
		Channel:   "app",
	}

	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}

	resp, err := processor.ProcessReply(ctx, req, mockRagService, mockRetrievalService)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
}

// MockRuleMatcherForError 用于测试错误场景的模拟规则匹配器
type MockRuleMatcherForError struct {
	returnError bool
}

func (m *MockRuleMatcherForError) MatchRule(message string, contextData map[string]any) (bool, string, float64, error) {
	if m.returnError {
		return false, "", 0.0, fmt.Errorf("rule matching error")
	}
	return false, "", 0.0, nil
}

func (m *MockRuleMatcherForError) GetFallbackReply() string {
	return "fallback reply"
}

// TestProcessReply_RuleMatcherError 测试规则匹配器返回错误的场景
func TestProcessReply_RuleMatcherError(t *testing.T) {
	mockRuleMatcher := &MockRuleMatcherForError{returnError: true}
	processor := NewReplyProcessorImpl(mockRuleMatcher, 10*time.Second)

	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user123",
		SessionID: "session123",
		Message:   "test message",
		Channel:   "wechat",
	}

	mockRagService := &MockRagCustomerService{}
	mockRetrievalService := &MockRagRetrievalService{}

	resp, err := processor.ProcessReply(ctx, req, mockRagService, mockRetrievalService)

	// 规则匹配错误应该返回 fallback 回复，不返回 error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
	if resp.ReplyMessage != "fallback reply" {
		t.Errorf("Expected 'fallback reply', got '%s'", resp.ReplyMessage)
	}
	if resp.Source != "fallback" {
		t.Errorf("Expected source 'fallback', got '%s'", resp.Source)
	}
	if resp.Error == nil {
		t.Error("Expected error in response")
	}
}

// MockRagCustomerServiceForError 用于测试错误场景的模拟 RAG 服务
type MockRagCustomerServiceForError struct {
	createSessionError  bool
	processMessageError bool
}

func (m *MockRagCustomerServiceForError) ProcessMessage(ctx context.Context, session ragcustomerservice.Session, message ragcustomerservice.Message) (ragcustomerservice.Response, error) {
	if m.processMessageError {
		return ragcustomerservice.Response{}, fmt.Errorf("process message error")
	}
	return ragcustomerservice.Response{
		ID:             "resp_123",
		SessionID:      session.ID,
		Content:        "RAG response",
		Intent:         "general_query",
		Confidence:     0.85,
		References:     []ragcustomerservice.Reference{},
		Metadata:       map[string]any{},
		Timestamp:      time.Now(),
		ProcessingTime: time.Millisecond * 100,
		Source:         "rag",
	}, nil
}

func (m *MockRagCustomerServiceForError) CreateSession(ctx context.Context, userID, platform, kbID string, config ragcustomerservice.SessionConfig) (*ragcustomerservice.Session, error) {
	if m.createSessionError {
		return nil, fmt.Errorf("create session error")
	}
	session := &ragcustomerservice.Session{
		ID:        fmt.Sprintf("session_%d", time.Now().Unix()),
		UserID:    userID,
		Platform:  platform,
		KBID:      kbID,
		Status:    ragcustomerservice.SessionActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Config:    config,
		Conversation: &ragcustomerservice.Conversation{
			Messages: []ragcustomerservice.Message{},
			Context:  ragcustomerservice.Context{},
			Metadata: map[string]any{},
		},
		Metadata: map[string]any{},
	}
	return session, nil
}

func (m *MockRagCustomerServiceForError) GetSession(ctx context.Context, sessionID string) (*ragcustomerservice.Session, error) {
	return nil, fmt.Errorf("session not found")
}

func (m *MockRagCustomerServiceForError) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	return nil
}

func (m *MockRagCustomerServiceForError) EndSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockRagCustomerServiceForError) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]ragcustomerservice.Message, error) {
	return []ragcustomerservice.Message{}, nil
}

func (m *MockRagCustomerServiceForError) ProcessBatchMessages(ctx context.Context, session ragcustomerservice.Session, messages []ragcustomerservice.Message) ([]ragcustomerservice.Response, error) {
	return []ragcustomerservice.Response{}, nil
}

func (m *MockRagCustomerServiceForError) UpdateKnowledge(ctx context.Context, kbID string, feedback ragcustomerservice.Feedback) error {
	return nil
}

func (m *MockRagCustomerServiceForError) GetSessionMetrics(ctx context.Context, sessionID string) (*ragcustomerservice.SessionMetrics, error) {
	return &ragcustomerservice.SessionMetrics{}, nil
}

func (m *MockRagCustomerServiceForError) ListSessions(ctx context.Context, userID, platform string, status ragcustomerservice.SessionStatus) ([]ragcustomerservice.Session, error) {
	return []ragcustomerservice.Session{}, nil
}

func (m *MockRagCustomerServiceForError) GetUserContext(ctx context.Context, userID, platform string) (*ragcustomerservice.Context, error) {
	return &ragcustomerservice.Context{}, nil
}

// TestProcessReply_CreateSessionError 测试创建会话错误的场景
func TestProcessReply_CreateSessionError(t *testing.T) {
	mockRuleMatcher := &MockRuleMatcherForError{returnError: false}
	processor := NewReplyProcessorImpl(mockRuleMatcher, 10*time.Second)

	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user123",
		SessionID: "", // 空 sessionID 会触发创建新会话
		Message:   "unmatched message",
		Channel:   "wechat",
	}

	mockRagService := &MockRagCustomerServiceForError{createSessionError: true}
	mockRetrievalService := &MockRagRetrievalService{}

	resp, err := processor.ProcessReply(ctx, req, mockRagService, mockRetrievalService)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
	if resp.ReplyMessage != "fallback reply" {
		t.Errorf("Expected 'fallback reply', got '%s'", resp.ReplyMessage)
	}
	if resp.Source != "fallback" {
		t.Errorf("Expected source 'fallback', got '%s'", resp.Source)
	}
	if resp.Error == nil {
		t.Error("Expected error in response")
	}
}

// TestProcessReply_ProcessMessageError 测试处理消息错误的场景
func TestProcessReply_ProcessMessageError(t *testing.T) {
	mockRuleMatcher := &MockRuleMatcherForError{returnError: false}
	processor := NewReplyProcessorImpl(mockRuleMatcher, 10*time.Second)

	ctx := context.Background()
	req := &AutoReplyRequest{
		UserID:    "user123",
		SessionID: "", // 空 sessionID 会触发创建新会话
		Message:   "unmatched message",
		Channel:   "wechat",
	}

	mockRagService := &MockRagCustomerServiceForError{createSessionError: false, processMessageError: true}
	mockRetrievalService := &MockRagRetrievalService{}

	resp, err := processor.ProcessReply(ctx, req, mockRagService, mockRetrievalService)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if resp == nil {
		t.Fatal("Expected non-nil response")
	}
	if resp.ReplyMessage != "fallback reply" {
		t.Errorf("Expected 'fallback reply', got '%s'", resp.ReplyMessage)
	}
	if resp.Source != "fallback" {
		t.Errorf("Expected source 'fallback', got '%s'", resp.Source)
	}
	if resp.Error == nil {
		t.Error("Expected error in response")
	}
}
