package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoSourceCatalogRepository 信源目录仓储
// SourceCatalogRepository 是同接口别名（service/crawler.go 使用的名字）
type GeoSourceCatalogRepository interface {
	Upsert(ctx context.Context, item *model.GeoSourceCatalog) error
	BatchUpsert(ctx context.Context, items []*model.GeoSourceCatalog) error
	GetByID(ctx context.Context, id uint) (*model.GeoSourceCatalog, error)
	GetByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error)
	// FindByURL crawler.go 使用的名称，同 GetByURL
	FindByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error)
	FindByDomain(ctx context.Context, domain string) ([]*model.GeoSourceCatalog, error)
	List(ctx context.Context, level, category string, page, limit int) ([]*model.GeoSourceCatalog, int64, error)
	ListSeedURLs(ctx context.Context) ([]*model.GeoSourceCatalog, error)
	UpdateLastChecked(ctx context.Context, id uint, t time.Time) error
	Delete(ctx context.Context, id uint) error
}

// SourceCatalogRepository service/crawler.go 使用的别名
type SourceCatalogRepository = GeoSourceCatalogRepository

type geoSourceCatalogRepo struct{ db *gorm.DB }

func NewGeoSourceCatalogRepository() GeoSourceCatalogRepository {
	return &geoSourceCatalogRepo{db: _db.GetDB()}
}
func NewGeoSourceCatalogRepositoryWithDB(db *gorm.DB) GeoSourceCatalogRepository {
	return &geoSourceCatalogRepo{db: db}
}

func (r *geoSourceCatalogRepo) Upsert(ctx context.Context, item *model.GeoSourceCatalog) error {
	return r.db.WithContext(ctx).Where("source_url = ?", item.SourceURL).
		Assign(model.GeoSourceCatalog{
			Domain:      item.Domain,
			Level:       item.Level,
			Category:    item.Category,
			Description: item.Description,
			Verified:    item.Verified,
		}).
		FirstOrCreate(item).Error
}

func (r *geoSourceCatalogRepo) BatchUpsert(ctx context.Context, items []*model.GeoSourceCatalog) error {
	for _, it := range items {
		if err := r.Upsert(ctx, it); err != nil {
			return err
		}
	}
	return nil
}

func (r *geoSourceCatalogRepo) GetByID(ctx context.Context, id uint) (*model.GeoSourceCatalog, error) {
	var m model.GeoSourceCatalog
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&m).Error
	return &m, err
}

func (r *geoSourceCatalogRepo) GetByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error) {
	var m model.GeoSourceCatalog
	err := r.db.WithContext(ctx).Where("source_url = ?", url).First(&m).Error
	return &m, err
}

// FindByURL crawler.go 使用的名称
func (r *geoSourceCatalogRepo) FindByURL(ctx context.Context, url string) (*model.GeoSourceCatalog, error) {
	return r.GetByURL(ctx, url)
}

func (r *geoSourceCatalogRepo) FindByDomain(ctx context.Context, domain string) ([]*model.GeoSourceCatalog, error) {
	var list []*model.GeoSourceCatalog
	err := r.db.WithContext(ctx).
		Where("domain = ? AND verified = true", domain).
		Order("level ASC").
		Find(&list).Error
	return list, err
}

func (r *geoSourceCatalogRepo) List(ctx context.Context, level, category string, page, limit int) ([]*model.GeoSourceCatalog, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	var list []*model.GeoSourceCatalog
	var total int64
	q := r.db.WithContext(ctx).Model(&model.GeoSourceCatalog{})
	if level != "" {
		q = q.Where("level = ?", level)
	}
	if category != "" {
		q = q.Where("category = ?", category)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Offset((page - 1) * limit).Limit(limit).Order("updated_at DESC").Find(&list).Error
	return list, total, err
}

func (r *geoSourceCatalogRepo) ListSeedURLs(ctx context.Context) ([]*model.GeoSourceCatalog, error) {
	var list []*model.GeoSourceCatalog
	err := r.db.WithContext(ctx).
		Where("verified = true").
		Order("level ASC, updated_at DESC").
		Find(&list).Error
	return list, err
}

func (r *geoSourceCatalogRepo) UpdateLastChecked(ctx context.Context, id uint, t time.Time) error {
	return r.db.WithContext(ctx).Model(&model.GeoSourceCatalog{}).
		Where("id = ?", id).Update("last_checked", t).Error
}

func (r *geoSourceCatalogRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.GeoSourceCatalog{}, "id = ?", id).Error
}
