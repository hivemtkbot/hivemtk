package repository

import (
	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// CustomReportRepository 自定义报表仓库
type CustomReportRepository struct {
	db *gorm.DB
}

// NewCustomReportRepository 创建自定义报表仓库
func NewCustomReportRepository() *CustomReportRepository {
	return &CustomReportRepository{
		db: _db.GetDB(),
	}
}

// Create 创建报表
func (r *CustomReportRepository) Create(report *model.CustomReport) error {
	return r.db.Create(report).Error
}

// GetByID 根据 ID 获取报表
func (r *CustomReportRepository) GetByID(id uint) (*model.CustomReport, error) {
	var report model.CustomReport
	err := r.db.First(&report, id).Error
	if err != nil {
		return nil, err
	}
	return &report, nil
}

// GetAll 获取所有报表列表
func (r *CustomReportRepository) GetAll(page, pageSize int) ([]*model.CustomReport, int64, error) {
	var reports []*model.CustomReport
	var total int64

	offset := (page - 1) * pageSize

	r.db.Model(&model.CustomReport{}).Count(&total)

	err := r.db.
		Order("created_at DESC").
		Limit(pageSize).Offset(offset).
		Find(&reports).Error
	if err != nil {
		return nil, 0, err
	}

	return reports, total, nil
}

// GetByDataSource 根据数据源获取报表(排除公开模板)
func (r *CustomReportRepository) GetByDataSource(dataSource string) ([]*model.CustomReport, error) {
	var reports []*model.CustomReport
	err := r.db.Where("data_source = ? AND is_public = ?", dataSource, false).
		Order("created_at DESC").
		Find(&reports).Error
	return reports, err
}

// Update 更新报表
func (r *CustomReportRepository) Update(report *model.CustomReport) error {
	return r.db.Save(report).Error
}

// Delete 删除报表
func (r *CustomReportRepository) Delete(id uint) error {
	return r.db.Where("id = ?", id).Delete(&model.CustomReport{}).Error
}

// GetPublicTemplates 获取公开报表模板
func (r *CustomReportRepository) GetPublicTemplates() ([]*model.CustomReport, error) {
	var reports []*model.CustomReport
	err := r.db.Where("is_public = ?", true).
		Order("created_at DESC").
		Find(&reports).Error
	return reports, err
}

// UseTemplate 使用模板创建报表
func (r *CustomReportRepository) UseTemplate(templateID uint, createdBy uint) (*model.CustomReport, error) {
	var template model.CustomReport
	err := r.db.First(&template, templateID).Error
	if err != nil {
		return nil, err
	}

	report := &model.CustomReport{
		Name:        template.Name,
		Description: template.Description,
		DataSource:  template.DataSource,
		Dimensions:  template.Dimensions,
		Metrics:     template.Metrics,
		Filters:     template.Filters,
		ChartType:   template.ChartType,
		ChartConfig: template.ChartConfig,
		IsPublic:    false,
		CreatedBy:   createdBy,
	}

	err = r.db.Create(report).Error
	return report, err
}

// NewCustomReportRepositoryWithDB 创建指定数据库连接的自定义报表仓库实例（用于测试）
func NewCustomReportRepositoryWithDB(db *gorm.DB) *CustomReportRepository {
	return &CustomReportRepository{db: db}
}

