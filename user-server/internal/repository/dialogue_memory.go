package repository

// dialogue_memory_repository.go 对话长期记忆仓储
//
// 五层架构归属: L4 数据访问层
// 设计依据: service/dialogue_memory.go 历史直连 DB 操作下沉
//
// 覆盖 model:
//   - DialogueMemory (对话长期记忆主表)
//   - MessageHub (短期记忆来源：按 conversation_id 取最近 N 条)

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// DialogueMemoryRepository 对话记忆仓储接口
type DialogueMemoryRepository interface {
	// DialogueMemory 主表
	GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error)
	CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	ListDialogueMemoriesByCustomer(ctx context.Context, customerID string, limit int) ([]*model.DialogueMemory, int64, error)

	// MessageHub 短期记忆来源
	ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error)
}

// dialogueMemoryRepository 实现 DialogueMemoryRepository
type dialogueMemoryRepository struct {
	db *gorm.DB
}

// NewDialogueMemoryRepository 构造（无参，内部取库句柄）
func NewDialogueMemoryRepository() DialogueMemoryRepository {
	return &dialogueMemoryRepository{db: _db.GetDB()}
}

// NewDialogueMemoryRepositoryWithDB 创建指定数据库连接的 DialogueMemoryRepository 实例（用于测试）
func NewDialogueMemoryRepositoryWithDB(db *gorm.DB) DialogueMemoryRepository {
	return &dialogueMemoryRepository{db: db}
}

// GetDialogueMemoryBySession 按 session_id 取对话记忆
// 未找到时返回 (nil, gorm.ErrRecordNotFound)，由 service 层判断是否创建
func (r *dialogueMemoryRepository) GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error) {
	var mem model.DialogueMemory
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&mem).Error
	if err != nil {
		return nil, err
	}
	return &mem, nil
}

// CreateDialogueMemory 创建对话记忆
func (r *dialogueMemoryRepository) CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	return r.db.WithContext(ctx).Create(mem).Error
}

// SaveDialogueMemory 保存对话记忆（全字段更新）
func (r *dialogueMemoryRepository) SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	return r.db.WithContext(ctx).Save(mem).Error
}

// ListDialogueMemoriesByCustomer 列出客户对话记忆（按 updated_at DESC）
// 返回 (列表, 总数, 错误)
func (r *dialogueMemoryRepository) ListDialogueMemoriesByCustomer(ctx context.Context, customerID string, limit int) ([]*model.DialogueMemory, int64, error) {
	query := r.db.WithContext(ctx).Model(&model.DialogueMemory{}).Where("customer_id = ?", customerID)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var mems []*model.DialogueMemory
	if err := query.Order("updated_at DESC").Limit(limit).Find(&mems).Error; err != nil {
		return nil, 0, err
	}
	return mems, total, nil
}

// ListMessageHubByConversation 按 conversation_id 取最近 N 条消息（按 sent_at DESC）
// service 层将其反转为正序后作为短期记忆返回
func (r *dialogueMemoryRepository) ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error) {
	var records []model.MessageHub
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
