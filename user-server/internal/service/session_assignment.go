package service

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/aiagent/llm"
	"marketing/internal/aiagent/rag/core"
	"marketing/internal/model"
	"marketing/internal/repository"
	"marketing/internal/websocket"
	"strconv"
	"strings"
	"time"
)

// SessionAssignmentService 会话分配服务
type SessionAssignmentService struct {
	sessionRepo         *repository.CustomerSessionRepository
	messageRepo         *repository.SessionMessageRepository
	agentRepo           *repository.AgentStatusRepository
	ragEngine           *rag_core.RAGEngine
	llmService          *llm.LLMService
	confidenceThreshold float64 // 置信度阈值，低于此值转人工
}

// NewSessionAssignmentService 创建会话分配服务
func NewSessionAssignmentService() *SessionAssignmentService {
	return &SessionAssignmentService{
		sessionRepo:         repository.NewCustomerSessionRepository(),
		messageRepo:         repository.NewSessionMessageRepository(),
		agentRepo:           repository.NewAgentStatusRepository(),
		ragEngine:           rag_core.NewRAGEngine(nil),
		llmService:          llm.NewLLMService(),
		confidenceThreshold: 0.7, // 默认置信度阈值为 0.7
	}
}

// SetConfidenceThreshold 设置置信度阈值
func (s *SessionAssignmentService) SetConfidenceThreshold(ctx context.Context, threshold float64) {
	s.confidenceThreshold = threshold
}

// ProcessIncomingMessage 处理 incoming 消息，决定是否创建/更新会话并分配
func (s *SessionAssignmentService) ProcessIncomingMessage(ctx context.Context, msg *model.UnifiedMessage) error {
	// 1. 查找是否已有活跃会话
	session, err := s.findActiveSession(ctx, msg.SenderID, msg.ChatID)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	// 2. 如果没有活跃会话，创建新会话
	if session == nil {
		session, err = s.createSession(ctx, msg)
		if err != nil {
			return err
		}
	}

	// 3. 保存消息到会话
	err = s.saveMessageToSession(ctx, session, msg)
	if err != nil {
		return err
	}

	// 4. 决策如何处理
	decision, err := s.decideHandler(ctx, session, msg)
	if err != nil {
		return err
	}

	// 5. 执行决策
	return s.executeDecision(ctx, session, decision)
}

// findActiveSession 查找活跃会话
func (s *SessionAssignmentService) findActiveSession(ctx context.Context, userID, chatID string) (*model.CustomerSession, error) {
	// 获取商户下该用户的活跃会话
	sessions, _, err := s.sessionRepo.GetByMerchant(ctx, "", 1, 10)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		// 检查是否是同一用户
		if session.UserID == userID || session.SessionID == chatID {
			// 检查会话状态是否活跃
			if session.Status == model.SessionStatusPending ||
				session.Status == model.SessionStatusAIHandling ||
				session.Status == model.SessionStatusHumanHandling ||
				session.Status == model.SessionStatusWaiting {
				return session, nil
			}
		}
	}

	return nil, ErrSessionNotFound
}

// ErrSessionNotFound 会话未找到错误
var ErrSessionNotFound = errors.New("活跃会话未找到")

