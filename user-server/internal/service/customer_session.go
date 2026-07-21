package service

import (
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
}

// NewCustomerSessionService 创建客服会话服务实例
func NewCustomerSessionService() *CustomerSessionService {
	return &CustomerSessionService{
		sessionRepo:    repository.NewCustomerSessionRepository(),
		messageRepo:    repository.NewSessionMessageRepository(),
		agentRepo:      repository.NewAgentStatusRepository(),
		suggestionRepo: repository.NewAISuggestionRepository(),
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
func (s *CustomerSessionService) CreateSession(req *CreateSessionRequest) (*model.CustomerSession, error) {
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
