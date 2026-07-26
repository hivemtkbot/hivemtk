package service

import (
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
)

type MessageQueueService struct {
	repo  *repository.MessageQueueRepository
	queues map[string][]model.QueuedMessage
	status map[string]model.QueueStatus
	mu     sync.RWMutex
}

// NewMessageQueueService 构造消息队列服务
//
// 五层架构 §三.5：构造函数保留 db *gorm.DB 参数（调用方不变），
// 内部创建 repository 实例，service 不再持有 db。
//
// 测试场景下 db 为 nil 时使用全局 DB（由 SetTestDB 设置）。
func NewMessageQueueService(db *gorm.DB) *MessageQueueService {
	var repo *repository.MessageQueueRepository
	if db != nil {
		repo = repository.NewMessageQueueRepositoryWithDB(db)
	} else {
		repo = repository.NewMessageQueueRepository()
	}
	return &MessageQueueService{
		repo:   repo,
		queues: make(map[string][]model.QueuedMessage),
		status: make(map[string]model.QueueStatus),
	}
}

func (mq *MessageQueueService) AddBatch(ctx context.Context, messages []model.QueuedMessage) (string, error) {
	queueID := generateQueueID()

	queueStatus := &model.WhatsAppQueueStatus{
		QueueID: queueID,
		Total:   len(messages),
		Sent:    0,
		Failed:  0,
		Status:  "pending",
	}
	if err := mq.repo.CreateQueueStatus(ctx, queueStatus); err != nil {
		return "", fmt.Errorf("持久化队列状态失败: %w", err)
	}

	dbMessages := make([]*model.WhatsAppMessageQueue, 0, len(messages))
	for _, msg := range messages {
		dbMessages = append(dbMessages, &model.WhatsAppMessageQueue{
			QueueID:   queueID,
			MessageID: msg.ID,
			Content:   msg.Content,
			Status:    "pending",
			Platform:  "whatsapp",
			Recipient: msg.PhoneNumber,
		})
	}
	if err := mq.repo.CreateMessages(ctx, dbMessages); err != nil {
		// 回滚已写入的队列状态记录
		_ = mq.repo.DeleteQueueStatusByQueueID(ctx, queueID)
		return "", fmt.Errorf("持久化队列消息失败: %w", err)
	}

	mq.mu.Lock()
	defer mq.mu.Unlock()

	mq.queues[queueID] = messages
	mq.status[queueID] = model.QueueStatus{
		Total:   len(messages),
		Sent:    0,
		Failed:  0,
		Status:  "pending",
		Created: time.Now(),
		Updated: time.Now(),
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

	dbMessages, err := mq.repo.ListMessagesByQueueID(ctx, queueID)
	if err != nil {
		logger.Errorf("从数据库加载队列消息失败 queueID=%s: %v", queueID, err)
		return []model.QueuedMessage{}
	}
	if len(dbMessages) == 0 {
		return []model.QueuedMessage{}
	}

	messages := make([]model.QueuedMessage, 0, len(dbMessages))
	for _, m := range dbMessages {
		messages = append(messages, model.QueuedMessage{
			ID:          m.MessageID,
			PhoneNumber: m.Recipient,
			Content:     m.Content,
			Status:      m.Status,
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
	if err := mq.repo.UpdateMessageStatus(ctx, queueID, messageID, msgStatus); err != nil {
		logger.Errorf("更新队列消息状态失败 queueID=%s messageID=%s: %v", queueID, messageID, err)
	}

	// 单条原子 UPDATE，DB 侧自增 sent/failed 并判定完成态，消除竞态与额外查询。
	if err := mq.repo.UpdateQueueStatusAtomic(ctx, queueID, success); err != nil {
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

	dbStatus, err := mq.repo.GetQueueStatusByQueueID(ctx, queueID)
	if err != nil || dbStatus == nil {
		return model.QueueStatus{}
	}

	return model.QueueStatus{
		QueueID: dbStatus.QueueID,
		Total:   dbStatus.Total,
		Sent:    dbStatus.Sent,
		Failed:  dbStatus.Failed,
		Status:  dbStatus.Status,
	}
}

func (mq *MessageQueueService) ListAllStatuses(ctx context.Context) []model.QueueStatus {
	dbStatuses, err := mq.repo.ListAllQueueStatuses(ctx)
	if err != nil {
		logger.Errorf("列出队列状态失败: %v", err)
		return []model.QueueStatus{}
	}

	result := make([]model.QueueStatus, 0, len(dbStatuses))
	for _, s := range dbStatuses {
		result = append(result, model.QueueStatus{
			QueueID: s.QueueID,
			Total:   s.Total,
			Sent:    s.Sent,
			Failed:  s.Failed,
			Status:  s.Status,
		})
	}
	return result
}

func generateQueueID() string {
	return fmt.Sprintf("queue_%d", time.Now().UnixNano())
}

func (mq *MessageQueueService) RecordGroupMessage(ctx context.Context, message model.QueuedMessage, status, errMsg string) {
	record := &model.WhatsappGroupMessage{
		ID:         uuid.New().String(),
		Phone:      message.PhoneNumber,
		Content:    message.Content,
		TemplateID: message.TemplateID,
		Status:     status,
		ErrorMsg:   errMsg,
		SentAt:     time.Now(),
	}
	if err := mq.repo.CreateGroupMessage(ctx, record); err != nil {
		logger.Errorf("写入群发记录失败: %v", err)
	}
}
