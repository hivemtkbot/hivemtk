package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"marketing/internal/model"
	"marketing/internal/repository"
	"marketing/internal/websocket"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CustomerSessionService 客服会话服务
type CustomerSessionService struct {
	sessionRepo    *repository.CustomerSessionRepository
	messageRepo    *repository.SessionMessageRepository
	agentRepo      *repository.AgentStatusRepository
	suggestionRepo *repository.AISuggestionRepository
	blacklistRepo  *repository.UserBlacklistRepository
}

// NewCustomerSessionService 创建客服会话服务实例
func NewCustomerSessionService() *CustomerSessionService {
	return &CustomerSessionService{
		sessionRepo:    repository.NewCustomerSessionRepository(),
		messageRepo:    repository.NewSessionMessageRepository(),
		agentRepo:      repository.NewAgentStatusRepository(),
		suggestionRepo: repository.NewAISuggestionRepository(),
		blacklistRepo:  repository.NewUserBlacklistRepository(),
	}
}

// NewCustomerSessionServiceWithDB 创建指定数据库连接的客服会话服务实例
//
// 用于：① 测试注入独立真实测试库（testutil.NewTestDB）② 多库/独立事务场景。
// 避免依赖全局 db.GetDB()，保证 reach.web.send 等触达链路使用调用方真实 DB。
func NewCustomerSessionServiceWithDB(db *gorm.DB) *CustomerSessionService {
	return &CustomerSessionService{
		sessionRepo:    repository.NewCustomerSessionRepositoryWithDB(db),
		messageRepo:    repository.NewSessionMessageRepositoryWithDB(db),
		agentRepo:      repository.NewAgentStatusRepositoryWithDB(db),
		suggestionRepo: repository.NewAISuggestionRepositoryWithDB(db),
		blacklistRepo:  repository.NewUserBlacklistRepositoryWithDB(db),
	}
}

// CreateSessionRequest 创建会话请求
type CreateSessionRequest struct {
	Platform   model.Platform `json:"platform" binding:"required"`
	AccountID  string         `json:"account_id" binding:"required"`
	UserID     string         `json:"user_id" binding:"required"`
	UserName   string         `json:"user_name"`
	UserAvatar string         `json:"user_avatar"`
	UserPhone  string         `json:"user_phone"`
	UserEmail  string         `json:"user_email"`
}

// CreateSession 创建会话
//
// 2026-07-22 拉黑串联：进入时先校验 IsUserBlacklisted(userID, platform)，
// 命中即拒绝创建（避免已被拉黑的访客通过新会话绕过黑名单）。
func (s *CustomerSessionService) CreateSession(req *CreateSessionRequest) (*model.CustomerSession, error) {
	if req.UserID != "" {
		banned, blErr := s.blacklistRepo.IsBlacklisted(req.UserID, req.Platform)
		if blErr != nil {
			return nil, blErr
		}
		if banned {
			return nil, errors.New("该访客已被加入黑名单，无法创建新会话")
		}
	}

	sessionID := generateSessionID()

	session := &model.CustomerSession{
		SessionID:  sessionID,
		Platform:   req.Platform,
		AccountID:  req.AccountID,
		UserID:     req.UserID,
		UserName:   req.UserName,
		UserAvatar: req.UserAvatar,
		UserPhone:  req.UserPhone,
		UserEmail:  req.UserEmail,
		Status:     model.SessionStatusPending,
		Priority:   0,
	}

	if err := s.sessionRepo.Create(session); err != nil {
		return nil, err
	}

	return session, nil
}

// GetSessions 获取会话列表
func (s *CustomerSessionService) GetSessions(status model.SessionStatus, page, pageSize int) ([]*model.CustomerSession, int64, error) {
	return s.sessionRepo.GetByMerchant(status, page, pageSize)
}

func (s *CustomerSessionService) GetPendingSessions(page, pageSize int) ([]*model.CustomerSession, error) {
	statuses := []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
		model.SessionStatusWaiting,
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	var sessions []*model.CustomerSession
	if err := s.sessionRepo.GetDB().Where("status IN ?", statuses).
		Order("priority DESC, COALESCE(last_message_at, created_at) ASC").
		Offset(offset).Limit(pageSize).Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *CustomerSessionService) CountPendingSessions() (int64, error) {
	statuses := []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
		model.SessionStatusWaiting,
	}
	var count int64
	if err := s.sessionRepo.GetDB().Model(&model.CustomerSession{}).
		Where("status IN ?", statuses).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetSessionByID 获取会话详情
