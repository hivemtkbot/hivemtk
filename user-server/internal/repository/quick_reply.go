package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// QuickReplyRepository 快捷回复仓库
type QuickReplyRepository struct {
	db *gorm.DB
}

// NewQuickReplyRepository 创建快捷回复仓库实例
func NewQuickReplyRepository() *QuickReplyRepository {
	return &QuickReplyRepository{
		db: _db.GetDB(),
	}
}

// Create 创建快捷回复
func (r *QuickReplyRepository) Create(ctx context.Context, reply *model.QuickReply) error {
	return r.db.Create(reply).Error
}

// Update 更新快捷回复
func (r *QuickReplyRepository) Update(ctx context.Context, reply *model.QuickReply) error {
	return r.db.Save(reply).Error
}

// Delete 删除快捷回复
func (r *QuickReplyRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.QuickReply{}, id).Error
}

// GetByID 根据ID获取快捷回复
func (r *QuickReplyRepository) GetByID(ctx context.Context, id uint) (*model.QuickReply, error) {
	var reply model.QuickReply
	err := r.db.First(&reply, id).Error
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (r *QuickReplyRepository) GetByMerchant(ctx context.Context, category string) ([]*model.QuickReply, error) {
	var replies []*model.QuickReply
	query := r.db.Where("is_public = ?", true)
	if category != "" {
		query = query.Where("category = ?", category)
	}
	err := query.Order("sort_order ASC").Find(&replies).Error
	return replies, err
}

// GetCategories 获取快捷回复分类列表
func (r *QuickReplyRepository) GetCategories(ctx context.Context) ([]string, error) {
	var categories []string
	err := r.db.Model(&model.QuickReply{}).
		Where("is_public = ?", true).
		Distinct("category").Pluck("category", &categories).Error
	return categories, err
}

