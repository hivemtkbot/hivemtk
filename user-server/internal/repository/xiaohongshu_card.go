package repository

import (
	"context"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// XiaohongshuCardRepository 小红书卡片仓储接口
type XiaohongshuCardRepository interface {
	Create(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Update(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.XiaohongshuCard, int64, error)
	IncrementViewCount(ctx context.Context, id uint) (*model.XiaohongshuCard, error)
	CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error
	UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error
}

// xiaohongshuCardRepository 小红书卡片仓储实现
type xiaohongshuCardRepository struct {
	db *gorm.DB
}

// NewXiaohongshuCardRepository 创建小红书卡片仓储实例
func NewXiaohongshuCardRepository(db *gorm.DB) XiaohongshuCardRepository {
	return &xiaohongshuCardRepository{
		db: db,
	}
}

// Create 创建小红书卡片
func (r *xiaohongshuCardRepository) Create(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新小红书卡片
func (r *xiaohongshuCardRepository) Update(ctx context.Context, card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if err := r.db.Save(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除小红书卡片
func (r *xiaohongshuCardRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.XiaohongshuCard{}, id).Error
}

// GetByID 根据 ID 获取小红书卡片
func (r *xiaohongshuCardRepository) GetByID(ctx context.Context, id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取小红书卡片列表
func (r *xiaohongshuCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.XiaohongshuCard, int64, error) {
	var cards []model.XiaohongshuCard
	var total int64

	query := r.db.Model(&model.XiaohongshuCard{})

	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?",
			"%"+req.Keyword+"%", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	if req.Page > 0 && req.PageSize > 0 {
		offset := (req.Page - 1) * req.PageSize
		query = query.Offset(offset).Limit(req.PageSize)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

// IncrementViewCount 增加浏览次数
func (r *xiaohongshuCardRepository) IncrementViewCount(ctx context.Context, id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&card).Update("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}

	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}

	return &card, nil
}

// CreateActivity 创建活动记录
func (r *xiaohongshuCardRepository) CreateActivity(ctx context.Context, activity *model.XiaohongshuCardActivity) error {
	return r.db.Create(activity).Error
}

// UpdateShortLinkID 更新短链 ID
func (r *xiaohongshuCardRepository) UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error {
	return r.db.Model(&model.XiaohongshuCard{}).Where("id = ?", id).Update("short_link_id", shortLinkID).Error
}