func (s *CustomerSessionService) GetSessionByID(id uint) (*model.CustomerSession, error) {
	session, err := s.sessionRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// AssignSessionRequest 分配会话请求
type AssignSessionRequest struct {
	SessionID uint `json:"session_id" binding:"required"`
	AgentID   uint `json:"agent_id" binding:"required"`
}

// AssignSession 分配会话给客服
func (s *CustomerSessionService) AssignSession(req *AssignSessionRequest) error {
	// 获取客服信息
	agent, err := s.agentRepo.GetByAgentID(req.AgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	// 检查客服是否可分配
	if agent.Status == "offline" {
		return errors.New("客服不在线")
	}
	if agent.ActiveSessions >= agent.MaxSessions {
		return errors.New("客服会话已满")
	}

	// 分配会话
	if err := s.sessionRepo.AssignAgent(req.SessionID, req.AgentID, agent.AgentName); err != nil {
		return err
	}

	// 更新客服活跃会话数
	if err := s.agentRepo.IncrementActiveSessions(req.AgentID); err != nil {
		return err
	}

	// 通知客服
	session, _ := s.sessionRepo.GetByID(req.SessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(req.AgentID), 10), session)
		// 通知访客：人工客服已接入（完成网页客服渠道的坐席侧闭环）
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "客服已接入，正在为您服务",
		}, session.SessionID)
	}

	return nil
}

// AutoAssign 自动分配会话
func (s *CustomerSessionService) AutoAssign(sessionID uint) error {
	// 获取在线客服
	agents, err := s.agentRepo.GetOnlineAgents()
	if err != nil || len(agents) == 0 {
		return errors.New("没有可用的在线客服")
	}

	// 选择活跃会话最少的客服
	selectedAgent := agents[0]
	for _, agent := range agents {
		if agent.ActiveSessions < selectedAgent.ActiveSessions {
			selectedAgent = agent
		}
	}

	return s.AssignSession(&AssignSessionRequest{
		SessionID: sessionID,
		AgentID:   selectedAgent.AgentID,
	})
}

// SendMessageRequest 发送消息请求
type SendMessageRequest struct {
	SessionID   string            `json:"session_id"`
	Content     string            `json:"content" binding:"required"`
	ContentType model.MessageType `json:"content_type"`
	MediaURL    string            `json:"media_url"`
	// 2026-07-21 修复：SenderType 不再 binding:"required"，发送者身份由 controller 从鉴权上下文派生。
	// 旧 binding:"required" 会导致坐席/管理员调用 API 时必须伪造 sender_type 字段（即使 controller 会覆盖），
	// 否则直接 400。这是安全加固的反向副作用——把"必填校验"留在 controller 中更合适。
	SenderType   string  `json:"sender_type"`
	SenderID     string  `json:"sender_id"`
	SenderName   string  `json:"sender_name"`
	SenderAvatar string  `json:"sender_avatar"`
	AIConfidence float64 `json:"ai_confidence"`
	AISource     string  `json:"ai_source"`
}

// SendMessage 发送消息
func (s *CustomerSessionService) SendMessage(req *SendMessageRequest) (*model.SessionMessage, error) {
	// 获取会话
	session, err := s.sessionRepo.GetBySessionID(req.SessionID)
	if err != nil {
		return nil, errors.New("会话不存在")
	}

	// 创建消息
	message := &model.SessionMessage{
		SessionID:    req.SessionID,
		Content:      req.Content,
		ContentType:  req.ContentType,
		MediaURL:     req.MediaURL,
		SenderType:   req.SenderType,
		SenderID:     req.SenderID,
		SenderName:   req.SenderName,
		SenderAvatar: req.SenderAvatar,
		AIConfidence: req.AIConfidence,
		AISource:     req.AISource,
	}

	if req.ContentType == "" {
		message.ContentType = model.MessageTypeText
	}

	if err := s.messageRepo.Create(message); err != nil {
		return nil, err
	}

	// 更新会话最后消息
	if err := s.sessionRepo.UpdateLastMessage(session.ID, req.Content, req.SenderType); err != nil {
		return nil, err
	}

	// 更新回复计数
	if req.SenderType == "ai" {
		s.sessionRepo.IncrementAIReplyCount(session.ID)
	} else if req.SenderType == "agent" {
		s.sessionRepo.IncrementHumanReplyCount(session.ID)
		// 更新客服消息计数
		if session.AgentID > 0 {
			s.agentRepo.IncrementTodayMessages(session.AgentID)
		}
	}

	// 通知客服新消息
	if session.AgentID > 0 && req.SenderType == "user" {
		websocket.NotifyNewMessage(strconv.FormatUint(uint64(session.AgentID), 10), message)
	}

	// 推送坐席/AI 回复给访客（实时），完成网页客服渠道闭环
	// 访客离线时 SendToVisitor 静默丢弃，依赖 WebSocket 重连后的离线消息补发
	if req.SenderType == "agent" || req.SenderType == "ai" {
		s.pushToVisitor(session.SessionID, message)
	}

	return message, nil
}