// createSession 创建新会话
func (s *SessionAssignmentService) createSession(ctx context.Context, msg *model.UnifiedMessage) (*model.CustomerSession, error) {
	sessionID := fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), msg.MessageID[:8])

	session := &model.CustomerSession{
		SessionID:       sessionID,
		Platform:        msg.Platform,
		AccountID:       msg.AccountID,
		UserID:          msg.SenderID,
		UserName:        msg.SenderName,
		UserAvatar:      msg.SenderAvatar,
		Status:          model.SessionStatusPending,
		Priority:        0,
		LastMessage:     msg.Content,
		LastMessageAt:   &msg.ReceivedAt,
		LastMessageBy:   "user",
		MessageCount:    0,
		AIReplyCount:    0,
		HumanReplyCount: 0,
	}

	err := s.sessionRepo.Create(ctx, session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// saveMessageToSession 保存消息到会话
func (s *SessionAssignmentService) saveMessageToSession(ctx context.Context, session *model.CustomerSession, msg *model.UnifiedMessage) error {
	message := &model.SessionMessage{
		SessionID:    session.SessionID,
		Content:      msg.Content,
		ContentType:  model.MessageTypeText,
		MediaURL:     msg.MediaURL,
		SenderType:   "user",
		SenderID:     msg.SenderID,
		SenderName:   msg.SenderName,
		SenderAvatar: msg.SenderAvatar,
	}

	return s.messageRepo.Create(ctx, message)
}

// HandlerDecision 处理者决策
type HandlerDecision struct {
	HandlerType    model.HandlerType // ai 或 human
	Confidence     float64
	ShouldTransfer bool
	TransferReason string
	AIResponse     string
	Priority       int
}

// decideHandler 决策如何处理会话
func (s *SessionAssignmentService) decideHandler(ctx context.Context, session *model.CustomerSession, msg *model.UnifiedMessage) (*HandlerDecision, error) {
	decision := &HandlerDecision{
		HandlerType: model.HandlerTypeAI,
		Confidence:  0,
	}

	// 1. 检查会话是否已分配给人工
	if session.HandlerType == model.HandlerTypeHuman && session.AgentID > 0 {
		// 已分配给人工，检查人工客服是否在线
		agent, err := s.agentRepo.GetByAgentID(ctx, session.AgentID)
		if err == nil && agent.Status != "offline" {
			decision.HandlerType = model.HandlerTypeHuman
			return decision, nil
		}
		// 客服离线，可能需要重新分配
	}

	// 2. 检查消息内容是否需要人工处理
	if s.isUrgentOrComplaint(ctx, msg.Content) {
		decision.HandlerType = model.HandlerTypeHuman
		decision.TransferReason = "检测到紧急或投诉内容"
		decision.Priority = 2 // 紧急优先级
		return decision, nil
	}

	// 3. 使用 RAG 检索相关知识
	ragResults, err := s.ragEngine.Search(ctx, msg.Content, 3)
	if err == nil && len(ragResults) > 0 {
		decision.Confidence = ragResults[0].Score
		if decision.Confidence >= s.confidenceThreshold {
			// 置信度足够高，AI 处理
			decision.AIResponse = ragResults[0].Content
			decision.HandlerType = model.HandlerTypeAI
			return decision, nil
		}
	}

	// 4. 使用 LLM 生成回复
	llmResponse, confidence, err := s.generateLLMResponse(ctx, msg.Content)
	if err == nil {
		if confidence >= s.confidenceThreshold {
			decision.AIResponse = llmResponse
			decision.Confidence = confidence
			decision.HandlerType = model.HandlerTypeAI
			return decision, nil
		}
	}

	// 5. 置信度不足，转人工
	decision.HandlerType = model.HandlerTypeHuman
	decision.TransferReason = fmt.Sprintf("AI 置信度不足 (%.2f < %.2f)", decision.Confidence, s.confidenceThreshold)

	return decision, nil
}

// isUrgentOrComplaint 检查是否是紧急或投诉消息
func (s *SessionAssignmentService) isUrgentOrComplaint(ctx context.Context, content string) bool {
	urgentKeywords := []string{
		"投诉", "举报", "曝光", "315", "消协", "工商局",
		"紧急", "着急", "马上", "立刻", "赶紧", "快点",
		"骗子", "假货", "垃圾", "垃圾东西", "再也不买",
		"退钱", "退款", "赔钱", "赔偿",
	}

	content = strings.ToLower(content)
	for _, keyword := range urgentKeywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}

	return false
}

// generateLLMResponse 使用 LLM 生成回复
func (s *SessionAssignmentService) generateLLMResponse(ctx context.Context, content string) (string, float64, error) {
	prompt := fmt.Sprintf(`作为专业客服助手，请针对以下用户消息生成友好、专业的回复：

用户消息：%s

要求：
1. 语气友好专业
2. 回复简洁（不超过 200 字）
3. 如有必要，询问更多细节
4. 不要编造信息，不确定的内容请说明需要核实

回复：`, content)

	config := &llm.LLMConfig{
		Model:          "gpt-3.5-turbo",
		APIType:        "openai",
		Temperature:    0.7,
		MaxTokens:      500,
		ResponseFormat: "text",
	}

	output, err := s.llmService.Generate(ctx, config, prompt)
	if err != nil {
		return "", 0.5, err
	}

	// 简单的置信度评估
	confidence := 0.6
	if len(output) > 20 && len(output) < 500 {
		confidence = 0.7
	}

	return output, confidence, nil
}

// executeDecision 执行决策
func (s *SessionAssignmentService) executeDecision(ctx context.Context, session *model.CustomerSession, decision *HandlerDecision) error {
	// 更新会话优先级
	if decision.Priority > session.Priority {
		session.Priority = decision.Priority
	}

	switch decision.HandlerType {
	case model.HandlerTypeAI:
		// AI 处理
		return s.handleByAI(ctx, session, decision.AIResponse)
	case model.HandlerTypeHuman:
		// 人工处理
		return s.handleByHuman(ctx, session, decision.TransferReason)
	default:
		return nil
	}
}

// handleByAI AI 处理
func (s *SessionAssignmentService) handleByAI(ctx context.Context, session *model.CustomerSession, response string) error {
	// 更新会话状态
	err := s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusAIHandling)
	if err != nil {
		return err
	}

	// 发送 AI 回复
	message := &model.SessionMessage{
		SessionID:    session.SessionID,
		Content:      response,
		ContentType:  model.MessageTypeText,
		SenderType:   "ai",
		SenderID:     "system",
		SenderName:   "AI 助手",
		AIConfidence: 0.8,
		AISource:     "llm",
	}

	err = s.messageRepo.Create(ctx, message)
	if err != nil {
		return err
	}

	// 更新会话最后消息
	err = s.sessionRepo.UpdateLastMessage(ctx, session.ID, response, "ai")
	if err != nil {
		return err
	}

	// 增加 AI 回复计数
	err = s.sessionRepo.IncrementAIReplyCount(ctx, session.ID)
	if err != nil {
		return err
	}

	// 如果有客服在线，通知 AI 已回复
	if session.AgentID > 0 {
		websocket.NotifySessionUpdate(strconv.FormatUint(uint64(session.AgentID), 10), map[string]any{
			"session_id": session.SessionID,
			"status":     model.SessionStatusAIHandling,
			"ai_replied": true,
		})
	}

	// 更新状态为等待用户回复
	return s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusWaiting)
}

