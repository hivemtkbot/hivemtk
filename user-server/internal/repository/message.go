package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type MessageRepository interface {
	Create(user *model.Message) error
	GetByID(id uint) (*model.Message, error)
	GetMessageList(page int, limit int) ([]*model.Message, int64, error)
	Delete(id string) error
}

type messageRepo struct {
	db *gorm.DB
}

func NewMessageRepository() MessageRepository {
	return &messageRepo{db: _db.GetDB()}
}

func (r *messageRepo) Create(message *model.Message) error {
	return r.db.Create(message).Error
}

func (r *messageRepo) GetByID(id uint) (*model.Message, error) {
	var message model.Message
	err := r.db.First(&message, id).Error
	return &message, err
}

func (r *messageRepo) GetMessageList(page int, limit int) ([]*model.Message, int64, error) {
	var messages []*model.Message
	var total int64
	err := r.db.Offset((page - 1) * limit).Limit(limit).Find(&messages).Count(&total).Error
	return messages, total, err
}

func (r *messageRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Message{}).Error
}