// pushToVisitor 将坐席/AI 的回复实时推送给访客端 WebSocket
// 若访客在线则标记 delivered_at，避免重连时离线补发重复展示
func (s *CustomerSessionService) pushToVisitor(sessionID string, message *model.SessionMessage) {
	payload := map[string]any{
		"session_id":   sessionID,
		"id":           message.ID,
		"content":      message.Content,
		"content_type": message.ContentType,
		"media_url":    message.MediaURL,
		"sender_type":  message.SenderType,
		"sender_name":  message.SenderName,
		"sender_id":    message.SenderID,
		"created_at":   message.CreatedAt,
	}
	if websocket.IsVisitorOnline(sessionID) {
		_ = websocket.SendToVisitor(websocket.TypeMessage, payload, sessionID)
		now := time.Now()
		_ = s.sessionRepo.GetDB().Model(&model.SessionMessage{}).
			Where("id = ?", message.ID).Update("delivered_at", &now).Error
	}
}

// GetMessages 获取会话消息列表
func (s *CustomerSessionService) GetMessages(sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
	// 验证会话归属
	session, err := s.sessionRepo.GetBySessionID(sessionID)
	if err != nil {
		return nil, 0, errors.New("会话不存在")
	}
	_ = session

	return s.messageRepo.GetBySessionID(sessionID, page, pageSize)
}

// UpdateSessionStatus 更新会话状态
func (s *CustomerSessionService) UpdateSessionStatus(sessionID uint, status model.SessionStatus) error {
	session, err := s.sessionRepo.GetByID(sessionID)
	if err != nil {
		return err
	}

	// 更新状态
	if err := s.sessionRepo.UpdateStatus(sessionID, status); err != nil {
		return err
	}

	// 如果是关闭会话，减少客服活跃会话数
	if status == model.SessionStatusResolved || status == model.SessionStatusClosed {
		if session.AgentID > 0 {
			s.agentRepo.DecrementActiveSessions(session.AgentID)
		}
	}

	// 通知客服
	if session.AgentID > 0 {
		websocket.NotifySessionUpdate(strconv.FormatUint(uint64(session.AgentID), 10), map[string]any{
			"session_id": session.SessionID,
			"status":     status,
		})
	}

	return nil
}

// RateSession 评价会话
func (s *CustomerSessionService) RateSession(sessionID uint, rating int, comment string) error {
	session, err := s.sessionRepo.GetByID(sessionID)
	if err != nil {
		return err
	}
	_ = session

	session.Rating = rating
	session.RatingComment = comment
	return s.sessionRepo.Update(session)
}

// TransferSession 转接会话
func (s *CustomerSessionService) TransferSession(sessionID uint, newAgentID uint) error {
	session, err := s.sessionRepo.GetByID(sessionID)
	if err != nil {
		return err
	}

	// 获取新客服信息
	newAgent, err := s.agentRepo.GetByAgentID(newAgentID)
	if err != nil {
		return errors.New("客服不存在")
	}

	// 减少原客服活跃会话数
	if session.AgentID > 0 {
		s.agentRepo.DecrementActiveSessions(session.AgentID)
	}

	// 分配给新客服
	if err := s.sessionRepo.AssignAgent(sessionID, newAgentID, newAgent.AgentName); err != nil {
		return err
	}

	// 增加新客服活跃会话数
	if err := s.agentRepo.IncrementActiveSessions(newAgentID); err != nil {
		return err
	}

	// 通知新客服
	session, _ = s.sessionRepo.GetByID(sessionID)
	if session != nil {
		websocket.NotifyNewSession(strconv.FormatUint(uint64(newAgentID), 10), session)
		// 通知访客：已转接至其他客服
		_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
			"session_id": session.SessionID,
			"handler":    "human",
			"reason":     "已为您转接客服，请稍候",
		}, session.SessionID)
	}

	return nil
}

