package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailDraftRepository 草稿仓库接口
type EmailDraftRepository interface {
	Create(ctx context.Context, draft *model.EmailDraft) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.EmailDraft, error)
	List(ctx context.Context) ([]*model.EmailDraft, error)
	Update(ctx context.Context, draft *model.EmailDraft) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type emailDraftRepo struct {
	db *gorm.DB
}

// NewEmailDraftRepository 创建草稿仓库实例
func NewEmailDraftRepository() EmailDraftRepository {
	return &emailDraftRepo{db: _db.GetDB()}
}

func (r *emailDraftRepo) Create(ctx context.Context, draft *model.EmailDraft) error {
	return r.db.WithContext(ctx).Create(draft).Error
}

func (r *emailDraftRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.EmailDraft, error) {
	var draft model.EmailDraft
	if err := r.db.WithContext(ctx).First(&draft, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &draft, nil
}

func (r *emailDraftRepo) List(ctx context.Context) ([]*model.EmailDraft, error) {
	var drafts []*model.EmailDraft
	if err := r.db.WithContext(ctx).Order("created_at DESC").Find(&drafts).Error; err != nil {
		return nil, err
	}
	return drafts, nil
}

func (r *emailDraftRepo) Update(ctx context.Context, draft *model.EmailDraft) error {
	return r.db.WithContext(ctx).Save(draft).Error
}

func (r *emailDraftRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Delete(&model.EmailDraft{}, "id = ?", id).Error
}
