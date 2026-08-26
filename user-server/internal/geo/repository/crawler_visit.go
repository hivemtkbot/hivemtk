package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"

	"gorm.io/gorm"
)

// GeoCrawlerVisitRepository 爬虫访问仓储
type GeoCrawlerVisitRepository interface {
	Create(ctx context.Context, v *model.GeoCrawlerVisit) error
	StatsByEngine(ctx context.Context, days int) (map[string]int64, error)
}

type geoCrawlerVisitRepo struct{ db *gorm.DB }

func NewGeoCrawlerVisitRepository(db *gorm.DB) GeoCrawlerVisitRepository {
	return &geoCrawlerVisitRepo{db: db}
}

func (r *geoCrawlerVisitRepo) Create(ctx context.Context, v *model.GeoCrawlerVisit) error {
	return r.db.WithContext(ctx).Create(v).Error
}

func (r *geoCrawlerVisitRepo) StatsByEngine(ctx context.Context, days int) (map[string]int64, error) {
	since := time.Now().AddDate(0, 0, -days)
	type row struct {
		Engine string
		N      int64
	}
	var rows []row
	err := r.db.WithContext(ctx).Model(&model.GeoCrawlerVisit{}).
		Select("engine, COUNT(*) as n").
		Where("created_at >= ?", since).
		Group("engine").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Engine] = r.N
	}
	return out, nil
}
