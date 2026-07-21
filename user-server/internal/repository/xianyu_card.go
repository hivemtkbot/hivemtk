package repository

import (
	"context"
	"errors"
	"fmt"
	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
)

// XianyuCardRepository 咸鱼卡片仓库接口
type XianyuCardRepository interface {
	Create(ctx context.Context, card *model.XianyuCard) error
	Update(ctx context.Context, card *model.XianyuCard) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.XianyuCard, error)
	GetList(ctx context.Context, req *dto.XianyuCardListRequest) ([]model.XianyuCard, int64, error)
}

// xianyuCardRepository 咸鱼卡片仓库实现
type xianyuCardRepository struct {
	db *gorm.DB
}

// NewXianyuCardRepository 创建咸鱼卡片仓库
func NewXianyuCardRepository(db *gorm.DB) XianyuCardRepository {
	return &xianyuCardRepository{db: db}
}

// Create 创建咸鱼卡片
func (r *xianyuCardRepository) Create(ctx context.Context, card *model.XianyuCard) error {
	if err := r.db.WithContext(ctx).Create(card).Error; err != nil {
		return fmt.Errorf("创建咸鱼卡片失败: %w", err)
	}
	return nil
}

// Update 更新咸鱼卡片
func (r *xianyuCardRepository) Update(ctx context.Context, card *model.XianyuCard) error {
	if err := r.db.WithContext(ctx).Save(card).Error; err != nil {
		return fmt.Errorf("更新咸鱼卡片失败: %w", err)
	}
	return nil
}

// Delete 删除咸鱼卡片
func (r *xianyuCardRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.XianyuCard{}, id)
	if result.Error != nil {
		return fmt.Errorf("删除咸鱼卡片失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("咸鱼卡片不存在")
	}
	return nil
}

// GetByID 根据ID获取咸鱼卡片
func (r *xianyuCardRepository) GetByID(ctx context.Context, id uint) (*model.XianyuCard, error) {
	var card model.XianyuCard
	if err := r.db.WithContext(ctx).First(&card, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("咸鱼卡片不存在")
		}
		return nil, fmt.Errorf("获取咸鱼卡片失败: %w", err)
	}
	return &card, nil
}

// GetList 获取咸鱼卡片列表
func (r *xianyuCardRepository) GetList(ctx context.Context, req *dto.XianyuCardListRequest) ([]model.XianyuCard, int64, error) {
	var cards []model.XianyuCard
	var total int64

	// 构建查询条件
	query := r.db.WithContext(ctx).Model(&model.XianyuCard{})

	// 关键词搜索
	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ?", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	// 激活状态筛选
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取咸鱼卡片总数失败: %w", err)
	}

	// 获取列表
	offset := (req.Page - 1) * req.PageSize
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(req.PageSize).
		Find(&cards).Error; err != nil {
		return nil, 0, fmt.Errorf("获取咸鱼卡片列表失败: %w", err)
	}

	return cards, total, nil
}