// TagSession 标记会话
func (s *CustomerSessionService) TagSession(sessionID uint, tags []string) error {
	session, err := s.sessionRepo.GetByID(sessionID)
	if err != nil {
		return err
	}
	_ = session

	tagsJSON, _ := json.Marshal(tags)
	session.Tags = string(tagsJSON)
	return s.sessionRepo.Update(session)
}

// ============================================================================
// 方向10：坐席实时聊天看板 - AI/人工接管与切换
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §三
// ============================================================================

// TakeoverRequest 坐席接管请求
type TakeoverRequest struct {
	SessionID uint   `json:"session_id" binding:"required"`
	AgentID   uint   `json:"agent_id" binding:"required"`
	Reason    string `json:"reason"` // 接管原因：AI 答非所问 / 客户要求 / 投诉升级
}

// TakeoverByAgent 坐席接管 AI 会话
//
// 行为：
//  1. 把会话 handler_type 切到 human、status 切到 human_handling
//  2. 记录接管人 AgentID（若尚未分配则 AssignAgent 一次）
//  3. 给该会话加 Redis 人工接管锁（InboxIngressService 收到新消息时绕过 AI）
//  4. 通过 WebSocket 通知前端会话更新
//
// 幂等：同一坐席重复接管直接返回成功（不重复扣活跃数）
func (s *CustomerSessionService) TakeoverByAgent(ctx context.Context, req *TakeoverRequest) error {
	if req.SessionID == 0 || req.AgentID == 0 {
		return errors.New("session_id and agent_id required")
	}
	session, err := s.sessionRepo.GetByID(req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}

	// 校验坐席存在
	agent, err := s.agentRepo.GetByAgentID(req.AgentID)
	if err != nil || agent == nil {
		return errors.New("坐席不存在")
	}
	// 校验坐席状态：仅 online/busy 可接管
	if agent.Status == "offline" {
		return errors.New("坐席已离线，无法接管")
	}
	// 容量校验
	if agent.ActiveSessions >= agent.MaxSessions {
		return errors.New("坐席会话已满")
	}

	// 幂等：会话已分配给该坐席 → 直接锁定 + 切状态
	if session.AgentID == req.AgentID {
		session.HandlerType = model.HandlerTypeHuman
		session.Status = model.SessionStatusHumanHandling
		now := time.Now()
		session.LastMessageAt = &now
		if err := s.sessionRepo.Update(session); err != nil {
			return err
		}
		_ = s.lockHumanSession(ctx, session.SessionID, req.Reason)
		_ = s.notifySessionUpdate(session, "handler_changed", "human")
		return nil
	}

	// 接管：减少原坐席活跃数（如有）
	if session.AgentID > 0 {
		_ = s.agentRepo.DecrementActiveSessions(session.AgentID)
	}
	// 分配新坐席
	if err := s.sessionRepo.AssignAgent(req.SessionID, req.AgentID, agent.AgentName); err != nil {
		return err
	}
	_ = s.agentRepo.IncrementActiveSessions(req.AgentID)

	// 切 handler / status
	updated, err := s.sessionRepo.GetByID(req.SessionID)
	if err != nil {
		return err
	}
	updated.HandlerType = model.HandlerTypeHuman
	updated.Status = model.SessionStatusHumanHandling
	now := time.Now()
	updated.LastMessageAt = &now
	if err := s.sessionRepo.Update(updated); err != nil {
		return err
	}
	// 写 Redis 人工锁 + 推 WebSocket + 访客提示
	_ = s.lockHumanSession(ctx, updated.SessionID, req.Reason)
	_ = s.notifySessionUpdate(updated, "handler_changed", "human")
	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id": updated.SessionID,
		"handler":    "human",
		"reason":     "客服已接管，正在为您服务",
	}, updated.SessionID)
	return nil
}

