package repository

import (
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// CustomerSessionRepository 客服会话仓库
type CustomerSessionRepository struct {
	db *gorm.DB
}

// GetDB 返回数据库连接
func (r *CustomerSessionRepository) GetDB() *gorm.DB {
	return r.db
}

// NewCustomerSessionRepository 创建客服会话仓库实例
func NewCustomerSessionRepository() *CustomerSessionRepository {
	return &CustomerSessionRepository{
		db: _db.GetDB(),
	}
}

// NewCustomerSessionRepositoryWithDB 创建指定数据库连接的 CustomerSessionRepository 实例（用于测试）
func NewCustomerSessionRepositoryWithDB(db *gorm.DB) *CustomerSessionRepository {
	return &CustomerSessionRepository{
		db: db,
	}
}

// Create 创建会话
func (r *CustomerSessionRepository) Create(session *model.CustomerSession) error {
	return r.db.Create(session).Error
}

// Update 更新会话
func (r *CustomerSessionRepository) Update(session *model.CustomerSession) error {
	return r.db.Save(session).Error
}

// GetByID 根据ID获取会话
func (r *CustomerSessionRepository) GetByID(id uint) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.First(&session, id).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetBySessionID 根据SessionID获取会话
func (r *CustomerSessionRepository) GetBySessionID(sessionID string) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *CustomerSessionRepository) GetByMerchant(status model.SessionStatus, page, pageSize int) ([]*model.CustomerSession, int64, error) {
	var sessions []*model.CustomerSession
	var total int64

	query := r.db.Model(&model.CustomerSession{}).Where("1 = 1")
	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Order("last_message_at DESC").Offset(offset).Limit(pageSize).Find(&sessions).Error
	return sessions, total, err
}

// GetPendingSessions 获取等待处理的会话
func (r *CustomerSessionRepository) GetPendingSessions() ([]*model.CustomerSession, error) {
	var sessions []*model.CustomerSession
	err := r.db.Where("status IN ?", []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
	}).Order("priority DESC, last_message_at ASC").Find(&sessions).Error
	return sessions, err
}

// GetAgentSessions 获取客服的活跃会话
func (r *CustomerSessionRepository) GetAgentSessions(agentID uint) ([]*model.CustomerSession, error) {
	var sessions []*model.CustomerSession
	err := r.db.Where("agent_id = ? AND status IN ?", agentID, []model.SessionStatus{
		model.SessionStatusHumanHandling,
		model.SessionStatusWaiting,
	}).Order("last_message_at DESC").Find(&sessions).Error
	return sessions, err
}

// GetActiveByUserID 根据用户 ID 获取其当前活跃会话（用于营销流程 assign_agent 动作）。
// 活跃状态：pending / ai_handling / waiting / human_handling。
// 若存在多条，返回最近一条有消息记录的会话。
func (r *CustomerSessionRepository) GetActiveByUserID(userID string) (*model.CustomerSession, error) {
	if userID == "" {
		return nil, errors.New("user_id 不能为空")
	}
	var session model.CustomerSession
	err := r.db.Where("user_id = ? AND status IN ?", userID, []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
		model.SessionStatusWaiting,
		model.SessionStatusHumanHandling,
	}).Order("last_message_at DESC, id DESC").First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// UpdateStatus 更新会话状态
func (r *CustomerSessionRepository) UpdateStatus(id uint, status model.SessionStatus) error {
	updates := map[string]any{"status": status}
	if status == model.SessionStatusResolved {
		now := time.Now()
		updates["resolved_at"] = &now
	} else if status == model.SessionStatusClosed {
		now := time.Now()
		updates["closed_at"] = &now
	}
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).Updates(updates).Error
}

// AssignAgent 分配客服
func (r *CustomerSessionRepository) AssignAgent(id uint, agentID uint, agentName string) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).Updates(map[string]any{
		"agent_id":     agentID,
		"agent_name":   agentName,
		"handler_type": model.HandlerTypeHuman,
		"status":       model.SessionStatusHumanHandling,
	}).Error
}

