package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// ObsConfigRepository OBS 配置仓储接口
//
// 全局统一走 is_default 列（无 License 关联接口）。
type ObsConfigRepository interface {
	Create(ctx context.Context, config *model.ObsConfig) error
	GetByID(ctx context.Context, id string) (*model.ObsConfig, error)
	GetList(ctx context.Context, page int, limit int, provider string, status string) ([]*model.ObsConfig, int64, error)
	Update(ctx context.Context, config *model.ObsConfig) error
	Delete(ctx context.Context, id string) error
	GetDefault(ctx context.Context) (*model.ObsConfig, error)
	SetDefault(ctx context.Context, id string) error
	ClearDefault(ctx context.Context) error
	UpdateStatus(ctx context.Context, id string, status model.ObsStatus) error
	CountByStatus(ctx context.Context, status model.ObsStatus) (int64, error)
	Count(ctx context.Context) (int64, error)
}

type obsConfigRepo struct {
	db *gorm.DB
}

func NewObsConfigRepository() ObsConfigRepository {
	return &obsConfigRepo{db: _db.GetDB()}
}

func NewObsConfigRepositoryWithDB(db *gorm.DB) ObsConfigRepository {
	return &obsConfigRepo{db: db}
}

func (r *obsConfigRepo) Create(ctx context.Context, config *model.ObsConfig) error {
	return r.db.Create(config).Error
}

func (r *obsConfigRepo) GetByID(ctx context.Context, id string) (*model.ObsConfig, error) {
	var config model.ObsConfig
	err := r.db.Where("id = ?", id).First(&config).Error
	return &config, err
}

func (r *obsConfigRepo) GetList(ctx context.Context, page int, limit int, provider string, status string) ([]*model.ObsConfig, int64, error) {
	var configs []*model.ObsConfig
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.ObsConfig{})

	if provider != "" {
		query = query.Where("provider = ?", provider)
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&configs).Error
	return configs, total, err
}

func (r *obsConfigRepo) Update(ctx context.Context, config *model.ObsConfig) error {
	return r.db.Save(config).Error
}

func (r *obsConfigRepo) Delete(ctx context.Context, id string) error {
	return r.db.Where("id = ?", id).Delete(&model.ObsConfig{}).Error
}

func (r *obsConfigRepo) GetDefault(ctx context.Context) (*model.ObsConfig, error) {
	var config model.ObsConfig
	err := r.db.Where("is_default = ?", true).First(&config).Error
	return &config, err
}

func (r *obsConfigRepo) SetDefault(ctx context.Context, id string) error {
	err := r.ClearDefault(ctx)
	if err != nil {
		return err
	}

	return r.db.Model(&model.ObsConfig{}).Where("id = ?", id).Update("is_default", true).Error
}

func (r *obsConfigRepo) ClearDefault(ctx context.Context) error {
	return r.db.Model(&model.ObsConfig{}).Where("is_default = ?", true).Update("is_default", false).Error
}

func (r *obsConfigRepo) UpdateStatus(ctx context.Context, id string, status model.ObsStatus) error {
	return r.db.Model(&model.ObsConfig{}).Where("id = ?", id).Update("status", status).Error
}

func (r *obsConfigRepo) CountByStatus(ctx context.Context, status model.ObsStatus) (int64, error) {
	var count int64
	err := r.db.Model(&model.ObsConfig{}).Where("status = ?", status).Count(&count).Error
	return count, err
}

func (r *obsConfigRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.Model(&model.ObsConfig{}).Count(&count).Error
	return count, err
}