// ReleaseToAIRequest 释放回 AI 请求
type ReleaseToAIRequest struct {
	SessionID uint `json:"session_id" binding:"required"`
	AgentID   uint `json:"agent_id" binding:"required"`
}

// ReleaseToAI 坐席释放会话回 AI 托管
//
// 行为：
//  1. 把 handler_type 切回 ai、status 切回 waiting
//  2. 解 Redis 人工锁（InboxIngressService 后续消息会重新走 AI 路由）
//  3. 坐席活跃数 -1
//  4. 推 WebSocket 给坐席与访客
//
// 仅当会话原本属于该坐席才允许释放（防止误操作别人会话）
func (s *CustomerSessionService) ReleaseToAI(ctx context.Context, req *ReleaseToAIRequest) error {
	if req.SessionID == 0 || req.AgentID == 0 {
		return errors.New("session_id and agent_id required")
	}
	session, err := s.sessionRepo.GetByID(req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}
	if session.AgentID != req.AgentID {
		return errors.New("无权操作：会话不属于该坐席")
	}

	session.HandlerType = model.HandlerTypeAI
	session.Status = model.SessionStatusWaiting
	session.AgentID = 0
	session.AgentName = ""
	now := time.Now()
	session.LastMessageAt = &now
	if err := s.sessionRepo.Update(session); err != nil {
		return err
	}
	_ = s.agentRepo.DecrementActiveSessions(req.AgentID)
	_ = s.unlockHumanSession(ctx, session.SessionID)
	_ = s.notifySessionUpdate(session, "handler_changed", "ai")
	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id": session.SessionID,
		"handler":    "ai",
		"reason":     "已切回 AI 托管，请稍候",
	}, session.SessionID)
	return nil
}

// SwitchHandlerRequest AI/人工切换请求（统一接口）
type SwitchHandlerRequest struct {
	SessionID   uint              `json:"session_id" binding:"required"`
	AgentID     uint              `json:"agent_id"`
	HandlerType model.HandlerType `json:"handler_type" binding:"required"` // ai / human
	Reason      string            `json:"reason"`
}

// SwitchHandler 通用 AI/人工切换（前端按钮只调一个接口）
//
// 委派给 TakeoverByAgent / ReleaseToAI，避免上层维护两条调用路径。
func (s *CustomerSessionService) SwitchHandler(ctx context.Context, req *SwitchHandlerRequest) error {
	if req.SessionID == 0 {
		return errors.New("session_id required")
	}
	switch req.HandlerType {
	case model.HandlerTypeHuman:
		if req.AgentID == 0 {
			return errors.New("切人工时 agent_id required")
		}
		return s.TakeoverByAgent(ctx, &TakeoverRequest{
			SessionID: req.SessionID,
			AgentID:   req.AgentID,
			Reason:    req.Reason,
		})
	case model.HandlerTypeAI:
		if req.AgentID == 0 {
			// agent_id=0 表示仅切 handler（保留原 AgentID 字段不动，仅切类型）。
			// 通过临时读出会话来取 AgentID
			sess, err := s.sessionRepo.GetByID(req.SessionID)
			if err != nil {
				return errors.New("会话不存在")
			}
			req.AgentID = sess.AgentID
			if req.AgentID == 0 {
				return errors.New("会话尚未分配坐席，无需切回 AI")
			}
		}
		return s.ReleaseToAI(ctx, &ReleaseToAIRequest{
			SessionID: req.SessionID,
			AgentID:   req.AgentID,
		})
	default:
		return fmt.Errorf("invalid handler_type: %s", req.HandlerType)
	}
}

// LockHumanSession 锁定会话为人工接管（暴露给 controller，转接/投诉升级时调用）
func (s *CustomerSessionService) LockHumanSession(ctx context.Context, sessionID, reason string) error {
	return s.lockHumanSession(ctx, sessionID, reason)
}

// UnlockHumanSession 解锁人工接管锁
func (s *CustomerSessionService) UnlockHumanSession(ctx context.Context, sessionID string) error {
	return s.unlockHumanSession(ctx, sessionID)
}

// lockHumanSession 内部：写 Redis 人工接管锁
func (s *CustomerSessionService) lockHumanSession(ctx context.Context, sessionID, reason string) error {
	svc := NewInboxIngressService(nil, nil)
	return svc.LockSessionForHuman(ctx, sessionID, reason)
}