// UpdateLastMessage 更新最后消息
func (r *CustomerSessionRepository) UpdateLastMessage(id uint, message string, messageBy string) error {
	now := time.Now()
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).Updates(map[string]any{
		"last_message":    message,
		"last_message_at": &now,
		"last_message_by": messageBy,
	}).Update("message_count", gorm.Expr("message_count + 1")).Error
}

// UpdateLastMessageBySessionID 按 session_id 更新最后消息
//
// SOP 节点执行器只知道 session_id（业务字段），不知道 DB 主键 id，
// 此方法封装按 session_id 更新操作，便于执行器直接调用。
// 失败不阻塞 SOP 流程（best-effort 语义）。
func (r *CustomerSessionRepository) UpdateLastMessageBySessionID(sessionID string, message string, messageBy string) error {
	if sessionID == "" {
		return errors.New("session_id is empty")
	}
	now := time.Now()
	return r.db.Model(&model.CustomerSession{}).Where("session_id = ?", sessionID).Updates(map[string]any{
		"last_message":    message,
		"last_message_at": &now,
		"last_message_by": messageBy,
	}).Update("message_count", gorm.Expr("message_count + 1")).Error
}

// IncrementAIReplyCount 增加AI回复计数
func (r *CustomerSessionRepository) IncrementAIReplyCount(id uint) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).
		Update("ai_reply_count", gorm.Expr("ai_reply_count + 1")).Error
}

// IncrementHumanReplyCount 增加人工回复计数
func (r *CustomerSessionRepository) IncrementHumanReplyCount(id uint) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).
		Update("human_reply_count", gorm.Expr("human_reply_count + 1")).Error
}

// SessionMessageRepository 会话消息仓库
type SessionMessageRepository struct {
	db *gorm.DB
}

// NewSessionMessageRepository 创建会话消息仓库实例
func NewSessionMessageRepository() *SessionMessageRepository {
	return &SessionMessageRepository{
		db: _db.GetDB(),
	}
}

// NewSessionMessageRepositoryWithDB 创建指定数据库连接的 SessionMessageRepository 实例（用于测试）
func NewSessionMessageRepositoryWithDB(db *gorm.DB) *SessionMessageRepository {
	return &SessionMessageRepository{
		db: db,
	}
}

// Create 创建消息
func (r *SessionMessageRepository) Create(message *model.SessionMessage) error {
	return r.db.Create(message).Error
}

// FindRecentDuplicate 查找最近 N 秒内同 (session, content, sender_type, sender_id) 的消息
// 用于：visitor 端 + orchestrator 双保存的去重（2026-07-17 修复 chat 双发 bug）
// 返回找到的最近一条消息（如果存在），否则返回 nil
func (r *SessionMessageRepository) FindRecentDuplicate(sessionID, senderType, senderID, content string, window time.Duration) (*model.SessionMessage, error) {
	var existing model.SessionMessage
	threshold := time.Now().Add(-window)
	err := r.db.Where("session_id = ? AND sender_type = ? AND sender_id = ? AND content = ? AND created_at >= ?",
		sessionID, senderType, senderID, content, threshold).
		Order("id DESC").
		First(&existing).Error
	if err != nil {
		return nil, err
	}
	return &existing, nil
}

// GetBySessionID 获取会话的消息列表
func (r *SessionMessageRepository) GetBySessionID(sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
	var messages []*model.SessionMessage
	var total int64

	query := r.db.Model(&model.SessionMessage{}).Where("session_id = ?", sessionID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Order("created_at ASC").Offset(offset).Limit(pageSize).Find(&messages).Error
	return messages, total, err
}

// MarkAsRead 标记消息已读
func (r *SessionMessageRepository) MarkAsRead(sessionID string, beforeTime time.Time) error {
	now := time.Now()
	return r.db.Model(&model.SessionMessage{}).
		Where("session_id = ? AND is_read = ? AND created_at <= ?", sessionID, false, beforeTime).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		}).Error
}

