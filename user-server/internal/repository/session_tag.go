package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

// SessionTagRepository 会话标签仓库
type SessionTagRepository struct {
	db *gorm.DB
}

// NewSessionTagRepository 创建会话标签仓库实例
func NewSessionTagRepository() *SessionTagRepository {
	return &SessionTagRepository{
		db: _db.GetDB(),
	}
}

// Create 创建标签
func (r *SessionTagRepository) Create(ctx context.Context, tag *model.SessionTag) error {
	return r.db.Create(tag).Error
}

// Update 更新标签
func (r *SessionTagRepository) Update(ctx context.Context, tag *model.SessionTag) error {
	return r.db.Save(tag).Error
}

// Delete 删除标签
func (r *SessionTagRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.SessionTag{}, id).Error
}

func (r *SessionTagRepository) GetByMerchant(ctx context.Context) ([]*model.SessionTag, error) {
	var tags []*model.SessionTag
	err := r.db.Where("1 = 1").Order("sort_order ASC").Find(&tags).Error
	return tags, err
}

// GetByID 根据ID获取标签
func (r *SessionTagRepository) GetByID(ctx context.Context, id uint) (*model.SessionTag, error) {
	var tag model.SessionTag
	err := r.db.First(&tag, id).Error
	if err != nil {
		return nil, err
	}
	return &tag, nil
}
