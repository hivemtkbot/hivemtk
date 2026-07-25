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

// GetByUserID 获取某用户的所有会话（CC-P2 N+1 优化）
//
// 替代 service 层「GetByMerchant(空 merchant, 1, 1000) + 内存过滤」模式，
// 直接按 user_id 走索引，单 SQL 拉全该用户会话。
func (r *CustomerSessionRepository) GetByUserID(ctx context.Context, userID string) ([]*model.CustomerSession, error) {
	if userID == "" {
		return nil, nil
	}
	var sessions []*model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// ListByUserIDsBatch 批量按 user_id 拉取会话，返回按 user_id 分组的 map（CC-P2 N+1 优化）
//
// 用于「客户列表 → 每用户 360 视图」场景：单次 SQL 拉所有用户的会话，
// 避免 N 个用户各跑一次 GetByUserID 造成的 N+1。
//   - userIDs: 待查询的 user id 列表，空时返回 (empty map, nil)
//   - 入参去重 + 跳过空串
//
// 返回值：map[userID][]*CustomerSession，未命中的 userID 不会出现在 map 中。
func (r *CustomerSessionRepository) ListByUserIDsBatch(ctx context.Context, userIDs []string) (map[string][]*model.CustomerSession, error) {
	result := make(map[string][]*model.CustomerSession, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	unique := make([]string, 0, len(userIDs))
	seen := make(map[string]struct{}, len(userIDs))
	for _, id := range userIDs {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 {
		return result, nil
	}
	var sessions []*model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("user_id IN ?", unique).
		Order("user_id ASC, created_at ASC").
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	for _, s := range sessions {
		result[s.UserID] = append(result[s.UserID], s)
	}
	return result, nil
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

// UpdateHandlerType 更新会话处理方类型（ai / human）
//
// 用于关键词触发自动转人工但 AutoAssign 失败的场景：
// 仅修改 handler_type，不动 status（由调用方另行控制）。
func (r *CustomerSessionRepository) UpdateHandlerType(ctx context.Context, id uint, handlerType model.HandlerType) error {
	return r.db.WithContext(ctx).Model(&model.CustomerSession{}).
		Where("id = ?", id).
		Update("handler_type", handlerType).Error
}

// GetBySessionIDPlatformAccountUser 按 (session_id, platform, account_id, user_id) 查询会话
//
// 用于访客侧按 session_id 拉取会话时校验归属：
// 仅当 4 元组完全匹配才返回，避免越权访问。
// 未命中返回 (nil, gorm.ErrRecordNotFound)（与 service 层历史 errors.Is 检查兼容）。
func (r *CustomerSessionRepository) GetBySessionIDPlatformAccountUser(
	ctx context.Context, sessionID string, platform model.Platform,
	accountID, userID string,
) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("session_id = ? AND platform = ? AND account_id = ? AND user_id = ?",
			sessionID, platform, accountID, userID).
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// GetLatestActiveByPlatformAccountUser 拉取最近一条未结束会话
//
// 用于访客 OpenSession(Resume=true) 与离线消息续接：
//   - 按 (platform, account_id, user_id) 过滤
//   - 排除已结束状态（excludeStatuses，通常为 [resolved, closed]）
//   - 按 last_message_at DESC NULLS LAST, created_at DESC 排序，取首条
//
// 未命中返回 (nil, gorm.ErrRecordNotFound)（与历史行为一致）。
func (r *CustomerSessionRepository) GetLatestActiveByPlatformAccountUser(
	ctx context.Context, platform model.Platform,
	accountID, userID string, excludeStatuses []model.SessionStatus,
) (*model.CustomerSession, error) {
	var session model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND user_id = ?", platform, accountID, userID).
		Where("status NOT IN ?", excludeStatuses).
		Order("last_message_at DESC NULLS LAST, created_at DESC").
		First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// ListRecentClosedByPlatformAccountUser 拉取最近 N 天内已结束会话
//
// 用于访客离线消息列表展示：
//   - 按 (platform, account_id, user_id) 过滤
//   - status IN includeStatuses（通常为 [resolved, closed]）
//   - created_at > since
//   - 按 created_at DESC 排序，limit 限制条数
func (r *CustomerSessionRepository) ListRecentClosedByPlatformAccountUser(
	ctx context.Context, platform model.Platform,
	accountID, userID string, includeStatuses []model.SessionStatus,
	since time.Time, limit int,
) ([]*model.CustomerSession, error) {
	var sessions []*model.CustomerSession
	err := r.db.WithContext(ctx).
		Where("platform = ? AND account_id = ? AND user_id = ?", platform, accountID, userID).
		Where("status IN ?", includeStatuses).
		Where("created_at > ?", since).
		Order("created_at DESC").
		Limit(limit).
		Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}
