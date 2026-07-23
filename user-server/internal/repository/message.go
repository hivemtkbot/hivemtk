package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type MessageRepository interface {
	Create(ctx context.Context, user *model.Message) error
	GetByID(ctx context.Context, id uint) (*model.Message, error)
	GetMessageList(ctx context.Context, page int, limit int) ([]*model.Message, int64, error)
	Delete(ctx context.Context, id string) error
}

type messageRepo struct {
	db *gorm.DB
}

func NewMessageRepository() MessageRepository {
	return &messageRepo{db: _db.GetDB()}
}

func (r *messageRepo) Create(ctx context.Context, message *model.Message) error {
	return r.db.Create(message).Error
}

func (r *messageRepo) GetByID(ctx context.Context, id uint) (*model.Message, error) {
	var message model.Message
	err := r.db.First(&message, id).Error
	return &message, err
}

func (r *messageRepo) GetMessageList(ctx context.Context, page int, limit int) ([]*model.Message, int64, error) {
	var messages []*model.Message
	var total int64
	err := r.db.Offset((page - 1) * limit).Limit(limit).Find(&messages).Count(&total).Error
	return messages, total, err
}

func (r *messageRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Message{}).Error
}
