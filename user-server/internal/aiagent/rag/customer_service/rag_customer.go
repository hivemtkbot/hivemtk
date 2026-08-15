package ragcustomerservice

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"hivemtk-user/internal/aiagent/llm"
	ragretrieval "hivemtk-user/internal/aiagent/rag/retrieval"
	"hivemtk-user/internal/pkg/utils/logger"
)

// RagCustomerServiceImpl RAG客服服务实现
type RagCustomerServiceImpl struct {
	dialogManager        DialogManagerInterface
	contextUnderstanding ContextUnderstandingInterface
	responseGenerator    ResponseGeneratorInterface
	qualityAssessor      QualityAssessmentInterface
	feedbackLearner      FeedbackLearningInterface
	retrievalService     ragretrieval.RagRetrievalService 
	config               *CustomerServiceConfig
}

// CustomerServiceConfig 客服服务配置
type CustomerServiceConfig struct {
	MaxHistoryLength        int           `json:"max_history_length"`         
	DefaultMaxHistoryLength int           `json:"default_max_history_length"` 
	DefaultTimeout          int           `json:"default_timeout"`            
	DefaultTemperature      float64       `json:"default_temperature"`        
	DefaultMaxTokens        int           `json:"default_max_tokens"`         
	RetrievalTopK           int           `json:"retrieval_top_k"`            
	RetrievalThreshold      float64       `json:"retrieval_threshold"`        
	CacheTTL                time.Duration `json:"cache_ttl"`                  
	MaxConcurrentSessions   int           `json:"max_concurrent_sessions"`    
	SessionCleanupInterval  time.Duration `json:"session_cleanup_interval"`   
	EnableFallback          bool          `json:"enable_fallback"`            
	FallbackResponse        string        `json:"fallback_response"`          
}

// NewRagCustomerService 创建新的RAG客服服务
func NewRagCustomerService(
	dialogManager DialogManagerInterface,
	contextUnderstanding ContextUnderstandingInterface,
	responseGenerator ResponseGeneratorInterface,
	qualityAssessor QualityAssessmentInterface,
	feedbackLearner FeedbackLearningInterface,
	retrievalService ragretrieval.RagRetrievalService,
	config *CustomerServiceConfig,
) *RagCustomerServiceImpl {
	if config == nil {
		config = &CustomerServiceConfig{
			DefaultMaxHistoryLength: 10,
			DefaultTimeout:          30 * 60, 
			DefaultTemperature:      0.7,
			DefaultMaxTokens:        1000,
			RetrievalTopK:           5,
			RetrievalThreshold:      0.5,
			CacheTTL:                30 * time.Minute,
			MaxConcurrentSessions:   100,
			SessionCleanupInterval:  5 * time.Minute,
			EnableFallback:          true,
			FallbackResponse:        "感谢您的消息，我们会尽快回复您！",
		}
	}

	return &RagCustomerServiceImpl{
		dialogManager:        dialogManager,
		contextUnderstanding: contextUnderstanding,
		responseGenerator:    responseGenerator,
		qualityAssessor:      qualityAssessor,
		feedbackLearner:      feedbackLearner,
		retrievalService:     retrievalService,
		config:               config,
	}
}

