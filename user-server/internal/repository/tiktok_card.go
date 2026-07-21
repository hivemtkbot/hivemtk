package repository

import (
	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
)

// TikTokCardRepository TikTok 卡片仓储接口
type TikTokCardRepository interface {
	Create(card *model.TikTokCard) (*model.TikTokCard, error)
	Update(card *model.TikTokCard) (*model.TikTokCard, error)
	Delete(id uint) error
	GetByID(id uint) (*model.TikTokCard, error)
	GetList(req *dto.TikTokCardListRequest) ([]model.TikTokCard, int64, error)
	IncrementViewCount(id uint) error
	IncrementLikeCount(id uint) error
	IncrementShareCount(id uint) error
	CreateActivity(activity *model.TikTokCardActivity) error
	GetOverallStats() (int64, int64, int64, []model.TikTokCard, error)
	GetCardStats(id uint, days int) (*model.TikTokCard, []model.TikTokCardActivity, error)
	ListAll() ([]model.TikTokCard, error)
}

// tiktokCardRepository TikTok 卡片仓储实现
type tiktokCardRepository struct {
	db *gorm.DB
}

// NewTikTokCardRepository 创建 TikTok 卡片仓储实例
func NewTikTokCardRepository(db *gorm.DB) TikTokCardRepository {
	return &tiktokCardRepository{db: db}
}

// Create 创建 TikTok 卡片
func (r *tiktokCardRepository) Create(card *model.TikTokCard) (*model.TikTokCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新 TikTok 卡片
func (r *tiktokCardRepository) Update(card *model.TikTokCard) (*model.TikTokCard, error) {
	if err := r.db.Save(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除 TikTok 卡片
func (r *tiktokCardRepository) Delete(id uint) error {
	return r.db.Where("id = ?", id).Delete(&model.TikTokCard{}).Error
}

// GetByID 根据 ID 获取 TikTok 卡片
func (r *tiktokCardRepository) GetByID(id uint) (*model.TikTokCard, error) {
	var card model.TikTokCard
	if err := r.db.Where("id = ?", id).First(&card).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取 TikTok 卡片列表
func (r *tiktokCardRepository) GetList(req *dto.TikTokCardListRequest) ([]model.TikTokCard, int64, error) {
	var cards []model.TikTokCard
	var total int64

	query := r.db.Model(&model.TikTokCard{})

	if req != nil {
		if req.Keyword != "" {
			like := "%" + req.Keyword + "%"
			query = query.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
		}
		if req.IsActive != nil {
			query = query.Where("is_active = ?", *req.IsActive)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := 1
	pageSize := 20
	if req != nil {
		if req.Page > 0 {
			page = req.Page
		}
		if req.PageSize > 0 {
			pageSize = req.PageSize
		}
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

// IncrementViewCount 增加浏览数
func (r *tiktokCardRepository) IncrementViewCount(id uint) error {
	return r.db.Model(&model.TikTokCard{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error
}

// IncrementLikeCount 增加点赞数
func (r *tiktokCardRepository) IncrementLikeCount(id uint) error {
	return r.db.Model(&model.TikTokCard{}).
		Where("id = ?", id).
		UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
}

// IncrementShareCount 增加分享数
func (r *tiktokCardRepository) IncrementShareCount(id uint) error {
	return r.db.Model(&model.TikTokCard{}).
		Where("id = ?", id).
		UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error
}

// CreateActivity 创建活动记录
func (r *tiktokCardRepository) CreateActivity(activity *model.TikTokCardActivity) error {
	return r.db.Create(activity).Error
}

// GetOverallStats 获取总体统计（独立部署：单租户）
func (r *tiktokCardRepository) GetOverallStats() (int64, int64, int64, []model.TikTokCard, error) {
	var totalCards, activeCards, totalViews int64

	if err := r.db.Model(&model.TikTokCard{}).Count(&totalCards).Error; err != nil {
		return 0, 0, 0, nil, err
	}
	if err := r.db.Model(&model.TikTokCard{}).Where("is_active = ?", true).Count(&activeCards).Error; err != nil {
		return 0, 0, 0, nil, err
	}
	if err := r.db.Model(&model.TikTokCard{}).Select("COALESCE(SUM(view_count), 0)").Row().Scan(&totalViews); err != nil {
		return 0, 0, 0, nil, err
	}

	var popular []model.TikTokCard
	if err := r.db.Order("view_count DESC").Limit(5).Find(&popular).Error; err != nil {
		return 0, 0, 0, nil, err
	}

	return totalCards, activeCards, totalViews, popular, nil
}

// GetCardStats 获取单个卡片统计（独立部署：单租户）
func (r *tiktokCardRepository) GetCardStats(id uint, days int) (*model.TikTokCard, []model.TikTokCardActivity, error) {
	var card model.TikTokCard
	if err := r.db.Where("id = ?", id).First(&card).Error; err != nil {
		return nil, nil, err
	}
	var activities []model.TikTokCardActivity
	if days <= 0 {
		days = 7
	}
	if err := r.db.Where("card_id = ?", id).Order("created_at DESC").Limit(50).Find(&activities).Error; err != nil {
		return nil, nil, err
	}
	return &card, activities, nil
}

// ListAll 列出所有卡片（独立部署：单租户）
func (r *tiktokCardRepository) ListAll() ([]model.TikTokCard, error) {
	var cards []model.TikTokCard
	if err := r.db.Find(&cards).Error; err != nil {
		return nil, err
	}
	return cards, nil
}
