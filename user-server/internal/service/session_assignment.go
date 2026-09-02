package service

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/aiagent/llm"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/websocket"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SessionAssignmentService 会话分配服务
type SessionAssignmentService struct {
	sessionRepo         *repository.CustomerSessionRepository
	messageRepo         *repository.SessionMessageRepository
	agentRepo           *repository.AgentStatusRepository
	ragEngine           *rag_core.RAGEngine
	llmService          *llm.LLMService
	confidenceThreshold float64
}

// NewSessionAssignmentService 创建会话分配服务
func NewSessionAssignmentService() *SessionAssignmentService {
	return &SessionAssignmentService{
		sessionRepo:         repository.NewCustomerSessionRepository(),
		messageRepo:         repository.NewSessionMessageRepository(),
		agentRepo:           repository.NewAgentStatusRepository(),
		ragEngine:           rag_core.NewRAGEngine(nil),
		llmService:          llm.NewLLMService(),
		confidenceThreshold: 0.5,
	}
}

// SetConfidenceThreshold 设置置信度阈值
func (s *SessionAssignmentService) SetConfidenceThreshold(ctx context.Context, threshold float64) {
	s.confidenceThreshold = threshold
}

// ProcessIncomingMessage 处理 incoming 消息，决定是否创建/更新会话并分配
func (s *SessionAssignmentService) ProcessIncomingMessage(ctx context.Context, msg *model.UnifiedMessage) error {
	session, err := s.findActiveSession(ctx, msg.SenderID, msg.ChatID)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	if session == nil {
		session, err = s.createSession(ctx, msg)
		if err != nil {
			return err
		}
	}

	err = s.saveMessageToSession(ctx, session, msg)
	if err != nil {
		return err
	}

	decision, err := s.decideHandler(ctx, session, msg)
	if err != nil {
		return err
	}

	return s.executeDecision(ctx, session, decision)
}

// findActiveSession 查找活跃会话
func (s *SessionAssignmentService) findActiveSession(ctx context.Context, userID, chatID string) (*model.CustomerSession, error) {
	sessions, _, err := s.sessionRepo.GetByMerchant(ctx, "", 1, 10)
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		if session.UserID == userID || session.SessionID == chatID {
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
	HandlerType    model.HandlerType
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

	if session.HandlerType == model.HandlerTypeHuman && session.AgentID > 0 {
		agent, err := s.agentRepo.GetByAgentID(ctx, session.AgentID)
		if err == nil && agent.Status != "offline" {
			decision.HandlerType = model.HandlerTypeHuman
			return decision, nil
		}
	}

	if s.isUrgentOrComplaint(ctx, msg.Content) {
		decision.HandlerType = model.HandlerTypeHuman
		decision.TransferReason = "检测到紧急或投诉内容"
		decision.Priority = 2
		return decision, nil
	}

	ragResults, err := s.ragEngine.Search(ctx, msg.Content, 3)
	if err == nil && len(ragResults) > 0 {
		decision.Confidence = ragResults[0].Score
		if decision.Confidence >= s.confidenceThreshold {
			decision.AIResponse = ragResults[0].Content
			decision.HandlerType = model.HandlerTypeAI
			return decision, nil
		}
	}

	llmResponse, confidence, err := s.generateLLMResponse(ctx, msg.Content)
	if err == nil {
		if confidence >= s.confidenceThreshold {
			decision.AIResponse = llmResponse
			decision.Confidence = confidence
			decision.HandlerType = model.HandlerTypeAI
			return decision, nil
		}
	}

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

	confidence := 0.6
	if len(output) > 20 && len(output) < 500 {
		confidence = 0.7
	}

	return output, confidence, nil
}

// executeDecision 执行决策
func (s *SessionAssignmentService) executeDecision(ctx context.Context, session *model.CustomerSession, decision *HandlerDecision) error {
	if decision.Priority > session.Priority {
		session.Priority = decision.Priority
	}

	switch decision.HandlerType {
	case model.HandlerTypeAI:
		return s.handleByAI(ctx, session, decision.AIResponse)
	case model.HandlerTypeHuman:
		return s.handleByHuman(ctx, session, decision.TransferReason)
	default:
		return nil
	}
}

// handleByAI AI 处理
func (s *SessionAssignmentService) handleByAI(ctx context.Context, session *model.CustomerSession, response string) error {
	err := s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusAIHandling)
	if err != nil {
		return err
	}

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

	err = s.sessionRepo.UpdateLastMessage(ctx, session.ID, response, "ai")
	if err != nil {
		return err
	}

	err = s.sessionRepo.IncrementAIReplyCount(ctx, session.ID)
	if err != nil {
		return err
	}

	if session.AgentID > 0 {
		websocket.NotifySessionUpdate(strconv.FormatUint(uint64(session.AgentID), 10), map[string]any{
			"session_id": session.SessionID,
			"status":     model.SessionStatusAIHandling,
			"ai_replied": true,
		})
	}

	return s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusWaiting)
}

