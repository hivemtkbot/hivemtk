package repository

import (
	"hivemtk-user/internal/content/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

type MaterialRepository interface {
	Create(material *model.Material) error
	GetByID(id string) (*model.Material, error)
	GetList(licenseID string, categoryID string, materialType string, search string, page int, limit int) ([]*model.Material, int64, error)
	Update(material *model.Material) error
	Delete(id string) error
	IncrementUsage(id string) error
	GetByHash(hash string, licenseID string) (*model.Material, error)
}

type materialRepo struct {
	db *gorm.DB
}

func NewMaterialRepository() MaterialRepository {
	return &materialRepo{db: _db.GetDB()}
}

func NewMaterialRepositoryWithDB(db *gorm.DB) MaterialRepository {
	return &materialRepo{db: db}
}

func (r *materialRepo) Create(material *model.Material) error {
	return r.db.Create(material).Error
}

func (r *materialRepo) GetByID(id string) (*model.Material, error) {
	var material model.Material
	err := r.db.Preload("Category").Where("id = ?", id).First(&material).Error
	return &material, err
}

func (r *materialRepo) GetList(licenseID string, categoryID string, materialType string, search string, page int, limit int) ([]*model.Material, int64, error) {
	var materials []*model.Material
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.Material{}).Where("license_id = ?", licenseID)

	if categoryID != "" {
		query = query.Where("category_id = ?", categoryID)
	}

	if materialType != "" {
		query = query.Where("type = ?", materialType)
	}

	if search != "" {
		query = query.Where("name LIKE ? OR tags LIKE ? OR description LIKE ?",
			"%"+search+"%", "%"+search+"%", "%"+search+"%")
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Preload("Category").Offset(offset).Limit(limit).Order("created_at DESC").Find(&materials).Error
	return materials, total, err
}

func (r *materialRepo) Update(material *model.Material) error {
	return r.db.Save(material).Error
}

func (r *materialRepo) Delete(id string) error {
	return r.db.Where("id = ?", id).Delete(&model.Material{}).Error
}

func (r *materialRepo) IncrementUsage(id string) error {
	return r.db.Model(&model.Material{}).Where("id = ?", id).Updates(map[string]any{
		"usage_count":  gorm.Expr("usage_count + 1"),
		"last_used_at": gorm.Expr("NOW()"),
	}).Error
}

func (r *materialRepo) GetByHash(hash string, licenseID string) (*model.Material, error) {
	var material model.Material
	err := r.db.Where("hash = ? AND license_id = ?", hash, licenseID).First(&material).Error
	return &material, err
}

