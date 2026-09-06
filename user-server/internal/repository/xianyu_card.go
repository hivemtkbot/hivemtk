package repository

import (
	"context"
	"errors"
	"fmt"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// XianyuCardRepository 闲鱼卡片仓库接口
type XianyuCardRepository interface {
	Create(ctx context.Context, card *model.XianyuCard) error
	Update(ctx context.Context, card *model.XianyuCard) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.XianyuCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.XianyuCard, int64, error)
}

type xianyuCardRepository struct {
	db *gorm.DB
}

// NewXianyuCardRepository 创建闲鱼卡片仓库
func NewXianyuCardRepository(db *gorm.DB) XianyuCardRepository {
	return &xianyuCardRepository{db: db}
}

func (r *xianyuCardRepository) Create(ctx context.Context, card *model.XianyuCard) error {
	if err := r.db.WithContext(ctx).Create(card).Error; err != nil {
		return fmt.Errorf("创建闲鱼卡片失败: %w", err)
	}
	return nil
}

func (r *xianyuCardRepository) Update(ctx context.Context, card *model.XianyuCard) error {
	if err := r.db.WithContext(ctx).Save(card).Error; err != nil {
		return fmt.Errorf("更新闲鱼卡片失败: %w", err)
	}
	return nil
}

func (r *xianyuCardRepository) Delete(ctx context.Context, id uint) error {
	result := r.db.WithContext(ctx).Delete(&model.XianyuCard{}, id)
	if result.Error != nil {
		return fmt.Errorf("删除闲鱼卡片失败: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("闲鱼卡片不存在")
	}
	return nil
}

func (r *xianyuCardRepository) GetByID(ctx context.Context, id uint) (*model.XianyuCard, error) {
	var card model.XianyuCard
	if err := r.db.WithContext(ctx).First(&card, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("闲鱼卡片不存在")
		}
		return nil, fmt.Errorf("获取闲鱼卡片失败: %w", err)
	}
	return &card, nil
}

func (r *xianyuCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.XianyuCard, int64, error) {
	var cards []model.XianyuCard
	var total int64

	query := r.db.WithContext(ctx).Model(&model.XianyuCard{})

	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ?", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取闲鱼卡片总数失败: %w", err)
	}

	offset := (req.Page - 1) * req.PageSize
	if err := query.
		Order("created_at DESC").
		Offset(offset).
		Limit(req.PageSize).
		Find(&cards).Error; err != nil {
		return nil, 0, fmt.Errorf("获取闲鱼卡片列表失败: %w", err)
	}

	return cards, total, nil
}
