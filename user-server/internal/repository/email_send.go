package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type EmailSendRepository interface {
	Create(email *model.EmailSend) error
	GetByID(id uuid.UUID) (*model.EmailSend, error)
	List() ([]*model.EmailSend, error)
	Delete(id uuid.UUID) error
	UpdateStatus(id uuid.UUID, status int) error
	GetPendingEmails() ([]*model.EmailSend, error)
}

type emailSendRepo struct {
	db *gorm.DB
}

func NewEmailSendRepository() EmailSendRepository {
	return &emailSendRepo{db: _db.GetDB()}
}

func (r *emailSendRepo) Create(email *model.EmailSend) error {
	return r.db.Create(email).Error
}

func (r *emailSendRepo) GetByID(id uuid.UUID) (*model.EmailSend, error) {
	var email model.EmailSend
	if err := r.db.First(&email, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &email, nil
}

func (r *emailSendRepo) GetPendingEmails() ([]*model.EmailSend, error) {
	var emails []*model.EmailSend
	now := time.Now()
	err := r.db.Where("status = 0 AND send_time <= ?", now).Find(&emails).Error
	return emails, err
}

func (r *emailSendRepo) List() ([]*model.EmailSend, error) {
	var emails []*model.EmailSend
	if err := r.db.Find(&emails).Error; err != nil {
		return nil, err
	}
	return emails, nil
}

func (r *emailSendRepo) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.EmailSend{}, "id = ?", id).Error
}

func (r *emailSendRepo) UpdateStatus(id uuid.UUID, status int) error {
	return r.db.Model(&model.EmailSend{}).Where("id = ?", id).Update("status", status).Error
}