// unlockHumanSession 内部：解 Redis 人工接管锁
func (s *CustomerSessionService) unlockHumanSession(ctx context.Context, sessionID string) error {
	svc := NewInboxIngressService(nil, nil)
	return svc.UnlockSessionForHuman(ctx, sessionID)
}

// notifySessionSessionUpdate 内部：推送会话状态变更给前端
func (s *CustomerSessionService) notifySessionUpdate(session *model.CustomerSession, event, handler string) error {
	agentID := strconv.FormatUint(uint64(session.AgentID), 10)
	return websocket.NotifySessionUpdate(agentID, map[string]any{
		"session_id":   session.SessionID,
		"handler_type": handler,
		"event":        event,
		"status":       session.Status,
		"updated_at":   time.Now().Unix(),
	})
}

// ============================================================================
// 方向10：拉黑 / 解除拉黑
// 文档：docs/企业级架构优化/坐席实时聊天看板.md §四
// ============================================================================

// BlacklistRequest 拉黑请求
type BlacklistRequest struct {
	SessionID    uint   `json:"session_id" binding:"required"`
	Reason       string `json:"reason"`        // 拉黑原因
	OperatorID   uint   `json:"operator_id"`   // 操作人（坐席 ID）
	OperatorName string `json:"operator_name"` // 操作人姓名
	TTLHours     int    `json:"ttl_hours"`     // 0 = 永久
}

// BlacklistUser 拉黑当前会话对应的访客（user_id 维度）
//
// 行为：
//  1. 会话必须存在且有 user_id，否则拒绝。
//  2. 幂等：若该 user_id+platform 已存在 active 记录，更新 reason/expires_at。
//  3. 拉黑后立即关闭该会话（status=closed）以防继续对话。
//  4. 推 WebSocket 给前端（type=handler_changed, event=blacklisted）。
func (s *CustomerSessionService) BlacklistUser(req *BlacklistRequest) error {
	if req.SessionID == 0 {
		return errors.New("session_id required")
	}
	session, err := s.sessionRepo.GetByID(req.SessionID)
	if err != nil {
		return errors.New("会话不存在")
	}
	if session.UserID == "" {
		return errors.New("该会话无 user_id，无法拉黑")
	}

	var expiresAt *time.Time
	if req.TTLHours > 0 {
		t := time.Now().Add(time.Duration(req.TTLHours) * time.Hour)
		expiresAt = &t
	}

	bl := &model.UserBlacklist{
		UserID:       session.UserID,
		Platform:     session.Platform,
		Reason:       req.Reason,
		Source:       "manual",
		OperatorID:   req.OperatorID,
		OperatorName: req.OperatorName,
		SessionID:    session.SessionID,
		Active:       true,
		ExpiresAt:    expiresAt,
	}
	if err := s.blacklistRepo.Add(bl); err != nil {
		return err
	}

	// 关闭该会话，避免继续对话
	_ = s.sessionRepo.UpdateStatus(req.SessionID, model.SessionStatusClosed)

	// 通知前端：handler_changed + blacklisted
	_ = s.notifySessionUpdate(session, "blacklisted", "human")
	_ = websocket.SendToVisitor(websocket.TypeAgentJoined, map[string]any{
		"session_id":  session.SessionID,
		"handler":     "human",
		"reason":      "因违反服务条款，该访客已被加入黑名单",
		"blacklisted": true,
	}, session.SessionID)

	return nil
}

// UnblacklistUser 解除拉黑
func (s *CustomerSessionService) UnblacklistUser(userID string, platform model.Platform) error {
	if userID == "" {
		return errors.New("user_id required")
	}
	return s.blacklistRepo.Remove(userID, platform)
}

// IsUserBlacklisted 判断访客是否在黑名单
func (s *CustomerSessionService) IsUserBlacklisted(userID string, platform model.Platform) (bool, error) {
	return s.blacklistRepo.IsBlacklisted(userID, platform)
}

// ListBlacklist 分页查询生效中的黑名单
func (s *CustomerSessionService) ListBlacklist(page, pageSize int) ([]*model.UserBlacklist, int64, error) {
	return s.blacklistRepo.ListActive(page, pageSize)
}

