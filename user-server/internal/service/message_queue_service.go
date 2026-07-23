package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"context"
)

type MessageQueueService struct {
	db	*gorm.DB
	queues	map[string][]model.QueuedMessage
	status	map[string]model.QueueStatus
	mu	sync.RWMutex
}

func NewMessageQueueService(db *gorm.DB) *MessageQueueService {
	return &MessageQueueService{
		db:	db,
		queues:	make(map[string][]model.QueuedMessage),
		status:	make(map[string]model.QueueStatus),
	}
}

func (mq *MessageQueueService) AddBatch(ctx context.Context, messages []model.QueuedMessage) (string, error) {
	queueID := generateQueueID()

	queueStatus := &model.WhatsAppQueueStatus{
		QueueID:	queueID,
		Total:		len(messages),
		Sent:		0,
		Failed:		0,
		Status:		"pending",
	}
	if err := mq.db.Create(queueStatus).Error; err != nil {
		return "", fmt.Errorf("持久化队列状态失败: %w", err)
	}

	dbMessages := make([]*model.WhatsAppMessageQueue, 0, len(messages))
	for _, msg := range messages {
		dbMessages = append(dbMessages, &model.WhatsAppMessageQueue{
			QueueID:	queueID,
			MessageID:	msg.ID,
			Content:	msg.Content,
			Status:		"pending",
			Platform:	"whatsapp",
			Recipient:	msg.PhoneNumber,
		})
	}
	if err := mq.db.Create(&dbMessages).Error; err != nil {
		mq.db.Where("queue_id = ?", queueID).Delete(&model.WhatsAppQueueStatus{})
		return "", fmt.Errorf("持久化队列消息失败: %w", err)
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()

	mq.queues[queueID] = messages
	mq.status[queueID] = model.QueueStatus{
		Total:		len(messages),
		Sent:		0,
		Failed:		0,
		Status:		"pending",
		Created:	time.Now(),
		Updated:	time.Now(),
	}

	return queueID, nil
}

func (mq *MessageQueueService) GetQueue(ctx context.Context, queueID string) []model.QueuedMessage {
	mq.mu.RLock()
	queue, exists := mq.queues[queueID]
	mq.mu.RUnlock()
	if exists {
		return queue
	}

	var dbMessages []model.WhatsAppMessageQueue
	if err := mq.db.Where("queue_id = ?", queueID).Order("id ASC").Find(&dbMessages).Error; err != nil {
		logger.Errorf("从数据库加载队列消息失败 queueID=%s: %v", queueID, err)
		return []model.QueuedMessage{}
	}
	if len(dbMessages) == 0 {
		return []model.QueuedMessage{}
	}

	messages := make([]model.QueuedMessage, 0, len(dbMessages))
	for _, m := range dbMessages {
		messages = append(messages, model.QueuedMessage{
			ID:		m.MessageID,
			PhoneNumber:	m.Recipient,
			Content:	m.Content,
			Status:		m.Status,
		})
	}
	return messages
}

func (mq *MessageQueueService) UpdateStatus(ctx context.Context, queueID, messageID string, success bool) {
	msgStatus := "sent"
	if !success {
		msgStatus = "failed"
	}

	// 1) 更新单条消息状态（不同表，必须独立语句）
	if err := mq.db.Model(&model.WhatsAppMessageQueue{}).
		Where("queue_id = ? AND message_id = ?", queueID, messageID).
		Update("status", msgStatus).Error; err != nil {
		logger.Errorf("更新队列消息状态失败 queueID=%s messageID=%s: %v", queueID, messageID, err)
	}

	// 性能审计 P3-2：原实现 First + Save 是 read-modify-write，存在并发竞态（两并发同读 sent=5 同写 6），
	// 且每条消息多 2 次查询（1000 万/日主动触达 = 2000 万额外查询）。
	// 改为单条原子 UPDATE，DB 侧自增 sent/failed 并判定完成态，消除竞态与额外查询。
	if err := mq.db.Model(&model.WhatsAppQueueStatus{}).
		Where("queue_id = ?", queueID).
		UpdateColumns(map[string]any{
			"sent":		gorm.Expr("sent + CASE WHEN ? THEN 1 ELSE 0 END", success),
			"failed":	gorm.Expr("failed + CASE WHEN ? THEN 0 ELSE 1 END", success),
			"status":	gorm.Expr("CASE WHEN (sent + CASE WHEN ? THEN 1 ELSE 0 END) + (failed + CASE WHEN ? THEN 0 ELSE 1 END) >= total THEN 'completed' ELSE status END", success, success),
			"updated_at":	gorm.Expr("NOW()"),
		}).Error; err != nil {
		logger.Errorf("更新队列聚合状态失败 queueID=%s: %v", queueID, err)
	}

	// 内存缓存同步（仅用于实时查询，非强一致；DB 为真相源）
	mq.mu.Lock()
	if qs, ok := mq.status[queueID]; ok {
		if success {
			qs.Sent++
		} else {
			qs.Failed++
		}
		qs.Updated = time.Now()
		if qs.Sent+qs.Failed >= qs.Total {
			qs.Status = "completed"
		}
		mq.status[queueID] = qs
	}
	mq.mu.Unlock()
}

func (mq *MessageQueueService) GetStatus(ctx context.Context, queueID string) model.QueueStatus {
	mq.mu.RLock()
	defer mq.mu.RUnlock()

	if qs, ok := mq.status[queueID]; ok {
		return qs
	}

	var dbStatus model.WhatsAppQueueStatus
	if err := mq.db.Where("queue_id = ?", queueID).First(&dbStatus).Error; err != nil {
		return model.QueueStatus{}
	}

	return model.QueueStatus{
		QueueID:	dbStatus.QueueID,
		Total:		dbStatus.Total,
		Sent:		dbStatus.Sent,
		Failed:		dbStatus.Failed,
		Status:		dbStatus.Status,
	}
}

func (mq *MessageQueueService) ListAllStatuses(ctx context.Context,) []model.QueueStatus {
	var dbStatuses []model.WhatsAppQueueStatus
	if err := mq.db.Order("created_at DESC").Find(&dbStatuses).Error; err != nil {
		logger.Errorf("列出队列状态失败: %v", err)
		return []model.QueueStatus{}
	}

	result := make([]model.QueueStatus, 0, len(dbStatuses))
	for _, s := range dbStatuses {
		result = append(result, model.QueueStatus{
			QueueID:	s.QueueID,
			Total:		s.Total,
			Sent:		s.Sent,
			Failed:		s.Failed,
			Status:		s.Status,
		})
	}
	return result
}

func generateQueueID() string {
	return fmt.Sprintf("queue_%d", time.Now().UnixNano())
}

func (mq *MessageQueueService) RecordGroupMessage(ctx context.Context, message model.QueuedMessage, status, errMsg string) {
	record := &model.WhatsappGroupMessage{
		ID:		uuid.New().String(),
		Phone:		message.PhoneNumber,
		Content:	message.Content,
		TemplateID:	message.TemplateID,
		Status:		status,
		ErrorMsg:	errMsg,
		SentAt:		time.Now(),
	}
	if err := mq.db.Create(record).Error; err != nil {
		logger.Errorf("写入群发记录失败: %v", err)
	}
}
