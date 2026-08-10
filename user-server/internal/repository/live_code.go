package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"time"

	"gorm.io/gorm"
)

// LiveCodeRepository 活码仓储接口
type LiveCodeRepository interface {
	Create(ctx context.Context, liveCode *model.LiveCode) error
	Update(ctx context.Context, liveCode *model.LiveCode) error
	Delete(ctx context.Context, id string) error
	GetAvailableLiveCodes(ctx context.Context) ([]*model.LiveCode, error)
	GetByID(ctx context.Context, id string) (*model.LiveCode, error)
	GetByShortLink(ctx context.Context, shortLink string) (*model.LiveCode, error)
	GetList(ctx context.Context, page, pageSize int, name, status string) ([]*model.LiveCode, int64, error)
	// IncrementClicks 原子累加活码点击次数，避免并发读改写导致计数丢失（lost-update）
	IncrementClicks(ctx context.Context, id string) error
}

// liveCodeRepository 活码仓储实现
type liveCodeRepository struct {
	db *gorm.DB
}

// NewLiveCodeRepository 创建活码仓储实例
func NewLiveCodeRepository(db *gorm.DB) LiveCodeRepository {
	return &liveCodeRepository{db: db}
}

// Create 创建活码
func (r *liveCodeRepository) Create(ctx context.Context, liveCode *model.LiveCode) error {
	return r.db.Create(liveCode).Error
}

// Update 更新活码
func (r *liveCodeRepository) Update(ctx context.Context, liveCode *model.LiveCode) error {
	return r.db.Save(liveCode).Error
}

// Delete 删除活码
func (r *liveCodeRepository) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.LiveCode{}).Error
}

// GetAvailableLiveCodes 获取可用的活码（用于轮询）
func (r *liveCodeRepository) GetAvailableLiveCodes(ctx context.Context) ([]*model.LiveCode, error) {
	var liveCodes []*model.LiveCode
	now := time.Now()

	err := r.db.Where("status = ? AND created_at > ?", 1, now.AddDate(0, 0, -7)).
		Find(&liveCodes).Error
	return liveCodes, err
}

// IncrementClicks 原子累加活码点击次数（total_clicks/daily_clicks 各 +1）。
// 使用数据库侧的 UPDATE ... SET col = col + 1，避免「读-改-写」在并发扫码时丢计数（lost-update）。
func (r *liveCodeRepository) IncrementClicks(ctx context.Context, id string) error {
	return r.db.Model(&model.LiveCode{}).
		Where("id = ?", id).
		UpdateColumns(map[string]interface{}{
			"total_clicks": gorm.Expr("total_clicks + 1"),
			"daily_clicks": gorm.Expr("daily_clicks + 1"),
		}).Error
}

// GetByID 根据ID获取活码
func (r *liveCodeRepository) GetByID(ctx context.Context, id string) (*model.LiveCode, error) {
	var liveCode model.LiveCode
	err := r.db.Where("id = ?", id).First(&liveCode).Error
	if err != nil {
		return nil, err
	}

	// 手动加载关联数据
	if liveCode.ShortDomainID > 0 {
		var shortDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
			liveCode.ShortDomain = &shortDomain
		}
	}

	if liveCode.EntryDomainID > 0 {
		var entryDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
			liveCode.EntryDomain = &entryDomain
		}
	}

	if liveCode.LandingDomainID > 0 {
		var landingDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
			liveCode.LandingDomain = &landingDomain
		}
	}

	return &liveCode, nil
}

// GetByShortLink 根据短链获取活码
func (r *liveCodeRepository) GetByShortLink(ctx context.Context, shortLink string) (*model.LiveCode, error) {
	var liveCode model.LiveCode
	err := r.db.Where("short_link = ?", shortLink).First(&liveCode).Error
	if err != nil {
		return nil, err
	}

	// 手动加载关联数据
	if liveCode.ShortDomainID > 0 {
		var shortDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
			liveCode.ShortDomain = &shortDomain
		}
	}

	if liveCode.EntryDomainID > 0 {
		var entryDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
			liveCode.EntryDomain = &entryDomain
		}
	}

	if liveCode.LandingDomainID > 0 {
		var landingDomain model.DomainPool
		if err := r.db.Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
			liveCode.LandingDomain = &landingDomain
		}
	}

	return &liveCode, nil
}

// GetList 获取活码列表
func (r *liveCodeRepository) GetList(ctx context.Context, page, pageSize int, name, status string) ([]*model.LiveCode, int64, error) {
	var liveCodes []*model.LiveCode
	var total int64

	// 构建查询条件
	query := r.db.Model(&model.LiveCode{})

	// 如果有名称搜索条件
	if name != "" {
		query = query.Where("name LIKE ?", "%"+name+"%")
	}

	// 如果有状态搜索条件
	if status != "" {
		// Convert string status to int for proper comparison
		if status == "1" {
			query = query.Where("status = ?", 1)
		} else if status == "0" {
			query = query.Where("status = ?", 0)
		}
	}

	// 计算总数
	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&liveCodes).Error
	if err != nil {
		return nil, 0, err
	}

	// 手动加载关联数据
	for _, liveCode := range liveCodes {
		if liveCode.ShortDomainID > 0 {
			var shortDomain model.DomainPool
			if err := r.db.Where("id = ?", liveCode.ShortDomainID).First(&shortDomain).Error; err == nil {
				liveCode.ShortDomain = &shortDomain
			}
		}

		if liveCode.EntryDomainID > 0 {
			var entryDomain model.DomainPool
			if err := r.db.Where("id = ?", liveCode.EntryDomainID).First(&entryDomain).Error; err == nil {
				liveCode.EntryDomain = &entryDomain
			}
		}

		if liveCode.LandingDomainID > 0 {
			var landingDomain model.DomainPool
			if err := r.db.Where("id = ?", liveCode.LandingDomainID).First(&landingDomain).Error; err == nil {
				liveCode.LandingDomain = &landingDomain
			}
		}
	}

	return liveCodes, total, nil
}
