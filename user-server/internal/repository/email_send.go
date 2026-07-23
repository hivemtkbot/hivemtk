package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailSendRepository interface {
	Create(ctx context.Context, email *model.EmailSend) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.EmailSend, error)
	List(ctx context.Context) ([]*model.EmailSend, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status int) error
	GetPendingEmails(ctx context.Context) ([]*model.EmailSend, error)
}

type emailSendRepo struct {
	db *gorm.DB
}

func NewEmailSendRepository() EmailSendRepository {
	return &emailSendRepo{db: _db.GetDB()}
}

func (r *emailSendRepo) Create(ctx context.Context, email *model.EmailSend) error {
	return r.db.WithContext(ctx).Create(email).Error
}

func (r *emailSendRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailSend, error) {
	var email model.EmailSend
	if err := r.db.WithContext(ctx).First(&email, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &email, nil
}

func (r *emailSendRepo) GetPendingEmails(ctx context.Context) ([]*model.EmailSend, error) {
	var emails []*model.EmailSend
	now := time.Now()
	err := r.db.WithContext(ctx).Where("status = 0 AND send_time <= ?", now).Find(&emails).Error
	return emails, err
}

func (r *emailSendRepo) List(ctx context.Context) ([]*model.EmailSend, error) {
	var emails []*model.EmailSend
	if err := r.db.WithContext(ctx).Find(&emails).Error; err != nil {
		return nil, err
	}
	return emails, nil
}

func (r *emailSendRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.EmailSend{}, "id = ?", id).Error
}

func (r *emailSendRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status int) error {
	return r.db.WithContext(ctx).Model(&model.EmailSend{}).Where("id = ?", id).Update("status", status).Error
}
