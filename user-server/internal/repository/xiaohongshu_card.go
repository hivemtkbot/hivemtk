package repository

import (
	"gorm.io/gorm"
	"marketing/internal/dto"
	"marketing/internal/model"
)

// XiaohongshuCardRepository 小红书卡片仓储接口
type XiaohongshuCardRepository interface {
	Create(card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Update(card *model.XiaohongshuCard) (*model.XiaohongshuCard, error)
	Delete(id uint) error
	GetByID(id uint) (*model.XiaohongshuCard, error)
	GetList(req any) ([]model.XiaohongshuCard, int64, error)
	IncrementViewCount(id uint) (*model.XiaohongshuCard, error)
	CreateActivity(activity *model.XiaohongshuCardActivity) error
	UpdateShortLinkID(id uint, shortLinkID *uint) error
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
func (r *xiaohongshuCardRepository) Create(card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if err := r.db.Create(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Update 更新小红书卡片
func (r *xiaohongshuCardRepository) Update(card *model.XiaohongshuCard) (*model.XiaohongshuCard, error) {
	if err := r.db.Save(card).Error; err != nil {
		return nil, err
	}
	return card, nil
}

// Delete 删除小红书卡片
func (r *xiaohongshuCardRepository) Delete(id uint) error {
	return r.db.Delete(&model.XiaohongshuCard{}, id).Error
}

// GetByID 根据 ID 获取小红书卡片
func (r *xiaohongshuCardRepository) GetByID(id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// GetList 获取小红书卡片列表
func (r *xiaohongshuCardRepository) GetList(req any) ([]model.XiaohongshuCard, int64, error) {
	var cards []model.XiaohongshuCard
	var total int64

	// 基础查询
	query := r.db.Model(&model.XiaohongshuCard{})

	// 根据 req 参数进行过滤、排序等操作
	if req != nil {
		// 尝试类型断言获取 DTO 参数
		if listReq, ok := req.(*dto.XiaohongshuCardListRequest); ok {
			// 关键词搜索（标题、描述、标签）
			if listReq.Keyword != "" {
				query = query.Where("title LIKE ? OR description LIKE ? OR tags LIKE ?",
					"%"+listReq.Keyword+"%", "%"+listReq.Keyword+"%", "%"+listReq.Keyword+"%")
			}

			// 状态筛选
			if listReq.IsActive != nil {
				query = query.Where("is_active = ?", *listReq.IsActive)
			}

			// 分页
			if listReq.Page > 0 && listReq.PageSize > 0 {
				offset := (listReq.Page - 1) * listReq.PageSize
				query = query.Offset(offset).Limit(listReq.PageSize)
			}
		}
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
func (r *xiaohongshuCardRepository) IncrementViewCount(id uint) (*model.XiaohongshuCard, error) {
	var card model.XiaohongshuCard
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}

	if err := r.db.Model(&card).Update("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		return nil, err
	}

	// 重新获取更新后的数据
	if err := r.db.First(&card, id).Error; err != nil {
		return nil, err
	}

	return &card, nil
}

// CreateActivity 创建活动记录
func (r *xiaohongshuCardRepository) CreateActivity(activity *model.XiaohongshuCardActivity) error {
	return r.db.Create(activity).Error
}

// UpdateShortLinkID 更新短链 ID
func (r *xiaohongshuCardRepository) UpdateShortLinkID(id uint, shortLinkID *uint) error {
	return r.db.Model(&model.XiaohongshuCard{}).Where("id = ?", id).Update("short_link_id", shortLinkID).Error
}
