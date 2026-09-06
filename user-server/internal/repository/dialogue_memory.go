package repository

import (
	"context"

	"gorm.io/gorm"

	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
)

// DialogueMemoryRepository 对话记忆仓储接口
type DialogueMemoryRepository interface {
	GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error)
	CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error
	ListDialogueMemoriesByCustomer(ctx context.Context, customerID string, limit int) ([]*model.DialogueMemory, int64, error)

	ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error)
}

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

func (r *dialogueMemoryRepository) GetDialogueMemoryBySession(ctx context.Context, sessionID string) (*model.DialogueMemory, error) {
	var mem model.DialogueMemory
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).First(&mem).Error
	if err != nil {
		return nil, err
	}
	return &mem, nil
}

func (r *dialogueMemoryRepository) CreateDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	return r.db.WithContext(ctx).Create(mem).Error
}

func (r *dialogueMemoryRepository) SaveDialogueMemory(ctx context.Context, mem *model.DialogueMemory) error {
	return r.db.WithContext(ctx).Save(mem).Error
}

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

func (r *dialogueMemoryRepository) ListMessageHubByConversation(ctx context.Context, conversationID string, limit int) ([]model.MessageHub, error) {
	var records []model.MessageHub
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).
		Order("sent_at DESC").Limit(limit).Find(&records).Error
	if err != nil {
		return nil, err
	}
	return records, nil
}
