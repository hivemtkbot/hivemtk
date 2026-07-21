package repository

import (
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

type LicenseRepository interface {
	Create(license *model.License) error
	GetByID(id string) (*model.License, error)
	GetByKey(key string) (*model.License, error)
	GetList(page int, limit int, status string, search string) ([]*model.License, int64, error)
	Update(license *model.License) error
	Delete(id string) error
	GetActiveLicenses() ([]*model.License, error)
	GetExpiredLicenses() ([]*model.License, error)
	UpdateStatus(id string, status model.LicenseStatus) error
	CountByStatus(status model.LicenseStatus) (int64, error)
}

type licenseRepo struct {
	db *gorm.DB
}

func NewLicenseRepository() LicenseRepository {
	return &licenseRepo{db: _db.GetDB()}
}

func (r *licenseRepo) Create(license *model.License) error {
	return r.db.Create(license).Error
}

func (r *licenseRepo) GetByID(id string) (*model.License, error) {
	var license model.License
	err := r.db.Where("id = ?", id).First(&license).Error
	return &license, err
}

func (r *licenseRepo) GetByKey(key string) (*model.License, error) {
	var license model.License
	err := r.db.Where("key = ?", key).First(&license).Error
	return &license, err
}

func (r *licenseRepo) GetList(page int, limit int, status string, search string) ([]*model.License, int64, error) {
	var licenses []*model.License
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.License{})

	// 状态筛选
	if status != "" {
		query = query.Where("status = ?", status)
	}

	// 搜索筛选
	if search != "" {
		query = query.Where("merchant_name LIKE ? OR contact_email LIKE ? OR key LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&licenses).Error
	return licenses, total, err
}

func (r *licenseRepo) Update(license *model.License) error {
	return r.db.Save(license).Error
}

func (r *licenseRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.License{}).Error
}

func (r *licenseRepo) GetActiveLicenses() ([]*model.License, error) {
	var licenses []*model.License
	now := time.Now()
	err := r.db.Where("status = ? AND expire_at > ?", model.LicenseStatusActive, now).Find(&licenses).Error
	return licenses, err
}

func (r *licenseRepo) GetExpiredLicenses() ([]*model.License, error) {
	var licenses []*model.License
	now := time.Now()
	err := r.db.Where("status = ? AND expire_at <= ?", model.LicenseStatusActive, now).Find(&licenses).Error
	return licenses, err
}

func (r *licenseRepo) UpdateStatus(id string, status model.LicenseStatus) error {
	return r.db.Model(&model.License{}).Where("id = ?", id).Update("status", status).Error
}

func (r *licenseRepo) CountByStatus(status model.LicenseStatus) (int64, error) {
	var count int64
	err := r.db.Model(&model.License{}).Where("status = ?", status).Count(&count).Error
	return count, err
}
