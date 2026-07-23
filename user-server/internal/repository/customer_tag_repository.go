package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
)

// CustomerTagRepository defines the interface for customer tag data access
type CustomerTagRepository interface {
	Create(ctx context.Context, tag *model.CustomerTag) error
	GetByID(ctx context.Context, id string) (*model.CustomerTag, error)
	ListByMerchant(ctx context.Context) ([]*model.CustomerTag, error)
	ListAutoTags(ctx context.Context) ([]*model.CustomerTag, error)
	Delete(ctx context.Context, id string) error
}

// customerTagRepository implements CustomerTagRepository
type customerTagRepository struct{}

// NewCustomerTagRepository creates a new CustomerTagRepository instance
func NewCustomerTagRepository() CustomerTagRepository {
	return &customerTagRepository{}
}

// Create creates a new customer tag
func (r *customerTagRepository) Create(ctx context.Context, tag *model.CustomerTag) error {
	return _db.GetDB().Create(tag).Error
}

// GetByID retrieves a tag by ID
func (r *customerTagRepository) GetByID(ctx context.Context, id string) (*model.CustomerTag, error) {
	var tag model.CustomerTag
	if err := _db.GetDB().First(&tag, "id = ?", id).Error; err != nil {
		return nil, nil
	}
	return &tag, nil
}

func (r *customerTagRepository) ListByMerchant(ctx context.Context) ([]*model.CustomerTag, error) {
	var tags []*model.CustomerTag

	if err := _db.GetDB().
		Order("category, name").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

func (r *customerTagRepository) ListAutoTags(ctx context.Context) ([]*model.CustomerTag, error) {
	var tags []*model.CustomerTag

	if err := _db.GetDB().
		Where("source = ?", model.TagSourceAuto).
		Order("category, name").
		Find(&tags).Error; err != nil {
		return nil, err
	}

	return tags, nil
}

// Delete deletes a tag by ID
func (r *customerTagRepository) Delete(ctx context.Context, id string) error {
	return _db.GetDB().Delete(&model.CustomerTag{}, "id = ?", id).Error
}
