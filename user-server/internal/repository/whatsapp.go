package repository

import (
	"errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
)

type WhatsappRepository interface {
	// accounts
	CreateAccount(acc *model.WhatsappAccount) error
	UpdateAccount(acc *model.WhatsappAccount) error
	DeleteAccount(id uuid.UUID) error
	ListAccounts() ([]*model.WhatsappAccount, error)
	GetAccount(id uuid.UUID) (*model.WhatsappAccount, error)

	// session
	UpsertSession(sess *model.WhatsappSession) error
	GetSession(accountID uuid.UUID) (*model.WhatsappSession, error)

	// drafts
	CreateDraft(d *model.WhatsappDraft) error
	UpdateDraft(d *model.WhatsappDraft) error
	DeleteDraft(id uuid.UUID) error
	ListDrafts() ([]*model.WhatsappDraft, error)
	GetDraft(id uuid.UUID) (*model.WhatsappDraft, error)

	// jobs
	CreateJob(j *model.WhatsappJob) error
	UpdateJob(j *model.WhatsappJob) error
	DeleteJob(id uuid.UUID) error
	ListJobs() ([]*model.WhatsappJob, error)
	GetJob(id uuid.UUID) (*model.WhatsappJob, error)

	// job details
	CreateJobDetail(d *model.WhatsappJobDetail) error
	UpdateJobDetail(d *model.WhatsappJobDetail) error
	ListJobDetails(jobID uuid.UUID) ([]*model.WhatsappJobDetail, error)
}

type whatsappRepo struct {
	db *gorm.DB
}

func NewWhatsappRepository() WhatsappRepository {
	return &whatsappRepo{db: _db.GetDB()}
}

// accounts
func (r *whatsappRepo) CreateAccount(acc *model.WhatsappAccount) error {
	return r.db.Create(acc).Error
}

func (r *whatsappRepo) UpdateAccount(acc *model.WhatsappAccount) error {
	return r.db.Model(&model.WhatsappAccount{}).Where("id = ?", acc.ID).Updates(acc).Error
}

func (r *whatsappRepo) DeleteAccount(id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappAccount{}).Error
}

func (r *whatsappRepo) ListAccounts() ([]*model.WhatsappAccount, error) {
	var list []*model.WhatsappAccount
	err := r.db.Order("created_at desc").Find(&list).Error
	return list, err
}

func (r *whatsappRepo) GetAccount(id uuid.UUID) (*model.WhatsappAccount, error) {
	var acc model.WhatsappAccount
	err := r.db.Where("id = ?", id).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &acc, err
}

// session
func (r *whatsappRepo) UpsertSession(sess *model.WhatsappSession) error {
	var exist model.WhatsappSession
	err := r.db.Where("account_id = ?", sess.AccountID).First(&exist).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(sess).Error
	}
	if err != nil {
		return err
	}
	sess.ID = exist.ID
	return r.db.Model(&model.WhatsappSession{}).Where("id = ?", sess.ID).Updates(sess).Error
}

func (r *whatsappRepo) GetSession(accountID uuid.UUID) (*model.WhatsappSession, error) {
	var sess model.WhatsappSession
	err := r.db.Where("account_id = ?", accountID).First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &sess, err
}

// drafts
func (r *whatsappRepo) CreateDraft(d *model.WhatsappDraft) error { return r.db.Create(d).Error }
func (r *whatsappRepo) UpdateDraft(d *model.WhatsappDraft) error {
	return r.db.Model(&model.WhatsappDraft{}).Where("id = ?", d.ID).Updates(d).Error
}
func (r *whatsappRepo) DeleteDraft(id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappDraft{}).Error
}
func (r *whatsappRepo) ListDrafts() ([]*model.WhatsappDraft, error) {
	var list []*model.WhatsappDraft
	err := r.db.Find(&list).Error
	return list, err
}
func (r *whatsappRepo) GetDraft(id uuid.UUID) (*model.WhatsappDraft, error) {
	var d model.WhatsappDraft
	err := r.db.Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

// jobs
func (r *whatsappRepo) CreateJob(j *model.WhatsappJob) error { return r.db.Create(j).Error }
func (r *whatsappRepo) UpdateJob(j *model.WhatsappJob) error {
	return r.db.Model(&model.WhatsappJob{}).Where("id = ?", j.ID).Updates(j).Error
}
func (r *whatsappRepo) DeleteJob(id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappJob{}).Error
}
func (r *whatsappRepo) ListJobs() ([]*model.WhatsappJob, error) {
	var list []*model.WhatsappJob
	err := r.db.Find(&list).Error
	return list, err
}
func (r *whatsappRepo) GetJob(id uuid.UUID) (*model.WhatsappJob, error) {
	var j model.WhatsappJob
	err := r.db.Where("id = ?", id).First(&j).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &j, err
}

// job details
func (r *whatsappRepo) CreateJobDetail(d *model.WhatsappJobDetail) error { return r.db.Create(d).Error }
func (r *whatsappRepo) UpdateJobDetail(d *model.WhatsappJobDetail) error {
	return r.db.Model(&model.WhatsappJobDetail{}).Where("id = ?", d.ID).Updates(d).Error
}
func (r *whatsappRepo) ListJobDetails(jobID uuid.UUID) ([]*model.WhatsappJobDetail, error) {
	var list []*model.WhatsappJobDetail
	err := r.db.Where("job_id = ?", jobID).Order("created_at asc").Find(&list).Error
	return list, err
}
