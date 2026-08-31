package repository

import (
	"hivemtk-user/internal/model"

	"context"

	"gorm.io/gorm"
)

// KuaishouCardRepository 快手卡片仓储接口
type KuaishouCardRepository interface {
	Create(ctx context.Context, card *model.KuaishouCard) (*model.KuaishouCard, error)
	Update(ctx context.Context, card *model.KuaishouCard) (*model.KuaishouCard, error)
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.KuaishouCard, error)
	GetList(ctx context.Context, req CardListFilter) ([]model.KuaishouCard, int64, error)
	IncrementViewCount(ctx context.Context, id uint) (*model.KuaishouCard, error)
	IncrementLikeCount(ctx context.Context, id uint) error
	IncrementShareCount(ctx context.Context, id uint) error
	CreateActivity(ctx context.Context, activity *model.KuaishouCardActivity) error
	UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error
}

// kuaishouCardRepository 快手卡片仓储实现
type kuaishouCardRepository struct {
	db *gorm.DB
}

// NewKuaishouCardRepository 创建快手卡片仓储实例
func NewKuaishouCardRepository(db *gorm.DB) KuaishouCardRepository {
	return &kuaishouCardRepository{
		db: db,
	}
}

// Create 创建快手卡片
func (r *kuaishouCardRepository) Create(ctx context.Context, card *model.KuaishouCard) (*model.KuaishouCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新快手卡片
func (r *kuaishouCardRepository) Update(ctx context.Context, card *model.KuaishouCard) (*model.KuaishouCard, error) {
	if err := r.db.Save(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除快手卡片
func (r *kuaishouCardRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.KuaishouCard{}, id).Error
}

// GetByID 根据ID获取快手卡片
func (r *kuaishouCardRepository) GetByID(ctx context.Context, id uint) (*model.KuaishouCard, error) {
	var card model.KuaishouCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取快手卡片列表
func (r *kuaishouCardRepository) GetList(ctx context.Context, req CardListFilter) ([]model.KuaishouCard, int64, error) {
	var cards []model.KuaishouCard
	var total int64

	query := r.db.Model(&model.KuaishouCard{})

	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR tags LIKE ?", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (req.Page - 1) * req.PageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(req.PageSize).Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

// IncrementViewCount 增加浏览数
func (r *kuaishouCardRepository) IncrementViewCount(ctx context.Context, id uint) (*model.KuaishouCard, error) {
	var card model.KuaishouCard
	if err := r.db.Model(&card).Where("id = ?", id).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}
	return r.GetByID(ctx, id)
}

// IncrementLikeCount 增加点赞数
func (r *kuaishouCardRepository) IncrementLikeCount(ctx context.Context, id uint) error {
	// 先检查卡片是否存在
	var card model.KuaishouCard
	if err := r.db.First(&card, id).Error; err != nil {
		return err
	}
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
}

// IncrementShareCount 增加分享数
func (r *kuaishouCardRepository) IncrementShareCount(ctx context.Context, id uint) error {
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error
}

// CreateActivity 创建活动记录
func (r *kuaishouCardRepository) CreateActivity(ctx context.Context, activity *model.KuaishouCardActivity) error {
	return r.db.Create(activity).Error
}

// UpdateShortLinkID 更新短链ID
func (r *kuaishouCardRepository) UpdateShortLinkID(ctx context.Context, id uint, shortLinkID *uint) error {
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).Update("short_link_id", shortLinkID).Error
}
