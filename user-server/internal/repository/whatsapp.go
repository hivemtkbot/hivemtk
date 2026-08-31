package repository

import (
	"context"
	"errors"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WhatsappRepository interface {
	CreateAccount(ctx context.Context, acc *model.WhatsappAccount) error
	UpdateAccount(ctx context.Context, acc *model.WhatsappAccount) error
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	ListAccounts(ctx context.Context) ([]*model.WhatsappAccount, error)
	GetAccount(ctx context.Context, id uuid.UUID) (*model.WhatsappAccount, error)

	UpsertSession(ctx context.Context, sess *model.WhatsappSession) error
	GetSession(ctx context.Context, accountID uuid.UUID) (*model.WhatsappSession, error)

	CreateDraft(ctx context.Context, d *model.WhatsappDraft) error
	UpdateDraft(ctx context.Context, d *model.WhatsappDraft) error
	DeleteDraft(ctx context.Context, id uuid.UUID) error
	ListDrafts(ctx context.Context) ([]*model.WhatsappDraft, error)
	GetDraft(ctx context.Context, id uuid.UUID) (*model.WhatsappDraft, error)

	CreateJob(ctx context.Context, j *model.WhatsappJob) error
	UpdateJob(ctx context.Context, j *model.WhatsappJob) error
	DeleteJob(ctx context.Context, id uuid.UUID) error
	ListJobs(ctx context.Context) ([]*model.WhatsappJob, error)
	GetJob(ctx context.Context, id uuid.UUID) (*model.WhatsappJob, error)

	CreateJobDetail(ctx context.Context, d *model.WhatsappJobDetail) error
	UpdateJobDetail(ctx context.Context, d *model.WhatsappJobDetail) error
	ListJobDetails(ctx context.Context, jobID uuid.UUID) ([]*model.WhatsappJobDetail, error)
}

type whatsappRepo struct {
	db *gorm.DB
}

func NewWhatsappRepository() WhatsappRepository {
	return &whatsappRepo{db: _db.GetDB()}
}

// accounts
func (r *whatsappRepo) CreateAccount(ctx context.Context, acc *model.WhatsappAccount) error {
	return r.db.Create(acc).Error
}

func (r *whatsappRepo) UpdateAccount(ctx context.Context, acc *model.WhatsappAccount) error {
	return r.db.Model(&model.WhatsappAccount{}).Where("id = ?", acc.ID).Updates(acc).Error
}

func (r *whatsappRepo) DeleteAccount(ctx context.Context, id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappAccount{}).Error
}

func (r *whatsappRepo) ListAccounts(ctx context.Context) ([]*model.WhatsappAccount, error) {
	var list []*model.WhatsappAccount
	err := r.db.Order("created_at desc").Find(&list).Error
	return list, err
}

func (r *whatsappRepo) GetAccount(ctx context.Context, id uuid.UUID) (*model.WhatsappAccount, error) {
	var acc model.WhatsappAccount
	err := r.db.Where("id = ?", id).First(&acc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &acc, err
}

// session
func (r *whatsappRepo) UpsertSession(ctx context.Context, sess *model.WhatsappSession) error {
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

func (r *whatsappRepo) GetSession(ctx context.Context, accountID uuid.UUID) (*model.WhatsappSession, error) {
	var sess model.WhatsappSession
	err := r.db.Where("account_id = ?", accountID).First(&sess).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &sess, err
}

// drafts
func (r *whatsappRepo) CreateDraft(ctx context.Context, d *model.WhatsappDraft) error {
	return r.db.Create(d).Error
}
func (r *whatsappRepo) UpdateDraft(ctx context.Context, d *model.WhatsappDraft) error {
	return r.db.Model(&model.WhatsappDraft{}).Where("id = ?", d.ID).Updates(d).Error
}
func (r *whatsappRepo) DeleteDraft(ctx context.Context, id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappDraft{}).Error
}
func (r *whatsappRepo) ListDrafts(ctx context.Context) ([]*model.WhatsappDraft, error) {
	var list []*model.WhatsappDraft
	err := r.db.Find(&list).Error
	return list, err
}
func (r *whatsappRepo) GetDraft(ctx context.Context, id uuid.UUID) (*model.WhatsappDraft, error) {
	var d model.WhatsappDraft
	err := r.db.Where("id = ?", id).First(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &d, err
}

// jobs
func (r *whatsappRepo) CreateJob(ctx context.Context, j *model.WhatsappJob) error {
	return r.db.Create(j).Error
}
func (r *whatsappRepo) UpdateJob(ctx context.Context, j *model.WhatsappJob) error {
	return r.db.Model(&model.WhatsappJob{}).Where("id = ?", j.ID).Updates(j).Error
}
func (r *whatsappRepo) DeleteJob(ctx context.Context, id uuid.UUID) error {
	return r.db.Where("id = ?", id).Delete(&model.WhatsappJob{}).Error
}
func (r *whatsappRepo) ListJobs(ctx context.Context) ([]*model.WhatsappJob, error) {
	var list []*model.WhatsappJob
	err := r.db.Find(&list).Error
	return list, err
}
func (r *whatsappRepo) GetJob(ctx context.Context, id uuid.UUID) (*model.WhatsappJob, error) {
	var j model.WhatsappJob
	err := r.db.Where("id = ?", id).First(&j).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &j, err
}

// job details
func (r *whatsappRepo) CreateJobDetail(ctx context.Context, d *model.WhatsappJobDetail) error {
	return r.db.Create(d).Error
}
func (r *whatsappRepo) UpdateJobDetail(ctx context.Context, d *model.WhatsappJobDetail) error {
	return r.db.Model(&model.WhatsappJobDetail{}).Where("id = ?", d.ID).Updates(d).Error
}
func (r *whatsappRepo) ListJobDetails(ctx context.Context, jobID uuid.UUID) ([]*model.WhatsappJobDetail, error) {
	var list []*model.WhatsappJobDetail
	err := r.db.Where("job_id = ?", jobID).Order("created_at asc").Find(&list).Error
	return list, err
}
