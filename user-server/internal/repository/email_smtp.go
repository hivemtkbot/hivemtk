package repository

import (
	"context"
	"hivemtk-user/internal/model"

	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type EmailSmtpRepository interface {
	Create(ctx context.Context, emailSmtp *model.EmailSmtp) error
	GetByID(ctx context.Context, id string) (*model.EmailSmtp, error)
	GetEmailSmtpList(ctx context.Context) ([]*model.EmailSmtp, error)
	Update(ctx context.Context, emailSmtp *model.EmailSmtp) error
	Delete(ctx context.Context, id string) error
}

type emailSmtpRepo struct {
	db *gorm.DB
}

func NewEmailSmtpRepository() EmailSmtpRepository {
	return &emailSmtpRepo{db: _db.GetDB()}
}

func (r *emailSmtpRepo) Create(ctx context.Context, emailSmtp *model.EmailSmtp) error {
	return r.db.WithContext(ctx).Create(emailSmtp).Error
}

func (r *emailSmtpRepo) Update(ctx context.Context, emailSmtp *model.EmailSmtp) error {
	return r.db.WithContext(ctx).Save(emailSmtp).Error
}

func (r *emailSmtpRepo) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.EmailSmtp{}).Error
}

func (r *emailSmtpRepo) GetByID(ctx context.Context, id string) (*model.EmailSmtp, error) {
	var emailSmtp model.EmailSmtp
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&emailSmtp).Error
	return &emailSmtp, err
}

func (r *emailSmtpRepo) GetEmailSmtpList(ctx context.Context) ([]*model.EmailSmtp, error) {
	var emailSmtp []*model.EmailSmtp
	err := r.db.WithContext(ctx).Find(&emailSmtp).Error
	return emailSmtp, err
}

