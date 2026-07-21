package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// EmailDraftRepository 草稿仓库接口
type EmailDraftRepository interface {
	Create(draft *model.EmailDraft) error
	GetByID(id uuid.UUID) (*model.EmailDraft, error)
	List() ([]*model.EmailDraft, error)
	Update(draft *model.EmailDraft) error
	Delete(id uuid.UUID) error
}

type emailDraftRepo struct {
	db *gorm.DB
}

// NewEmailDraftRepository 创建草稿仓库实例
func NewEmailDraftRepository() EmailDraftRepository {
	return &emailDraftRepo{db: _db.GetDB()}
}

// Create 创建草稿
func (r *emailDraftRepo) Create(draft *model.EmailDraft) error {
	return r.db.Create(draft).Error
}

// GetByID 根据ID获取草稿
func (r *emailDraftRepo) GetByID(id uuid.UUID) (*model.EmailDraft, error) {
	var draft model.EmailDraft
	if err := r.db.First(&draft, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &draft, nil
}

// List 获取所有草稿
func (r *emailDraftRepo) List() ([]*model.EmailDraft, error) {
	var drafts []*model.EmailDraft
	if err := r.db.Order("created_at DESC").Find(&drafts).Error; err != nil {
		return nil, err
	}
	return drafts, nil
}

// Update 更新草稿
func (r *emailDraftRepo) Update(draft *model.EmailDraft) error {
	return r.db.Save(draft).Error
}

// Delete 删除草稿
func (r *emailDraftRepo) Delete(id uuid.UUID) error {
	return r.db.Delete(&model.EmailDraft{}, "id = ?", id).Error
}
