package repository

import (
	"context"
	"time"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// ============== MessageHubRepository ==============

// MessageHubRepository 消息中台仓库
type MessageHubRepository struct {
	db *gorm.DB
}

// NewMessageHubRepository 创建消息中台仓库实例
func NewMessageHubRepository() *MessageHubRepository {
	return &MessageHubRepository{db: _db.GetDB()}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context，
// 避免服务层 / 测试层拼接时丢失链路追踪 / 超时控制。
func (r *MessageHubRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// SetMessageHubRepoDB 工具函数（service 层使用）
func SetMessageHubRepoDB(r *MessageHubRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

// Create 创建消息中台记录（唯一约束冲突返回 nil）
func (r *MessageHubRepository) Create(ctx context.Context, hub *model.MessageHub) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Create(hub).Error
}

// GetByID 按 ID 获取消息
func (r *MessageHubRepository) GetByID(ctx context.Context, id uint) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.First(&hub, id).Error
	return &hub, err
}

// GetByMsgID 按消息 ID 获取消息
func (r *MessageHubRepository) GetByMsgID(ctx context.Context, msgID string) (*model.MessageHub, error) {
	var hub model.MessageHub
	err := r.db.Where("msg_id = ?", msgID).First(&hub).Error
	return &hub, err
}

// ListByConversation 按会话 ID 列出消息
func (r *MessageHubRepository) ListByConversation(ctx context.Context, conversationID string, page, pageSize int) ([]*model.MessageHub, int64, error) {
	var items []*model.MessageHub
	var total int64
	query := r.db.Model(&model.MessageHub{}).Where("conversation_id = ?", conversationID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := query.Order("sent_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}

// Update 更新消息中台记录
func (r *MessageHubRepository) Update(ctx context.Context, hub *model.MessageHub) error {
	return r.db.Save(hub).Error
}

// MarkReadByID 标记消息为已读
func (r *MessageHubRepository) MarkReadByID(ctx context.Context, id uint) error {
	now := time.Now()
	return r.db.Model(&model.MessageHub{}).Where("id = ?", id).
		Updates(map[string]any{
			"is_read": true,
			"read_at": now,
		}).Error
}

// ============== InboxConversationRepository ==============

// InboxConversationRepository 统一收件箱会话仓库
type InboxConversationRepository struct {
	db *gorm.DB
}

// NewInboxConversationRepository 创建收件箱会话仓库实例
func NewInboxConversationRepository() *InboxConversationRepository {
	return &InboxConversationRepository{db: _db.GetDB()}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *InboxConversationRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// SetInboxConversationRepoDB 工具函数（service 层使用）
func SetInboxConversationRepoDB(r *InboxConversationRepository, db *gorm.DB) {
	r.SetDB(context.Background(), db)
}

// FindByPlatformAccountCustomer 按平台/账号/客户查找会话
func (r *InboxConversationRepository) FindByPlatformAccountCustomer(ctx context.Context, platform, accountID, customerID string) (*model.InboxConversation, error) {
	var conv model.InboxConversation
	err := r.db.Where("platform = ? AND account_id = ? AND customer_id = ?",
		platform, accountID, customerID).First(&conv).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// Create 创建会话
func (r *InboxConversationRepository) Create(ctx context.Context, conv *model.InboxConversation) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Create(conv).Error
}

// UpdateLastMessage 更新最后消息字段（含 unread_count 自增）
func (r *InboxConversationRepository) UpdateLastMessage(ctx context.Context, id uint, lastMessage string, lastMessageAt time.Time, unreadInc int) error {
	return r.db.Model(&model.InboxConversation{}).Where("id = ?", id).
		Updates(map[string]any{
			"last_message":    lastMessage,
			"last_message_at": lastMessageAt,
			"unread_count":    gorm.Expr("unread_count + ?", unreadInc),
		}).Error
}

// GetByID 按 ID 获取会话
func (r *InboxConversationRepository) GetByID(ctx context.Context, id uint) (*model.InboxConversation, error) {
	var conv model.InboxConversation
	err := r.db.First(&conv, id).Error
	return &conv, err
}

// Update 更新会话
func (r *InboxConversationRepository) Update(ctx context.Context, conv *model.InboxConversation) error {
	return r.db.Save(conv).Error
}

// ListByAccount 按账号列出会话
func (r *InboxConversationRepository) ListByAccount(ctx context.Context, platform, accountID string, page, pageSize int) ([]*model.InboxConversation, int64, error) {
	var items []*model.InboxConversation
	var total int64
	query := r.db.Model(&model.InboxConversation{}).
		Where("platform = ? AND account_id = ?", platform, accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	err := query.Order("pinned DESC, last_message_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&items).Error
	return items, total, err
}