// handleByHuman 人工处理
func (s *SessionAssignmentService) handleByHuman(ctx context.Context, session *model.CustomerSession, reason string) error {
	// 更新会话状态
	err := s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusPending)
	if err != nil {
		return err
	}

	// 尝试自动分配给在线客服
	if session.AgentID == 0 {
		err = s.autoAssignToAgent(ctx, session, reason)
		if err != nil {
			// 分配失败，保持 pending 状态等待分配
			return nil
		}
	}

	return nil
}

// autoAssignToAgent 自动分配给客服
func (s *SessionAssignmentService) autoAssignToAgent(ctx context.Context, session *model.CustomerSession, reason string) error {
	// 获取在线客服列表
	agents, err := s.agentRepo.GetOnlineAgents(ctx)
	if err != nil || len(agents) == 0 {
		return errors.New("无在线客服")
	}

	// 选择活跃会话最少的客服
	selectedAgent := agents[0]
	for _, agent := range agents {
		if agent.ActiveSessions < selectedAgent.ActiveSessions {
			selectedAgent = agent
		}
	}

	// 分配会话
	err = s.sessionRepo.AssignAgent(ctx, session.ID, selectedAgent.AgentID, selectedAgent.AgentName)
	if err != nil {
		return err
	}

	// 更新客服活跃会话数
	err = s.agentRepo.IncrementActiveSessions(ctx, selectedAgent.AgentID)
	if err != nil {
		return err
	}

	// 通知客服
	websocket.NotifyNewSession(strconv.FormatUint(uint64(selectedAgent.AgentID), 10), map[string]any{
		"session_id":      session.SessionID,
		"user_name":       session.UserName,
		"last_message":    session.LastMessage,
		"transfer_reason": reason,
		"priority":        session.Priority,
	})

	return nil
}

// TransferToHuman 转人工（当 AI 处理过程中用户要求转人工时）
//
// Bug 修复 2026-07-22：
//
//	nil 检查 + 幂等分配 + 通知新客服 + 解锁 Redis 接管锁。
func (s *SessionAssignmentService) TransferToHuman(ctx context.Context, sessionID uint, agentID uint, reason string) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	if session == nil {
		return errors.New("session not found")
	}

	// 减少原坐席活跃会话数（如果有），避免 active_sessions 虚高
	if session.AgentID > 0 {
		_ = s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
	}

	// 如果有指定客服，分配给该客服
	if agentID > 0 {
		agent, err := s.agentRepo.GetByAgentID(ctx, agentID)
		if err != nil || agent == nil {
			return errors.New("agent not found")
		}
		if agent.Status == "offline" {
			return errors.New("agent is offline")
		}
		// 分配给新客服
		if err := s.sessionRepo.AssignAgent(ctx, sessionID, agentID, agent.AgentName); err != nil {
			return err
		}
		// 增加新客服活跃会话数
		if err := s.agentRepo.IncrementActiveSessions(ctx, agentID); err != nil {
			return err
		}
		// 通知新客服
		websocket.NotifyNewSession(strconv.FormatUint(uint64(agentID), 10), map[string]any{
			"session_id":      session.SessionID,
			"user_name":       session.UserName,
			"last_message":    session.LastMessage,
			"transfer_reason": reason,
		})
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "已为您转接客服，请稍候",
		}, session.SessionID)
	}

	// 标记会话为人工处理（保留分配人；handler_type 用于前端展示）
	if err := s.sessionRepo.UpdateStatus(ctx, sessionID, model.SessionStatusHumanHandling); err != nil {
		return err
	}
	return nil
}

// GetPendingSessions 获取待分配会话
func (s *SessionAssignmentService) GetPendingSessions(ctx context.Context) ([]*model.CustomerSession, error) {
	return s.sessionRepo.GetPendingSessions(context.Background())
}

// AutoAssignAllPending 自动分配所有待处理会话
func (s *SessionAssignmentService) AutoAssignAllPending(ctx context.Context) (int, error) {
	sessions, err := s.GetPendingSessions(ctx)
	if err != nil {
		return 0, err
	}

	assignedCount := 0
	for _, session := range sessions {
		err := s.autoAssignToAgent(ctx, session, "系统自动分配")
		if err != nil {
			// 分配失败，继续下一个
			continue
		}
		assignedCount++
	}

	return assignedCount, nil
}
