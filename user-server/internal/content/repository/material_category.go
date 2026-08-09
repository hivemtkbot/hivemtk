package repository

import (
	"marketing/internal/content/model"
	_db "marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
)

type MaterialCategoryRepository interface {
	Create(category *model.MaterialCategory) error
	GetByID(id string) (*model.MaterialCategory, error)
	GetList(licenseID string, parentID string, materialType string, page int, limit int) ([]*model.MaterialCategory, int64, error)
	Update(category *model.MaterialCategory) error
	Delete(id string) error
	GetTree(licenseID string, materialType string) ([]*model.MaterialCategory, error)
	UpdateMaterialCount(categoryID string) error
}

type materialCategoryRepo struct {
	db *gorm.DB
}

func NewMaterialCategoryRepository() MaterialCategoryRepository {
	return &materialCategoryRepo{db: _db.GetDB()}
}

func NewMaterialCategoryRepositoryWithDB(db *gorm.DB) MaterialCategoryRepository {
	return &materialCategoryRepo{db: db}
}

func (r *materialCategoryRepo) Create(category *model.MaterialCategory) error {
	return r.db.Create(category).Error
}

func (r *materialCategoryRepo) GetByID(id string) (*model.MaterialCategory, error) {
	var category model.MaterialCategory
	err := r.db.Preload("Parent").Where("id = ?", id).First(&category).Error
	return &category, err
}

func (r *materialCategoryRepo) GetList(licenseID string, parentID string, materialType string, page int, limit int) ([]*model.MaterialCategory, int64, error) {
	var categories []*model.MaterialCategory
	var total int64
	offset := (page - 1) * limit

	query := r.db.Model(&model.MaterialCategory{}).Where("license_id = ?", licenseID)

	// 父级筛选
	if parentID != "" {
		query = query.Where("parent_id = ?", parentID)
	}
	// 类型筛选
	if materialType != "" {
		query = query.Where("type = ?", materialType)
	}

	err := query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = query.Preload("Parent").Offset(offset).Limit(limit).Order("sort ASC, created_at DESC").Find(&categories).Error
	return categories, total, err
}

func (r *materialCategoryRepo) Update(category *model.MaterialCategory) error {
	return r.db.Save(category).Error
}

func (r *materialCategoryRepo) Delete(id string) error {
	// 检查分类是否存在
	_, err := r.GetByID(id)
	if err != nil {
		return err
	}

	// 检查是否有子分类
	var childCount int64
	err = r.db.Model(&model.MaterialCategory{}).Where("parent_id = ?", id).Count(&childCount).Error
	if err != nil {
		return err
	}
	if childCount > 0 {
		return gorm.ErrForeignKeyViolated
	}

	// 检查是否有素材
	var materialCount int64
	err = r.db.Model(&model.Material{}).Where("category_id = ?", id).Count(&materialCount).Error
	if err != nil {
		return err
	}
	if materialCount > 0 {
		return gorm.ErrForeignKeyViolated
	}

	return r.db.Where("id = ?", id).Delete(&model.MaterialCategory{}).Error
}

func (r *materialCategoryRepo) GetTree(licenseID string, materialType string) ([]*model.MaterialCategory, error) {
	var categories []*model.MaterialCategory

	query := r.db.Where("license_id = ?", licenseID)

	if materialType != "" {
		query = query.Where("type = ?", materialType)
	}

	err := query.Preload("Children").Order("sort ASC, created_at DESC").Find(&categories).Error
	if err != nil {
		return nil, err
	}

	// 过滤掉有父级的分类，只返回根级
	var rootCategories []*model.MaterialCategory
	for _, category := range categories {
		if category.ParentID == nil {
			rootCategories = append(rootCategories, category)
		}
	}

	return rootCategories, nil
}

func (r *materialCategoryRepo) UpdateMaterialCount(categoryID string) error {
	var count int64
	err := r.db.Model(&model.Material{}).Where("category_id = ?", categoryID).Count(&count).Error
	if err != nil {
		return err
	}

	return r.db.Model(&model.MaterialCategory{}).Where("id = ?", categoryID).Update("material_count", count).Error
}