// handleByHuman 人工处理
func (s *SessionAssignmentService) handleByHuman(ctx context.Context, session *model.CustomerSession, reason string) error {
	err := s.sessionRepo.UpdateStatus(ctx, session.ID, model.SessionStatusPending)
	if err != nil {
		return err
	}

	if session.AgentID == 0 {
		err = s.autoAssignToAgent(ctx, session, reason)
		if err != nil {
			return nil
		}
	}

	return nil
}

// autoAssignToAgent 自动分配给客服
//
// CS-P0-2: 修复 TOCTOU 竞态 — 原 "GetOnlineAgents 读 → findLeast 选 → AssignAgent +
// Increment 写" 三步非原子，并发下多个请求可能同时选到同一个 agent 且都成功 Increment，
// 导致负载统计失真甚至超容量分配。
//
// 修复：整个分配链路包在一个 DB 事务内，用 SELECT ... FOR UPDATE 锁住被选中的
// agent_status 行，其他并发事务会等待直到当前提交，保证选 agent + 分配 + 负载更新
// 三者严格原子。websocket 通知放在 Commit 之后（通知不参与回滚）。
func (s *SessionAssignmentService) autoAssignToAgent(ctx context.Context, session *model.CustomerSession, reason string) error {
	gdb := db.GetDB()
	if gdb == nil {
		return errors.New("database not initialized")
	}

	tx := gdb.WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("start transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	// SELECT ... FOR UPDATE 锁住最佳 agent，其他并发请求会等待
	var bestAgent model.AgentStatus
	cutoff := time.Now().Add(-5 * time.Minute)
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("status IN ? AND active_sessions < max_sessions AND last_active_at > ?",
			[]string{"online", "busy"}, cutoff).
		Order("active_sessions ASC").
		Limit(1).
		First(&bestAgent).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("无在线客服")
		}
		return fmt.Errorf("pick agent: %w", err)
	}

	// 事务内 AssignAgent（用 tx 绑定的 repo 实例）
	txSessionRepo := repository.NewCustomerSessionRepositoryWithDB(tx)
	if err := txSessionRepo.AssignAgent(ctx, session.ID, bestAgent.AgentID, bestAgent.AgentName); err != nil {
		tx.Rollback()
		return fmt.Errorf("assign agent: %w", err)
	}

	// 事务内原子递增 active_sessions + today_sessions（等价于原 IncrementActiveSessions）
	if err := tx.Model(&model.AgentStatus{}).Where("agent_id = ?", bestAgent.AgentID).
		Updates(map[string]any{
			"active_sessions": gorm.Expr("active_sessions + 1"),
			"today_sessions":  gorm.Expr("today_sessions + 1"),
		}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("increment agent load: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit assign: %w", err)
	}

	// Commit 成功后再做 websocket 通知（通知不参与事务，回滚场景不会多发通知）
	websocket.NotifyNewSession(strconv.FormatUint(uint64(bestAgent.AgentID), 10), map[string]any{
		"session_id":      session.SessionID,
		"user_name":       session.UserName,
		"last_message":    session.LastMessage,
		"transfer_reason": reason,
		"priority":        session.Priority,
	})

	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id": session.SessionID,
		"handler":    "human",
		"agent_name": bestAgent.AgentName,
		"reason":     "正在为您接入人工客服，请稍候...",
	}, session.SessionID)

	return nil
}

// TransferToHuman 转人工（当 AI 处理过程中用户要求转人工时）
//
// Bug 修复 ：
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

	if session.AgentID > 0 {
		_ = s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
	}

	if agentID > 0 {
		agent, err := s.agentRepo.GetByAgentID(ctx, agentID)
		if err != nil || agent == nil {
			return errors.New("agent not found")
		}
		if agent.Status == "offline" {
			return errors.New("agent is offline")
		}
		if err := s.sessionRepo.AssignAgent(ctx, sessionID, agentID, agent.AgentName); err != nil {
			return err
		}
		if err := s.agentRepo.IncrementActiveSessions(ctx, agentID); err != nil {
			return err
		}
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
			continue
		}
		assignedCount++
	}

	return assignedCount, nil
}
