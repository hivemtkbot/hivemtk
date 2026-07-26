package repository

import (
	"fmt"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"

	"context"
	"gorm.io/gorm"
)

// CardListFilter 卡片列表查询过滤条件（repository 层本地定义，不依赖 dto）
// 所有平台卡片仓储统一使用此过滤结构，service 层负责 dto → CardListFilter 转换
type CardListFilter struct {
	Page     int
	PageSize int
	Keyword  string
	IsActive *bool
}

// DouyinCardRepository 抖音卡片仓储接口
type DouyinCardRepository interface {
	Create(ctx context.Context, card *model.DouyinCard) (*model.DouyinCard, error)
	Update(ctx context.Context, card *model.DouyinCard) (*model.DouyinCard, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.DouyinCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.DouyinCard, int64, error)
	IncrementViewCount(ctx context.Context, id uint) (*model.DouyinCard, error)
	IncrementLikeCount(ctx context.Context, id uint) error
	IncrementShareCount(ctx context.Context, id uint) error
}

// douyinCardRepository 抖音卡片仓储实现
type douyinCardRepository struct {
	db *gorm.DB
}

// NewDouyinCardRepository 创建抖音卡片仓储实例
func NewDouyinCardRepository(db *gorm.DB) DouyinCardRepository {
	return &douyinCardRepository{
		db: db,
	}
}

// Create 创建抖音卡片
func (r *douyinCardRepository) Create(ctx context.Context, card *model.DouyinCard) (*model.DouyinCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新抖音卡片
func (r *douyinCardRepository) Update(ctx context.Context, card *model.DouyinCard) (*model.DouyinCard, error) {
	logger.Infof("更新抖音卡片，ID: %d, ShortLinkID: %d", card.ID, card.ShortLinkID)

	// 直接更新ShortLinkID字段
	if err := r.db.Model(&model.DouyinCard{}).Where("id = ?", card.ID).Update("short_link_id", card.ShortLinkID).Error; err != nil {
		return nil, fmt.Errorf("更新short_link_id失败: %v", err)
	}

	// 更新其他字段
	if err := r.db.Model(&model.DouyinCard{}).Where("id = ?", card.ID).Updates(map[string]any{
		"title":          card.Title,
		"description":    card.Description,
		"image_url":      card.ImageURL,
		"redirect_url":   card.RedirectURL,
		"domain_pool_id": card.DomainPoolID,
		"tags":           card.Tags,
		"view_count":     card.ViewCount,
		"is_active":      card.IsActive,
	}).Error; err != nil {
		return nil, fmt.Errorf("更新其他字段失败: %v", err)
	}

	logger.Infof("更新抖音卡片成功")

	// 重新获取更新后的卡片
	var updatedCard model.DouyinCard
	if err := r.db.First(&updatedCard, card.ID).Error; err != nil {
		return nil, err
	}

	return &updatedCard, nil
}

// Delete 删除抖音卡片
func (r *douyinCardRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.DouyinCard{}, id).Error
}

// GetByID 根据ID获取抖音卡片
func (r *douyinCardRepository) GetByID(ctx context.Context, id uint) (*model.DouyinCard, error) {
	var card model.DouyinCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取抖音卡片列表
func (r *douyinCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.DouyinCard, int64, error) {
	var cards []model.DouyinCard
	var total int64

	// 基础查询
	query := r.db.Model(&model.DouyinCard{})

	// 关键词搜索（标题、描述、标签）
	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?",
			"%"+req.Keyword+"%", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	// 状态筛选
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	// 分页
	if req.Page > 0 && req.PageSize > 0 {
		offset := (req.Page - 1) * req.PageSize
		query = query.Offset(offset).Limit(req.PageSize)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 获取数据
	if err := query.Order("created_at DESC").Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

// IncrementViewCount 增加浏览次数
func (r *douyinCardRepository) IncrementViewCount(ctx context.Context, id uint) (*model.DouyinCard, error) {
	var card model.DouyinCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}

	card.ViewCount++
	if err := r.db.Save(&card).Error; err != nil {
		return nil, err
	}

	return &card, nil
}

// IncrementLikeCount 增加点赞次数
func (r *douyinCardRepository) IncrementLikeCount(ctx context.Context, id uint) error {
	return r.db.Model(&model.DouyinCard{}).Where("id = ?", id).Update("like_count", gorm.Expr("like_count + ?", 1)).Error
}

// IncrementShareCount 增加分享次数
func (r *douyinCardRepository) IncrementShareCount(ctx context.Context, id uint) error {
	// 先检查卡片是否存在
	var card model.DouyinCard
	if err := r.db.First(&card, id).Error; err != nil {
		return err
	}
	return r.db.Model(&model.DouyinCard{}).Where("id = ?", id).Update("share_count", gorm.Expr("share_count + ?", 1)).Error
}