// GetUnreadCount 获取未读消息数
func (r *SessionMessageRepository) GetUnreadCount(sessionID string, senderType string) int64 {
	var count int64
	r.db.Model(&model.SessionMessage{}).
		Where("session_id = ? AND sender_type = ? AND is_read = ?", sessionID, senderType, false).
		Count(&count)
	return count
}

// AgentStatusRepository 客服状态仓库
type AgentStatusRepository struct {
	db *gorm.DB
}

// NewAgentStatusRepository 创建客服状态仓库实例
func NewAgentStatusRepository() *AgentStatusRepository {
	return &AgentStatusRepository{
		db: _db.GetDB(),
	}
}

// NewAgentStatusRepositoryWithDB 创建指定数据库连接的客服状态仓库实例
func NewAgentStatusRepositoryWithDB(db *gorm.DB) *AgentStatusRepository {
	return &AgentStatusRepository{db: db}
}

// Create 创建客服状态
func (r *AgentStatusRepository) Create(status *model.AgentStatus) error {
	return r.db.Create(status).Error
}

// Update 更新客服状态
func (r *AgentStatusRepository) Update(status *model.AgentStatus) error {
	return r.db.Save(status).Error
}

// GetByAgentID 根据客服ID获取状态
func (r *AgentStatusRepository) GetByAgentID(agentID uint) (*model.AgentStatus, error) {
	var status model.AgentStatus
	err := r.db.Where("agent_id = ?", agentID).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// GetOnlineAgents 获取在线客服列表
func (r *AgentStatusRepository) GetOnlineAgents() ([]*model.AgentStatus, error) {
	var agents []*model.AgentStatus
	err := r.db.Where("status IN ? AND active_sessions < max_sessions", []string{"online", "busy"}).
		Order("active_sessions ASC").Find(&agents).Error
	return agents, err
}

// ListAllAgents 列出全部客服（不分在线/离线），用于客服监管控制台
func (r *AgentStatusRepository) ListAllAgents() ([]*model.AgentStatus, error) {
	var agents []*model.AgentStatus
	err := r.db.Order("agent_id ASC").Find(&agents).Error
	return agents, err
}

// UpdateStatus 更新客服状态
func (r *AgentStatusRepository) UpdateStatus(agentID uint, status string) error {
	updates := map[string]any{"status": status}
	now := time.Now()
	if status == "online" {
		updates["online_at"] = &now
	} else if status == "offline" {
		updates["offline_at"] = &now
	}
	updates["last_active_at"] = &now
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).Updates(updates).Error
}

// IncrementActiveSessions 增加活跃会话数
func (r *AgentStatusRepository) IncrementActiveSessions(agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).
		Updates(map[string]any{
			"active_sessions": gorm.Expr("active_sessions + 1"),
			"today_sessions":  gorm.Expr("today_sessions + 1"),
		}).Error
}

// DecrementActiveSessions 减少活跃会话数
func (r *AgentStatusRepository) DecrementActiveSessions(agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ? AND active_sessions > 0", agentID).
		Update("active_sessions", gorm.Expr("active_sessions - 1")).Error
}

// IncrementTodayMessages 增加今日消息数
func (r *AgentStatusRepository) IncrementTodayMessages(agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).
		Update("today_messages", gorm.Expr("today_messages + 1")).Error
}

// AISuggestionRepository AI建议仓库
type AISuggestionRepository struct {
	db *gorm.DB
}

// NewAISuggestionRepository 创建AI建议仓库实例
func NewAISuggestionRepository() *AISuggestionRepository {
	return &AISuggestionRepository{
		db: _db.GetDB(),
	}
}

