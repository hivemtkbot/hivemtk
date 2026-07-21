package repository

import (
	"marketing/internal/dto"
	"marketing/internal/model"

	"gorm.io/gorm"
)

// KuaishouCardRepository 快手卡片仓储接口
type KuaishouCardRepository interface {
	Create(card *model.KuaishouCard) (*model.KuaishouCard, error)
	Update(card *model.KuaishouCard) (*model.KuaishouCard, error)
	Delete(id uint) error
	GetByID(id uint) (*model.KuaishouCard, error)
	GetList(req *dto.KuaishouCardListRequest) ([]model.KuaishouCard, int64, error)
	IncrementViewCount(id uint) (*model.KuaishouCard, error)
	IncrementLikeCount(id uint) error
	IncrementShareCount(id uint) error
	CreateActivity(activity *model.KuaishouCardActivity) error
	UpdateShortLinkID(id uint, shortLinkID *uint) error
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
func (r *kuaishouCardRepository) Create(card *model.KuaishouCard) (*model.KuaishouCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新快手卡片
func (r *kuaishouCardRepository) Update(card *model.KuaishouCard) (*model.KuaishouCard, error) {
	if err := r.db.Save(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除快手卡片
func (r *kuaishouCardRepository) Delete(id uint) error {
	return r.db.Delete(&model.KuaishouCard{}, id).Error
}

// GetByID 根据ID获取快手卡片
func (r *kuaishouCardRepository) GetByID(id uint) (*model.KuaishouCard, error) {
	var card model.KuaishouCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取快手卡片列表
func (r *kuaishouCardRepository) GetList(req *dto.KuaishouCardListRequest) ([]model.KuaishouCard, int64, error) {
	var cards []model.KuaishouCard
	var total int64

	query := r.db.Model(&model.KuaishouCard{})

	// 应用搜索条件
	if req.Keyword != "" {
		query = query.Where("title LIKE ? OR tags LIKE ?", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}
	if req.IsActive != nil {
		query = query.Where("is_active = ?", *req.IsActive)
	}

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (req.Page - 1) * req.PageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(req.PageSize).Find(&cards).Error; err != nil {
		return nil, 0, err
	}

	return cards, total, nil
}

// IncrementViewCount 增加浏览数
func (r *kuaishouCardRepository) IncrementViewCount(id uint) (*model.KuaishouCard, error) {
	var card model.KuaishouCard
	if err := r.db.Model(&card).Where("id = ?", id).UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}
	return r.GetByID(id)
}

// IncrementLikeCount 增加点赞数
func (r *kuaishouCardRepository) IncrementLikeCount(id uint) error {
	// 先检查卡片是否存在
	var card model.KuaishouCard
	if err := r.db.First(&card, id).Error; err != nil {
		return err
	}
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).UpdateColumn("like_count", gorm.Expr("like_count + 1")).Error
}

// IncrementShareCount 增加分享数
func (r *kuaishouCardRepository) IncrementShareCount(id uint) error {
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).UpdateColumn("share_count", gorm.Expr("share_count + 1")).Error
}

// CreateActivity 创建活动记录
func (r *kuaishouCardRepository) CreateActivity(activity *model.KuaishouCardActivity) error {
	return r.db.Create(activity).Error
}

// UpdateShortLinkID 更新短链ID
func (r *kuaishouCardRepository) UpdateShortLinkID(id uint, shortLinkID *uint) error {
	return r.db.Model(&model.KuaishouCard{}).Where("id = ?", id).Update("short_link_id", shortLinkID).Error
}