// AgentStatusService 客服状态服务
type AgentStatusService struct {
	agentRepo *repository.AgentStatusRepository
}

// NewAgentStatusService 创建客服状态服务实例
func NewAgentStatusService() *AgentStatusService {
	return &AgentStatusService{
		agentRepo: repository.NewAgentStatusRepository(),
	}
}

// CreateAgentRequest 创建客服请求
type CreateAgentRequest struct {
	AgentID     uint   `json:"agent_id" binding:"required"`
	AgentName   string `json:"agent_name" binding:"required"`
	MaxSessions int    `json:"max_sessions"`
}

// CreateAgent 创建客服状态记录
func (s *AgentStatusService) CreateAgent(req *CreateAgentRequest) (*model.AgentStatus, error) {
	agent := &model.AgentStatus{
		AgentID:     req.AgentID,
		AgentName:   req.AgentName,
		Status:      "offline",
		MaxSessions: req.MaxSessions,
	}
	if agent.MaxSessions == 0 {
		agent.MaxSessions = 5
	}

	if err := s.agentRepo.Create(agent); err != nil {
		return nil, err
	}

	return agent, nil
}

// GetAgentStatus 获取客服状态
func (s *AgentStatusService) GetAgentStatus(agentID uint) (*model.AgentStatus, error) {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return nil, err
	}
	return agent, nil
}

// GetOnlineAgents 获取在线客服列表
func (s *AgentStatusService) GetOnlineAgents() ([]*model.AgentStatus, error) {
	return s.agentRepo.GetOnlineAgents()
}

// ListAllAgents 列出全部客服（不分在线/离线），用于客服监管控制台
func (s *AgentStatusService) ListAllAgents() ([]*model.AgentStatus, error) {
	return s.agentRepo.ListAllAgents()
}

// UpdateAgentStatus 更新客服状态
func (s *AgentStatusService) UpdateAgentStatus(agentID uint, status string) error {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return err
	}
	_ = agent

	// 检查状态变更合法性
	if agent.Status == "offline" && status != "online" {
		return errors.New("客服离线时只能切换到在线状态")
	}

	return s.agentRepo.UpdateStatus(agentID, status)
}

// GoOnline 客服上线
func (s *AgentStatusService) GoOnline(agentID uint) error {
	return s.UpdateAgentStatus(agentID, "online")
}

// GoOffline 客服下线
func (s *AgentStatusService) GoOffline(agentID uint) error {
	agent, err := s.agentRepo.GetByAgentID(agentID)
	if err != nil {
		return err
	}
	_ = agent

	// 检查是否有未完成的会话
	if agent.ActiveSessions > 0 {
		return errors.New("还有未完成的会话，请先处理或转接")
	}

	return s.agentRepo.UpdateStatus(agentID, "offline")
}

// GetAgentSessions 获取客服的活跃会话
func (s *AgentStatusService) GetAgentSessions(agentID uint) ([]*model.CustomerSession, error) {
	sessionRepo := repository.NewCustomerSessionRepository()
	return sessionRepo.GetAgentSessions(agentID)
}

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
func (s *AISuggestionService) CreateSuggestion(sessionID string, messageID uint, suggestion string, confidence float64, source string) (*model.AISuggestion, error) {
	ais := &model.AISuggestion{
		SessionID:  sessionID,
		MessageID:  messageID,
		Suggestion: suggestion,
		Confidence: confidence,
		Source:     source,
	}

	if err := s.suggestionRepo.Create(ais); err != nil {
		return nil, err
	}

	// 通知客服
	session, _ := s.sessionRepo.GetBySessionID(sessionID)
	if session != nil && session.AgentID > 0 {
		websocket.NotifyAISuggestion(strconv.FormatUint(uint64(session.AgentID), 10), ais)
	}

	return ais, nil
}

// GetSuggestions 获取会话的AI建议
func (s *AISuggestionService) GetSuggestions(sessionID string) ([]*model.AISuggestion, error) {
	return s.suggestionRepo.GetBySessionID(sessionID)
}

// UseSuggestion 使用AI建议
func (s *AISuggestionService) UseSuggestion(id uint, agentID uint) error {
	return s.suggestionRepo.MarkAsUsed(id, agentID)
}

// QuickReplyService 快捷回复服务
type QuickReplyService struct {
	replyRepo *repository.QuickReplyRepository
}

