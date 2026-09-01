package repository

import (
	"context"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoCompetitorRepository 竞品仓储
type GeoCompetitorRepository interface {
	List(ctx context.Context, status string) ([]*model.GeoCompetitor, error)
	GetByID(ctx context.Context, id uint) (*model.GeoCompetitor, error)
	Create(ctx context.Context, c *model.GeoCompetitor) error
	Update(ctx context.Context, c *model.GeoCompetitor) error
	Delete(ctx context.Context, id uint) error
	ListActive(ctx context.Context) ([]*model.GeoCompetitor, error)
}

type geoCompetitorRepo struct{ db *gorm.DB }

func NewGeoCompetitorRepository() GeoCompetitorRepository {
	return &geoCompetitorRepo{db: _db.GetDB()}
}

func (r *geoCompetitorRepo) List(ctx context.Context, status string) ([]*model.GeoCompetitor, error) {
	q := r.db.WithContext(ctx).Model(&model.GeoCompetitor{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var list []*model.GeoCompetitor
	err := q.Order("priority DESC, id ASC").Find(&list).Error
	return list, err
}

func (r *geoCompetitorRepo) GetByID(ctx context.Context, id uint) (*model.GeoCompetitor, error) {
	var c model.GeoCompetitor
	err := r.db.WithContext(ctx).First(&c, id).Error
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *geoCompetitorRepo) Create(ctx context.Context, c *model.GeoCompetitor) error {
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *geoCompetitorRepo) Update(ctx context.Context, c *model.GeoCompetitor) error {
	return r.db.WithContext(ctx).Save(c).Error
}

func (r *geoCompetitorRepo) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.GeoCompetitor{}, id).Error
}

func (r *geoCompetitorRepo) ListActive(ctx context.Context) ([]*model.GeoCompetitor, error) {
	return r.List(ctx, "active")
}