// ProcessMessage 处理用户消息
func (r *RagCustomerServiceImpl) ProcessMessage(ctx context.Context, session Session, message Message) (Response, error) {
	startTime := time.Now()

	err := r.dialogManager.AddMessage(ctx, session.ID, message)
	if err != nil {
		return Response{}, fmt.Errorf("failed to add message to session: %w", err)
	}

	conversation, err := r.dialogManager.GetConversationHistory(ctx, session.ID, session.Config.MaxHistoryLength)
	if err != nil {
		return Response{}, fmt.Errorf("failed to get conversation history: %w", err)
	}

	intent, err := r.contextUnderstanding.AnalyzeIntent(ctx, message.Content, conversation.Messages)
	if err != nil {
		return Response{}, fmt.Errorf("failed to analyze intent: %w", err)
	}

	entities, err := r.contextUnderstanding.ExtractEntities(ctx, message.Content)
	if err != nil {
		return Response{}, fmt.Errorf("failed to extract entities: %w", err)
	}

	sentiment, err := r.contextUnderstanding.AnalyzeSentiment(ctx, message.Content)
	if err != nil {
		return Response{}, fmt.Errorf("failed to analyze sentiment: %w", err)
	}

	updatedContext, err := r.contextUnderstanding.UpdateContext(ctx, conversation.Context, message, intent)
	if err != nil {
		return Response{}, fmt.Errorf("failed to update context: %w", err)
	}

	metadata := map[string]any{
		"last_intent":      intent.PrimaryIntent,
		"last_sentiment":   sentiment.Label,
		"last_entities":    entities,
		"updated_at":       time.Now(),
		"last_topic":       updatedContext.Topic,
		"last_interaction": time.Now(),
	}
	err = r.dialogManager.UpdateSessionMetadata(ctx, session.ID, metadata)
	if err != nil {
		return Response{}, fmt.Errorf("failed to update session metadata: %w", err)
	}

	searchParams := ragretrieval.SearchParams{
		TopK:                r.config.RetrievalTopK,
		SimilarityThreshold: r.config.RetrievalThreshold,
	}

	searchResults, err := r.retrievalService.Search(ctx, session.KBID, message.Content, searchParams)
	if err != nil {
		if r.config.EnableFallback {
			logger.Warnf("Warning: RAG search failed: %v, using fallback response", err)

			response := Response{
				ID:             generateResponseID(),
				SessionID:      session.ID,
				Content:        r.config.FallbackResponse,
				Intent:         intent.PrimaryIntent,
				Confidence:     0.1, 
				Metadata:       map[string]any{},
				Timestamp:      time.Now(),
				ProcessingTime: time.Since(startTime),
				Source:         "fallback",
			}

			response.Metadata["quality_score"] = 0.1
			response.Metadata["processing_time_ms"] = response.ProcessingTime.Milliseconds()
			response.Metadata["retrieval_error"] = err.Error()

			assistantMessage := Message{
				ID:        generateMessageID(),
				SessionID: session.ID,
				Role:      MessageRoleAssistant,
				Content:   response.Content,
				Timestamp: time.Now(),
				Metadata:  response.Metadata,
			}

			err = r.dialogManager.AddMessage(ctx, session.ID, assistantMessage)
			if err != nil {
				logger.Warnf("Warning: failed to add assistant message to session: %v", err)
			}

			return response, nil
		} else {
			return Response{}, fmt.Errorf("failed to search knowledge base: %w", err)
		}
	}

	generationRequest := ResponseGenerationRequest{
		Query:     message.Content,
		Context:   updatedContext,
		Session:   session,
		Config:    session.Config,
		LLMConfig: session.Config, 
	}

	// 将searchResults转换为interface{}切片
	var interfaceResults []any
	for _, result := range searchResults {
		interfaceResult := map[string]any{
			"document_id": result.DocumentID,
			"content":     result.Content,
			"title":       result.Title,
			"score":       result.Score,
			"metadata":    result.Metadata,
			"confidence":  result.Confidence,
		}
		interfaceResults = append(interfaceResults, interfaceResult)
	}
	generationRequest.SearchResults = interfaceResults

	responseContent, err := r.responseGenerator.GenerateResponse(ctx, generationRequest)
	if err != nil {
		return Response{}, fmt.Errorf("failed to generate response: %w", err)
	}

	qualityScore, err := r.qualityAssessor.EvaluateResponse(ctx, responseContent, message.Content, interfaceResults)
	if err != nil {
		logger.Warnf("Warning: failed to evaluate response quality: %v", err)
		qualityScore = 0.5 
	}

	response := Response{
		ID:             generateResponseID(),
		SessionID:      session.ID,
		Content:        responseContent,
		Intent:         intent.PrimaryIntent,
		Confidence:     qualityScore,
		Metadata:       map[string]any{},
		Timestamp:      time.Now(),
		ProcessingTime: time.Since(startTime),
		Source:         "rag", 
	}

	for _, result := range searchResults {
		ref := Reference{
			DocumentID: result.DocumentID,
			Content:    result.Content,
			Score:      result.Score,
			Source:     "knowledge_base",
			ChunkIndex: result.ChunkIndex,
		}
		response.References = append(response.References, ref)
	}

	response.Metadata["quality_score"] = qualityScore
	response.Metadata["processing_time_ms"] = response.ProcessingTime.Milliseconds()
	response.Metadata["retrieved_documents"] = len(searchResults)
	response.Metadata["intent_confidence"] = intent.Confidence
	response.Metadata["sentiment_score"] = sentiment.Score
	response.Metadata["sentiment_label"] = sentiment.Label

	assistantMessage := Message{
		ID:         generateMessageID(),
		SessionID:  session.ID,
		Role:       MessageRoleAssistant,
		Content:    responseContent,
		Timestamp:  time.Now(),
		Metadata:   response.Metadata,
		References: []string{}, 
	}

	err = r.dialogManager.AddMessage(ctx, session.ID, assistantMessage)
	if err != nil {
		logger.Warnf("Warning: failed to add assistant message to session: %v", err)
	}

	return response, nil
}

