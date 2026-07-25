package repository

import (
	"context"

	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// MessageQueueRepository WhatsApp 消息队列仓库
//
// 涵盖三张表的持久化：
//   - WhatsAppQueueStatus    队列整体状态（pending/completed/failed/partial）
//   - WhatsAppMessageQueue   队列中单条消息
//   - WhatsappGroupMessage   群发消息记录
type MessageQueueRepository struct {
	db *gorm.DB
}

// NewMessageQueueRepository 创建消息队列仓库实例
func NewMessageQueueRepository() *MessageQueueRepository {
	return &MessageQueueRepository{db: _db.GetDB()}
}

// NewMessageQueueRepositoryWithDB 创建指定数据库连接的 MessageQueueRepository 实例（用于测试 / 服务层注入）
func NewMessageQueueRepositoryWithDB(db *gorm.DB) *MessageQueueRepository {
	return &MessageQueueRepository{db: db}
}

// SetDB 注入 db（用于测试）
//
// 五层架构 §三.5 + §七：仓库方法必须首参为 ctx context.Context。
func (r *MessageQueueRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// CreateQueueStatus 创建队列状态记录
func (r *MessageQueueRepository) CreateQueueStatus(ctx context.Context, status *model.WhatsAppQueueStatus) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(status).Error
}

// CreateMessages 批量创建队列消息
func (r *MessageQueueRepository) CreateMessages(ctx context.Context, messages []*model.WhatsAppMessageQueue) error {
	if r == nil || r.db == nil || len(messages) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&messages).Error
}

// DeleteQueueStatusByQueueID 按 queue_id 删除队列状态（用于回滚）
func (r *MessageQueueRepository) DeleteQueueStatusByQueueID(ctx context.Context, queueID string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Where("queue_id = ?", queueID).Delete(&model.WhatsAppQueueStatus{}).Error
}

// ListMessagesByQueueID 按 queue_id 拉取队列消息（按 id 升序）
func (r *MessageQueueRepository) ListMessagesByQueueID(ctx context.Context, queueID string) ([]model.WhatsAppMessageQueue, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []model.WhatsAppMessageQueue
	if err := r.db.WithContext(ctx).Where("queue_id = ?", queueID).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// UpdateMessageStatus 更新单条消息状态
func (r *MessageQueueRepository) UpdateMessageStatus(ctx context.Context, queueID, messageID, status string) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.WhatsAppMessageQueue{}).
		Where("queue_id = ? AND message_id = ?", queueID, messageID).
		Update("status", status).Error
}

// UpdateQueueStatusAtomic 原子更新队列聚合状态（消除 read-modify-write 竞态）
//
// 性能审计 P3-2：单条 UPDATE 在 DB 侧自增 sent/failed 并判定完成态，
// 避免并发场景下两请求同读 sent=5 同写 6 的竞态。
func (r *MessageQueueRepository) UpdateQueueStatusAtomic(ctx context.Context, queueID string, success bool) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Model(&model.WhatsAppQueueStatus{}).
		Where("queue_id = ?", queueID).
		UpdateColumns(map[string]any{
			"sent":       gorm.Expr("sent + CASE WHEN ? THEN 1 ELSE 0 END", success),
			"failed":     gorm.Expr("failed + CASE WHEN ? THEN 0 ELSE 1 END", success),
			"status":     gorm.Expr("CASE WHEN (sent + CASE WHEN ? THEN 1 ELSE 0 END) + (failed + CASE WHEN ? THEN 0 ELSE 1 END) >= total THEN 'completed' ELSE status END", success, success),
			"updated_at": gorm.Expr("NOW()"),
		}).Error
}

// GetQueueStatusByQueueID 按 queue_id 获取队列状态
// 找不到时返回 nil（服务层会兜底为空 QueueStatus）
func (r *MessageQueueRepository) GetQueueStatusByQueueID(ctx context.Context, queueID string) (*model.WhatsAppQueueStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var status model.WhatsAppQueueStatus
	if err := r.db.WithContext(ctx).Where("queue_id = ?", queueID).First(&status).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &status, nil
}

// ListAllQueueStatuses 列出全部队列状态（按 created_at DESC）
func (r *MessageQueueRepository) ListAllQueueStatuses(ctx context.Context) ([]model.WhatsAppQueueStatus, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	var rows []model.WhatsAppQueueStatus
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CreateGroupMessage 写入群发消息记录
func (r *MessageQueueRepository) CreateGroupMessage(ctx context.Context, record *model.WhatsappGroupMessage) error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.WithContext(ctx).Create(record).Error
}
