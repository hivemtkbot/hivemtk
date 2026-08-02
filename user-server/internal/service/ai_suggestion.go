package service

import (
	"context"
	"strconv"

	"marketing/internal/model"
	"marketing/internal/repository"
	"marketing/internal/websocket"
)

// ============================================================================
// AI 建议服务（ai_suggestion.go）
// ----------------------------------------------------------------------------
// 从 customer_session.go 拆分（方向C）。
// 职责：AI 推理时给坐席的实时建议（按会话 ID 聚合）。
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §二
// ============================================================================

// AISuggestionService AI建议服务
type AISuggestionService struct {
	suggestionRepo *repository.AISuggestionRepository
	sessionRepo    *repository.CustomerSessionRepository
}

// NewAISuggestionService 创建AI建议服务实例
func NewAISuggestionService() *AISuggestionService {
	return &AISuggestionService{
		suggestionRepo: repository.NewAISuggestionRepository(),
		sessionRepo:    repository.NewCustomerSessionRepository(),
	}
}

// CreateSuggestion 创建AI建议
func (s *AISuggestionService) CreateSuggestion(ctx context.Context, sessionID string, messageID uint, suggestion string, confidence float64, source string) (*model.AISuggestion, error) {
	ais := &model.AISuggestion{
		SessionID:  sessionID,
		MessageID:  messageID,
		Suggestion: suggestion,
		Confidence: confidence,
		Source:     source,
	}

	if err := s.suggestionRepo.Create(ctx, ais); err != nil {
		return nil, err
	}

	// 通知客服
	session, _ := s.sessionRepo.GetBySessionID(ctx, sessionID)
	if session != nil && session.AgentID > 0 {
		websocket.NotifyAISuggestion(strconv.FormatUint(uint64(session.AgentID), 10), ais)
	}

	return ais, nil
}

// GetSuggestions 获取会话的AI建议
func (s *AISuggestionService) GetSuggestions(ctx context.Context, sessionID string) ([]*model.AISuggestion, error) {
	return s.suggestionRepo.GetBySessionID(ctx, sessionID)
}

// UseSuggestion 使用AI建议
func (s *AISuggestionService) UseSuggestion(ctx context.Context, id uint, agentID uint) error {
	return s.suggestionRepo.MarkAsUsed(ctx, id, agentID)
}
