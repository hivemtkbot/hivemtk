package repository

import (
	"context"
	"time"

	"hivemtk-user/internal/geo/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// GeoAlertRepository 负面告警仓储
type GeoAlertRepository interface {
	Create(ctx context.Context, alert *model.GeoAlert) error
	List(ctx context.Context, alertType, level string, page, limit int) ([]*model.GeoAlert, int64, error)
	MarkNotified(ctx context.Context, id uint) error
	DeleteBefore(ctx context.Context, before time.Time) error
	CountUnread(ctx context.Context) (int64, error)
	Delete(ctx context.Context, id uint) error
}

type geoAlertRepo struct {
	db *gorm.DB
}

func NewGeoAlertRepository() GeoAlertRepository {
	return &geoAlertRepo{db: _db.GetDB()}
}

// NewGeoAlertRepositoryWithDB 显式注入 gormDB（路由装配用，测试可替换）
func NewGeoAlertRepositoryWithDB(db *gorm.DB) GeoAlertRepository {
	return &geoAlertRepo{db: db}
}

func (r *geoAlertRepo) Create(ctx context.Context, alert *model.GeoAlert) error {
	return r.db.WithContext(ctx).Create(alert).Error
}

func (r *geoAlertRepo) List(ctx context.Context, alertType, level string, page, limit int) ([]*model.GeoAlert, int64, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := r.db.WithContext(ctx).Model(&model.GeoAlert{})
	if alertType != "" {
		q = q.Where("type = ?", alertType)
	}
	if level != "" {
		q = q.Where("level = ?", level)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.GeoAlert
	if err := q.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *geoAlertRepo) MarkNotified(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Model(&model.GeoAlert{}).
		Where("id = ?", id).
		Update("notified", true).Error
}

func (r *geoAlertRepo) DeleteBefore(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&model.GeoAlert{}).Error
}

// CountUnread 未确认（未通知）告警数，供前端角标
func (r *geoAlertRepo) CountUnread(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.GeoAlert{}).
		Where("notified = ?", false).
		Count(&n).Error
	return n, err
}

// Delete 单条软删
func (r *geoAlertRepo) Delete(ctx context.Context, id uint) error {
	res := r.db.WithContext(ctx).Delete(&model.GeoAlert{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