// CreateSession 创建新会话
func (r *RagCustomerServiceImpl) CreateSession(ctx context.Context, userID, platform, kbID string, config SessionConfig) (*Session, error) {
	if config.MaxHistoryLength == 0 {
		config.MaxHistoryLength = r.config.DefaultMaxHistoryLength
	}
	if config.Timeout == 0 {
		config.Timeout = r.config.DefaultTimeout
	}
	if config.Temperature == 0 {
		config.Temperature = r.config.DefaultTemperature
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = r.config.DefaultMaxTokens
	}

	session, err := r.dialogManager.CreateSession(ctx, userID, platform, kbID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession 获取会话信息
func (r *RagCustomerServiceImpl) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, err := r.dialogManager.GetSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// UpdateSession 更新会话信息
func (r *RagCustomerServiceImpl) UpdateSession(ctx context.Context, sessionID string, updates map[string]any) error {
	err := r.dialogManager.UpdateSessionMetadata(ctx, sessionID, updates)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// EndSession 结束会话
func (r *RagCustomerServiceImpl) EndSession(ctx context.Context, sessionID string) error {
	err := r.dialogManager.CloseSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to end session: %w", err)
	}

	return nil
}

// GetSessionHistory 获取会话历史
func (r *RagCustomerServiceImpl) GetSessionHistory(ctx context.Context, sessionID string, limit int) ([]Message, error) {
	conversation, err := r.dialogManager.GetConversationHistory(ctx, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get session history: %w", err)
	}

	return conversation.Messages, nil
}

// ProcessBatchMessages 批量处理消息
func (r *RagCustomerServiceImpl) ProcessBatchMessages(ctx context.Context, session Session, messages []Message) ([]Response, error) {
	responses := make([]Response, len(messages))

	for i, message := range messages {
		response, err := r.ProcessMessage(ctx, session, message)
		if err != nil {
			return nil, fmt.Errorf("failed to process message at index %d: %w", i, err)
		}
		responses[i] = response
	}

	return responses, nil
}

// UpdateKnowledge 更新知识库
func (r *RagCustomerServiceImpl) UpdateKnowledge(ctx context.Context, kbID string, feedback Feedback) error {
	err := r.feedbackLearner.ProcessFeedback(ctx, feedback)
	if err != nil {
		return fmt.Errorf("failed to process feedback: %w", err)
	}

	err = r.feedbackLearner.UpdateKnowledgeBase(ctx, kbID, []LearningInsight{
		{
			Type:           "content_improvement",
			Description:    "Learned from user feedback",
			Confidence:     0.8,
			Recommendation: "Consider adding more information about this topic",
			Source:         "user_feedback",
			CreatedAt:      time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update knowledge base: %w", err)
	}

	return nil
}

// GetSessionMetrics 获取会话指标
func (r *RagCustomerServiceImpl) GetSessionMetrics(ctx context.Context, sessionID string) (*SessionMetrics, error) {
	metrics := &SessionMetrics{
		SessionID:        sessionID,
		StartTime:        time.Now().Add(-1 * time.Hour), 
		EndTime:          time.Now(),
		MessageCount:     5,      
		AvgResponseTime:  1500.0, 
		ResolutionRate:   0.8,    
		UserSatisfaction: 4.2,    
		FeedbackCount:    2,      
	}

	return metrics, nil
}

// ListSessions 列出会话
func (r *RagCustomerServiceImpl) ListSessions(ctx context.Context, userID, platform string, status SessionStatus) ([]Session, error) {
	sessions, err := r.dialogManager.ListUserSessions(ctx, userID, platform, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	return sessions, nil
}

// GetUserContext 获取用户上下文
func (r *RagCustomerServiceImpl) GetUserContext(ctx context.Context, userID, platform string) (*Context, error) {
	sessions, err := r.dialogManager.ListUserSessions(ctx, userID, platform, SessionActive)
	if err != nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessions) == 0 {
		return &Context{}, nil
	}

	latestSession := sessions[0]
	for _, session := range sessions[1:] {
		if session.UpdatedAt.After(latestSession.UpdatedAt) {
			latestSession = session
		}
	}

	return &latestSession.Conversation.Context, nil
}

// generateResponseID 生成回复ID
func generateResponseID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("resp_%d_fallback", time.Now().UnixNano())
	}
	return fmt.Sprintf("resp_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// generateMessageID 生成消息ID
func generateMessageID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("msg_%d_fallback", time.Now().UnixNano())
	}
	return fmt.Sprintf("msg_%d_%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// NewDefaultRagCustomerService 创建默认实现的RAG客服服务
func NewDefaultRagCustomerService(retrievalService ragretrieval.RagRetrievalService) *RagCustomerServiceImpl {
	dialogManager := NewInMemoryDialogManager(nil)
	contextUnderstanding := NewContextUnderstandingService(nil)

	llmService := NewLLMServiceAdapter(llm.NewLLMService())

	responseGenerator := NewResponseGeneratorImpl(llmService, nil)
	qualityAssessor := NewQualityAssessorImpl(nil)
	feedbackLearner := NewSimpleFeedbackLearner()

	config := &CustomerServiceConfig{}

	return NewRagCustomerService(
		dialogManager,
		contextUnderstanding,
		responseGenerator,
		qualityAssessor,
		feedbackLearner,
		retrievalService,
		config,
	)
}

// SimpleFeedbackLearner 简单的反馈学习器实现
type SimpleFeedbackLearner struct{}

// NewSimpleFeedbackLearner 创建简单反馈学习器
func NewSimpleFeedbackLearner() *SimpleFeedbackLearner {
	return &SimpleFeedbackLearner{}
}

// ProcessFeedback 处理反馈
func (sfl *SimpleFeedbackLearner) ProcessFeedback(ctx context.Context, feedback Feedback) error {
	logger.Infof("Received feedback for session %s, response %s: rating=%d, helpful=%t, comment=%s",
		feedback.SessionID, feedback.ResponseID, feedback.Rating, feedback.IsHelpful, feedback.Comment)

	return nil
}

// LearnFromFeedback 从反馈中学习
func (sfl *SimpleFeedbackLearner) LearnFromFeedback(ctx context.Context, feedback Feedback) error {
	return nil
}

// GetLearningInsights 获取学习洞察
func (sfl *SimpleFeedbackLearner) GetLearningInsights(ctx context.Context, sessionID string) ([]LearningInsight, error) {
	return []LearningInsight{}, nil
}

// UpdateKnowledgeBase 更新知识库
func (sfl *SimpleFeedbackLearner) UpdateKnowledgeBase(ctx context.Context, kbID string, insights []LearningInsight) error {
	return nil
}

