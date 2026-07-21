package repository

import (
	"marketing/internal/model"

	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type EmailSmtpRepository interface {
	Create(emailSmtp *model.EmailSmtp) error
	GetByID(id string) (*model.EmailSmtp, error)
	GetEmailSmtpList() ([]*model.EmailSmtp, error)
	Update(emailSmtp *model.EmailSmtp) error
	Delete(id string) error
}

type emailSmtpRepo struct {
	db *gorm.DB
}

func NewEmailSmtpRepository() EmailSmtpRepository {
	return &emailSmtpRepo{db: _db.GetDB()}
}

func (r *emailSmtpRepo) Create(emailSmtp *model.EmailSmtp) error {
	return r.db.Create(emailSmtp).Error
}

func (r *emailSmtpRepo) Update(emailSmtp *model.EmailSmtp) error {
	return r.db.Save(emailSmtp).Error
}

func (r *emailSmtpRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.EmailSmtp{}).Error
}

func (r *emailSmtpRepo) GetByID(id string) (*model.EmailSmtp, error) {
	var emailSmtp model.EmailSmtp
	err := r.db.Where("id = ?", id).First(&emailSmtp).Error
	return &emailSmtp, err
}

func (r *emailSmtpRepo) GetEmailSmtpList() ([]*model.EmailSmtp, error) {
	var emailSmtp []*model.EmailSmtp
	err := r.db.Find(&emailSmtp).Error
	return emailSmtp, err
}