// NewQuickReplyService 创建快捷回复服务实例
func NewQuickReplyService() *QuickReplyService {
	return &QuickReplyService{
		replyRepo: repository.NewQuickReplyRepository(),
	}
}

// CreateReplyRequest 创建快捷回复请求
type CreateReplyRequest struct {
	Category  string `json:"category" binding:"required"`
	Title     string `json:"title" binding:"required"`
	Content   string `json:"content" binding:"required"`
	Channel   string `json:"channel"`
	SortOrder int    `json:"sort_order"`
	IsPublic  bool   `json:"is_public"`
}

// CreateReply 创建快捷回复
func (s *QuickReplyService) CreateReply(createdBy uint, req *CreateReplyRequest) (*model.QuickReply, error) {
	reply := &model.QuickReply{
		Category:  req.Category,
		Title:     req.Title,
		Content:   req.Content,
		Channel:   req.Channel,
		SortOrder: req.SortOrder,
		IsPublic:  req.IsPublic,
		CreatedBy: createdBy,
	}
	if !reply.IsPublic {
		reply.IsPublic = true // 默认公开
	}

	if err := s.replyRepo.Create(reply); err != nil {
		return nil, err
	}

	return reply, nil
}

// UpdateReply 更新快捷回复
func (s *QuickReplyService) UpdateReply(id uint, req *CreateReplyRequest) (*model.QuickReply, error) {
	reply, err := s.replyRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	reply.Category = req.Category
	reply.Title = req.Title
	reply.Content = req.Content
	reply.Channel = req.Channel
	reply.SortOrder = req.SortOrder
	reply.IsPublic = req.IsPublic

	if err := s.replyRepo.Update(reply); err != nil {
		return nil, err
	}

	return reply, nil
}

// DeleteReply 删除快捷回复
func (s *QuickReplyService) DeleteReply(id uint) error {
	reply, err := s.replyRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = reply

	return s.replyRepo.Delete(id)
}

// GetReplies 获取快捷回复列表
func (s *QuickReplyService) GetReplies(category string) ([]*model.QuickReply, error) {
	return s.replyRepo.GetByMerchant(category)
}

// GetCategories 获取快捷回复分类
func (s *QuickReplyService) GetCategories() ([]string, error) {
	return s.replyRepo.GetCategories()
}

// SessionTagService 会话标签服务
type SessionTagService struct {
	tagRepo *repository.SessionTagRepository
}

// NewSessionTagService 创建会话标签服务实例
func NewSessionTagService() *SessionTagService {
	return &SessionTagService{
		tagRepo: repository.NewSessionTagRepository(),
	}
}

// CreateTagRequest 创建标签请求
type CreateTagRequest struct {
	Name        string `json:"name" binding:"required"`
	Code        string `json:"code" binding:"required"`
	Group       string `json:"group"`
	Color       string `json:"color"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

// CreateTag 创建标签
func (s *SessionTagService) CreateTag(req *CreateTagRequest) (*model.SessionTag, error) {
	tag := &model.SessionTag{
		Name:        req.Name,
		Code:        req.Code,
		Group:       req.Group,
		Color:       req.Color,
		Description: req.Description,
		SortOrder:   req.SortOrder,
	}
	if tag.Color == "" {
		tag.Color = "#1890ff"
	}

	if err := s.tagRepo.Create(tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// UpdateTag 更新标签
func (s *SessionTagService) UpdateTag(id uint, req *CreateTagRequest) (*model.SessionTag, error) {
	tag, err := s.tagRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	tag.Name = req.Name
	tag.Code = req.Code
	tag.Group = req.Group
	tag.Color = req.Color
	tag.Description = req.Description
	tag.SortOrder = req.SortOrder

	if err := s.tagRepo.Update(tag); err != nil {
		return nil, err
	}

	return tag, nil
}

// DeleteTag 删除标签
func (s *SessionTagService) DeleteTag(id uint) error {
	tag, err := s.tagRepo.GetByID(id)
	if err != nil {
		return err
	}
	_ = tag

	return s.tagRepo.Delete(id)
}

// GetTags 获取标签列表
func (s *SessionTagService) GetTags() ([]*model.SessionTag, error) {
	return s.tagRepo.GetByMerchant()
}

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), uuid.New().String()[:8])
}
