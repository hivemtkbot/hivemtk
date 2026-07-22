package repository

import (
	"context"
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
func (r *CustomerSessionRepository) GetDB(ctx context.Context) *gorm.DB {
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
func (r *CustomerSessionRepository) Create(ctx context.Context, session *model.CustomerSession) error {
	return r.db.Create(session).Error
}

// Update 更新会话
func (r *CustomerSessionRepository) Update(ctx context.Context, session *model.CustomerSession) error {
	return r.db.Save(session).Error
}

// GetByID 根据ID获取会话
func (r *CustomerSessionRepository) GetByID(ctx context.Context, id uint) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.First(&session, id).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetBySessionID 根据SessionID获取会话
func (r *CustomerSessionRepository) GetBySessionID(ctx context.Context, sessionID string) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.Where("session_id = ?", sessionID).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *CustomerSessionRepository) GetByMerchant(ctx context.Context, status model.SessionStatus, page, pageSize int) ([]*model.CustomerSession, int64, error) {
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
func (r *CustomerSessionRepository) GetPendingSessions(ctx context.Context) ([]*model.CustomerSession, error) {
	var sessions []*model.CustomerSession
	err := r.db.Where("status IN ?", []model.SessionStatus{
		model.SessionStatusPending,
		model.SessionStatusAIHandling,
	}).Order("priority DESC, last_message_at ASC").Find(&sessions).Error
	return sessions, err
}

// GetAgentSessions 获取客服的活跃会话
func (r *CustomerSessionRepository) GetAgentSessions(ctx context.Context, agentID uint) ([]*model.CustomerSession, error) {
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
func (r *CustomerSessionRepository) GetActiveByUserID(ctx context.Context, userID string) (*model.CustomerSession, error) {
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
func (r *CustomerSessionRepository) UpdateStatus(ctx context.Context, id uint, status model.SessionStatus) error {
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
func (r *CustomerSessionRepository) AssignAgent(ctx context.Context, id uint, agentID uint, agentName string) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).Updates(map[string]any{
		"agent_id":     agentID,
		"agent_name":   agentName,
		"handler_type": model.HandlerTypeHuman,
		"status":       model.SessionStatusHumanHandling,
	}).Error
}

// UpdateLastMessage 更新最后消息
func (r *CustomerSessionRepository) UpdateLastMessage(ctx context.Context, id uint, message string, messageBy string) error {
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
func (r *CustomerSessionRepository) UpdateLastMessageBySessionID(ctx context.Context, sessionID string, message string, messageBy string) error {
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
func (r *CustomerSessionRepository) IncrementAIReplyCount(ctx context.Context, id uint) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).
		Update("ai_reply_count", gorm.Expr("ai_reply_count + 1")).Error
}

// IncrementHumanReplyCount 增加人工回复计数
func (r *CustomerSessionRepository) IncrementHumanReplyCount(ctx context.Context, id uint) error {
	return r.db.Model(&model.CustomerSession{}).Where("id = ?", id).
		Update("human_reply_count", gorm.Expr("human_reply_count + 1")).Error
}
