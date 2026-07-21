package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type ObsConfigRepository interface {
	Create(config *model.ObsConfig) error
	GetByID(id string) (*model.ObsConfig, error)
	GetList(page int, limit int, provider string, status string) ([]*model.ObsConfig, int64, error)
	GetListByLicense(licenseID string, page int, limit int) ([]*model.ObsConfig, int64, error)
	Update(config *model.ObsConfig) error
	Delete(id string) error
	GetDefault() (*model.ObsConfig, error)
	GetDefaultByLicense(licenseID string) (*model.ObsConfig, error)
	SetDefault(id string) error
	ClearDefault() error
	ClearDefaultByLicense(licenseID string) error
	UpdateStatus(id string, status model.ObsStatus) error
	CountByStatus(status model.ObsStatus) (int64, error)
	CountByLicense(licenseID string) (int64, error)
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

func (r *obsConfigRepo) Create(config *model.ObsConfig) error {
	return r.db.Create(config).Error
}

func (r *obsConfigRepo) GetByID(id string) (*model.ObsConfig, error) {
	var config model.ObsConfig
	err := r.db.Where("id = ?", id).First(&config).Error
	return &config, err
}

func (r *obsConfigRepo) GetList(page int, limit int, provider string, status string) ([]*model.ObsConfig, int64, error) {
	var configs []*model.ObsConfig
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.ObsConfig{})

	// 提供商筛选
	if provider != "" {
		query = query.Where("provider = ?", provider)
	}

	// 状态筛选
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

func (r *obsConfigRepo) GetListByLicense(licenseID string, page int, limit int) ([]*model.ObsConfig, int64, error) {
	var configs []*model.ObsConfig
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.ObsConfig{}).Where("license_id = ?", licenseID)

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&configs).Error
	return configs, total, err
}

func (r *obsConfigRepo) Update(config *model.ObsConfig) error {
	return r.db.Save(config).Error
}

func (r *obsConfigRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.ObsConfig{}).Error
}

func (r *obsConfigRepo) GetDefault() (*model.ObsConfig, error) {
	var config model.ObsConfig
	err := r.db.Where("is_default = ?", true).First(&config).Error
	return &config, err
}

func (r *obsConfigRepo) GetDefaultByLicense(licenseID string) (*model.ObsConfig, error) {
	var config model.ObsConfig
	err := r.db.Where("license_id = ? AND is_default = ?", licenseID, true).First(&config).Error
	return &config, err
}

func (r *obsConfigRepo) SetDefault(id string) error {
	// 先清除现有的默认配置
	err := r.ClearDefault()
	if err != nil {
		return err
	}

	// 设置新的默认配置
	return r.db.Model(&model.ObsConfig{}).Where("id = ?", id).Update("is_default", true).Error
}

func (r *obsConfigRepo) ClearDefault() error {
	return r.db.Model(&model.ObsConfig{}).Where("is_default = ?", true).Update("is_default", false).Error
}

func (r *obsConfigRepo) ClearDefaultByLicense(licenseID string) error {
	return r.db.Model(&model.ObsConfig{}).Where("license_id = ? AND is_default = ?", licenseID, true).Update("is_default", false).Error
}

func (r *obsConfigRepo) UpdateStatus(id string, status model.ObsStatus) error {
	return r.db.Model(&model.ObsConfig{}).Where("id = ?", id).Update("status", status).Error
}

func (r *obsConfigRepo) CountByStatus(status model.ObsStatus) (int64, error) {
	var count int64
	err := r.db.Model(&model.ObsConfig{}).Where("status = ?", status).Count(&count).Error
	return count, err
}

func (r *obsConfigRepo) CountByLicense(licenseID string) (int64, error) {
	var count int64
	err := r.db.Model(&model.ObsConfig{}).Where("license_id = ?", licenseID).Count(&count).Error
	return count, err
}
