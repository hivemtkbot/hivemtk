package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"marketing/internal/model"
	"marketing/internal/repository"
	"marketing/internal/websocket"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ============================================================================
// 客服会话服务（customer_session.go - 核心 CRUD）
// ----------------------------------------------------------------------------
// 文件拆分（2026-07-22 方向C）：
//   - customer_session.go            本文件：核心 CRUD（CreateSession / Get / SendMessage / UpdateStatus / Rate / Tag）
//   - customer_session_routing.go    路由：AssignSession / AutoAssign / TransferSession
//   - customer_session_takeover.go   接管：TakeoverByAgent / ReleaseToAI / SwitchHandler
//   - customer_session_blacklist.go  拉黑：BlacklistUser / UnblacklistUser / IsUserBlacklisted / preCreateBlacklistGuard
//   - agent_status.go                客服状态：AgentStatusService
//   - ai_suggestion.go               AI 建议：AISuggestionService
//   - quick_reply.go                 快捷回复：QuickReplyService
//   - session_tag.go                 会话标签：SessionTagService
//
// 文档：docs/企业级架构优化/坐席实时聊天看板.md
// ============================================================================

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
//
// 具体校验逻辑见 customer_session_blacklist.go 中的 preCreateBlacklistGuard。
func (s *CustomerSessionService) CreateSession(ctx context.Context, req *CreateSessionRequest) (*model.CustomerSession, error) {
	if err := s.preCreateBlacklistGuard(ctx, req); err != nil {
		return nil, err
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

	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, err
	}

	return session, nil
}

// GetSessions 获取会话列表
func (s *CustomerSessionService) GetSessions(ctx context.Context, status model.SessionStatus, page, pageSize int) ([]*model.CustomerSession, int64, error) {
	return s.sessionRepo.GetByMerchant(ctx, status, page, pageSize)
}

// GetPendingSessions 获取待处理会话（pending / AI handling / waiting）
func (s *CustomerSessionService) GetPendingSessions(ctx context.Context, page, pageSize int) ([]*model.CustomerSession, error) {
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
	if err := s.sessionRepo.GetDB(ctx).Where("status IN ?", statuses).
		Order("priority DESC, COALESCE(last_message_at, created_at) ASC").
		Offset(offset).Limit(pageSize).Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// CountPendingSessions 统计待处理会话数
func (s *CustomerSessionService) CountPendingSessions(ctx context.Context) (int64, error) {
	statuses := []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
		model.SessionStatusWaiting,
	}
	var count int64
	if err := s.sessionRepo.GetDB(ctx).Model(&model.CustomerSession{}).
		Where("status IN ?", statuses).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetSessionByID 获取会话详情
func (s *CustomerSessionService) GetSessionByID(ctx context.Context, id uint) (*model.CustomerSession, error) {
	session, err := s.sessionRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return session, nil
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
func (s *CustomerSessionService) SendMessage(ctx context.Context, req *SendMessageRequest) (*model.SessionMessage, error) {
	// 获取会话
	session, err := s.sessionRepo.GetBySessionID(ctx, req.SessionID)
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

	if err := s.messageRepo.Create(ctx, message); err != nil {
		return nil, err
	}

	// 更新会话最后消息
	if err := s.sessionRepo.UpdateLastMessage(ctx, session.ID, req.Content, req.SenderType); err != nil {
		return nil, err
	}

	// 更新回复计数
	if req.SenderType == "ai" {
		s.sessionRepo.IncrementAIReplyCount(ctx, session.ID)
	} else if req.SenderType == "agent" {
		s.sessionRepo.IncrementHumanReplyCount(ctx, session.ID)
		// 更新客服消息计数
		if session.AgentID > 0 {
			s.agentRepo.IncrementTodayMessages(ctx, session.AgentID)
		}
	}

	// 通知客服新消息
	if session.AgentID > 0 && req.SenderType == "user" {
		websocket.NotifyNewMessage(strconv.FormatUint(uint64(session.AgentID), 10), message)
	}

	// 推送坐席/AI 回复给访客（实时），完成网页客服渠道闭环
	// 访客离线时 SendToVisitor 静默丢弃，依赖 WebSocket 重连后的离线消息补发
	if req.SenderType == "agent" || req.SenderType == "ai" {
		s.pushToVisitor(ctx, session.SessionID, message)
	}

	return message, nil
}

// pushToVisitor 将坐席/AI 的回复实时推送给访客端 WebSocket
// 若访客在线则标记 delivered_at，避免重连时离线补发重复展示
func (s *CustomerSessionService) pushToVisitor(ctx context.Context, sessionID string, message *model.SessionMessage) {
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
		_ = s.sessionRepo.GetDB(ctx).Model(&model.SessionMessage{}).
			Where("id = ?", message.ID).Update("delivered_at", &now).Error
	}
}

// GetMessages 获取会话消息列表
func (s *CustomerSessionService) GetMessages(ctx context.Context, sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
	// 验证会话归属
	session, err := s.sessionRepo.GetBySessionID(ctx, sessionID)
	if err != nil {
		return nil, 0, errors.New("会话不存在")
	}
	_ = session

	return s.messageRepo.GetBySessionID(ctx, sessionID, page, pageSize)
}

// UpdateSessionStatus 更新会话状态
func (s *CustomerSessionService) UpdateSessionStatus(ctx context.Context, sessionID uint, status model.SessionStatus) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}

	// 更新状态
	if err := s.sessionRepo.UpdateStatus(ctx, sessionID, status); err != nil {
		return err
	}

	// 如果是关闭会话，减少客服活跃会话数
	if status == model.SessionStatusResolved || status == model.SessionStatusClosed {
		if session.AgentID > 0 {
			s.agentRepo.DecrementActiveSessions(ctx, session.AgentID)
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
func (s *CustomerSessionService) RateSession(ctx context.Context, sessionID uint, rating int, comment string) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	_ = session

	session.Rating = rating
	session.RatingComment = comment
	return s.sessionRepo.Update(ctx, session)
}

// TagSession 标记会话
func (s *CustomerSessionService) TagSession(ctx context.Context, sessionID uint, tags []string) error {
	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return err
	}
	_ = session

	tagsJSON, _ := json.Marshal(tags)
	session.Tags = string(tagsJSON)
	return s.sessionRepo.Update(ctx, session)
}

// generateSessionID 生成会话ID
func generateSessionID() string {
	return fmt.Sprintf("sess_%d_%s", time.Now().UnixNano(), uuid.New().String()[:8])
}