// NewAISuggestionRepositoryWithDB 创建指定数据库连接AI建议仓库实例
func NewAISuggestionRepositoryWithDB(db *gorm.DB) *AISuggestionRepository {
	return &AISuggestionRepository{db: db}
}

// Create 创建AI建议
func (r *AISuggestionRepository) Create(suggestion *model.AISuggestion) error {
	return r.db.Create(suggestion).Error
}

// GetBySessionID 获取会话的AI建议列表
func (r *AISuggestionRepository) GetBySessionID(sessionID string) ([]*model.AISuggestion, error) {
	var suggestions []*model.AISuggestion
	err := r.db.Where("session_id = ?", sessionID).Order("created_at DESC").Limit(10).Find(&suggestions).Error
	return suggestions, err
}

// MarkAsUsed 标记建议已使用
func (r *AISuggestionRepository) MarkAsUsed(id uint, agentID uint) error {
	now := time.Now()
	return r.db.Model(&model.AISuggestion{}).Where("id = ?", id).Updates(map[string]any{
		"is_used": true,
		"used_by": agentID,
		"used_at": &now,
	}).Error
}

// QuickReplyRepository 快捷回复仓库
type QuickReplyRepository struct {
	db *gorm.DB
}

// NewQuickReplyRepository 创建快捷回复仓库实例
func NewQuickReplyRepository() *QuickReplyRepository {
	return &QuickReplyRepository{
		db: _db.GetDB(),
	}
}

// Create 创建快捷回复
func (r *QuickReplyRepository) Create(reply *model.QuickReply) error {
	return r.db.Create(reply).Error
}

// Update 更新快捷回复
func (r *QuickReplyRepository) Update(reply *model.QuickReply) error {
	return r.db.Save(reply).Error
}

// Delete 删除快捷回复
func (r *QuickReplyRepository) Delete(id uint) error {
	return r.db.Delete(&model.QuickReply{}, id).Error
}

// GetByID 根据ID获取快捷回复
func (r *QuickReplyRepository) GetByID(id uint) (*model.QuickReply, error) {
	var reply model.QuickReply
	err := r.db.First(&reply, id).Error
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (r *QuickReplyRepository) GetByMerchant(category string) ([]*model.QuickReply, error) {
	var replies []*model.QuickReply
	query := r.db.Where("is_public = ?", true)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	err := query.Order("sort_order ASC").Find(&replies).Error
	return replies, err
}

// GetCategories 获取快捷回复分类列表
func (r *QuickReplyRepository) GetCategories() ([]string, error) {
	var categories []string
	err := r.db.Model(&model.QuickReply{}).
		Where("is_public = ?", true).
		Distinct("category").Pluck("category", &categories).Error
	return categories, err
}

// SessionTagRepository 会话标签仓库
type SessionTagRepository struct {
	db *gorm.DB
}

// NewSessionTagRepository 创建会话标签仓库实例
func NewSessionTagRepository() *SessionTagRepository {
	return &SessionTagRepository{
		db: _db.GetDB(),
	}
}

// Create 创建标签
func (r *SessionTagRepository) Create(tag *model.SessionTag) error {
	return r.db.Create(tag).Error
}

// Update 更新标签
func (r *SessionTagRepository) Update(tag *model.SessionTag) error {
	return r.db.Save(tag).Error
}

// Delete 删除标签
func (r *SessionTagRepository) Delete(id uint) error {
	return r.db.Delete(&model.SessionTag{}, id).Error
}

func (r *SessionTagRepository) GetByMerchant() ([]*model.SessionTag, error) {
	var tags []*model.SessionTag
	err := r.db.Where("1 = 1").Order("sort_order ASC").Find(&tags).Error
	return tags, err
}

// GetByID 根据ID获取标签
func (r *SessionTagRepository) GetByID(id uint) (*model.SessionTag, error) {
	var tag model.SessionTag
	err := r.db.First(&tag, id).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}
