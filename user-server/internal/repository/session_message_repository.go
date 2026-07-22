package repository

import (
	"context"
	"errors"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

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
func (r *SessionMessageRepository) Create(ctx context.Context, message *model.SessionMessage) error {
	return r.db.Create(message).Error
}

// FindRecentDuplicate 查找最近 N 秒内同 (session, content, sender_type, sender_id) 的消息
// 用于：visitor 端 + orchestrator 双保存的去重（2026-07-17 修复 chat 双发 bug）
// 返回找到的最近一条消息（如果存在），否则返回 nil
func (r *SessionMessageRepository) FindRecentDuplicate(ctx context.Context, sessionID, senderType, senderID, content string, window time.Duration) (*model.SessionMessage, error) {
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
func (r *SessionMessageRepository) GetBySessionID(ctx context.Context, sessionID string, page, pageSize int) ([]*model.SessionMessage, int64, error) {
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
func (r *SessionMessageRepository) MarkAsRead(ctx context.Context, sessionID string, beforeTime time.Time) error {
	now := time.Now()
	return r.db.Model(&model.SessionMessage{}).
		Where("session_id = ? AND is_read = ? AND created_at <= ?", sessionID, false, beforeTime).
		Updates(map[string]any{
			"is_read": true,
			"read_at": &now,
		}).Error
}

// GetUnreadCount 获取未读消息数
func (r *SessionMessageRepository) GetUnreadCount(ctx context.Context, sessionID string, senderType string) int64 {
	var count int64
	r.db.Model(&model.SessionMessage{}).
		Where("session_id = ? AND sender_type = ? AND is_read = ?", sessionID, senderType, false).
		Count(&count)
	return count
}

// ensure errors package is used to avoid import removal during splits
var _ = errors.New
