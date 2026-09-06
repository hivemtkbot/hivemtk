package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"
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
	// ProcessPendingEmails 为定时循环调用,默认每轮 100 封防无界载入;本轮已达上限时下一轮定时周期继续处理
	q := applyListLimit(r.db.WithContext(ctx).Where("status = 0 AND send_time <= ?", now), 100)
	if err := q.Find(&emails).Error; err != nil {
		return nil, err
	}
	return emails, nil
}

func (r *emailSendRepo) List(ctx context.Context) ([]*model.EmailSend, error) {
	var emails []*model.EmailSend
	// 默认 50 兜底;已达上限时调用方需分页重取
	q := applyListLimit(r.db.WithContext(ctx), 50)
	if err := q.Find(&emails).Error; err != nil {
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
