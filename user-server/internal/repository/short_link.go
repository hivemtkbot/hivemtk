package repository

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// ShortLinkRepository 短链仓储接口
type ShortLinkRepository interface {
	Create(ctx context.Context, shortLink *model.ShortLink) error
	Update(ctx context.Context, shortLink *model.ShortLink) error
	Delete(ctx context.Context, id uint) error
	GetByID(ctx context.Context, id uint) (*model.ShortLink, error)
	GetByShortCode(ctx context.Context, shortCode string) (*model.ShortLink, error)
	GetList(ctx context.Context, page, pageSize int, shortCode, originalURL string, status int) ([]*model.ShortLink, int64, error)
	GetTotalCount(ctx context.Context) (int64, error)
	IncreaseClickCount(ctx context.Context, id uint) error
}

type shortLinkRepository struct {
	db *gorm.DB
}

// NewShortLinkRepository 创建短链仓储实例
func NewShortLinkRepository(db *gorm.DB) ShortLinkRepository {
	return &shortLinkRepository{db: db}
}

func (r *shortLinkRepository) Create(ctx context.Context, shortLink *model.ShortLink) error {
	logger.Infof("创建短链，ShortCode: %s, OriginalURL: %s", shortLink.ShortCode, shortLink.OriginalURL)
	err := r.db.Create(shortLink).Error
	if err != nil {
		logger.Errorf("创建短链失败: %v", err)
		return err
	}
	logger.Infof("创建短链成功，ID: %d", shortLink.ID)
	return nil
}

func (r *shortLinkRepository) Update(ctx context.Context, shortLink *model.ShortLink) error {
	return r.db.Save(shortLink).Error
}

func (r *shortLinkRepository) Delete(ctx context.Context, id uint) error {
	return r.db.Delete(&model.ShortLink{}, id).Error
}

func (r *shortLinkRepository) GetByID(ctx context.Context, id uint) (*model.ShortLink, error) {
	var shortLink model.ShortLink
	err := r.db.First(&shortLink, id).Error
	if err != nil {
		return nil, err
	}
	return &shortLink, nil
}

func (r *shortLinkRepository) GetByShortCode(ctx context.Context, shortCode string) (*model.ShortLink, error) {
	var shortLink model.ShortLink
	err := r.db.Where("short_code = ?", shortCode).First(&shortLink).Error
	if err != nil {
		return nil, err
	}
	return &shortLink, nil
}

func (r *shortLinkRepository) GetList(ctx context.Context, page, pageSize int, shortCode, originalURL string, status int) ([]*model.ShortLink, int64, error) {
	var shortLinks []*model.ShortLink
	var total int64

	query := r.db.Model(&model.ShortLink{})

	if shortCode != "" {
		query = query.Where("short_code LIKE ?", "%"+shortCode+"%")
	}
	if originalURL != "" {
		query = query.Where("original_url LIKE ?", "%"+originalURL+"%")
	}
	if status > 0 {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err = query.Offset(offset).Limit(pageSize).Order("created_at DESC").Find(&shortLinks).Error
	if err != nil {
		return nil, 0, err
	}

	return shortLinks, total, nil
}

func (r *shortLinkRepository) GetTotalCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.Model(&model.ShortLink{}).Count(&count).Error
	return count, err
}

func (r *shortLinkRepository) IncreaseClickCount(ctx context.Context, id uint) error {
	return r.db.Model(&model.ShortLink{}).Where("id = ?", id).UpdateColumn("click_count", gorm.Expr("click_count + ?", 1)).Error
}
