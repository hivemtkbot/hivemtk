package repository

import (
	contentmodel "hivemtk-user/internal/content/model"
	"hivemtk-user/internal/ops/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// DashboardScreenRepository 数据大屏仓库
type DashboardScreenRepository struct {
	db *gorm.DB
}

// NewDashboardScreenRepository 创建数据大屏仓库实例
func NewDashboardScreenRepository() *DashboardScreenRepository {
	return &DashboardScreenRepository{
		db: _db.GetDB(),
	}
}

// Create 创建大屏
func (r *DashboardScreenRepository) Create(screen *model.DashboardScreen) error {
	return r.db.Create(screen).Error
}

// GetByID 根据 ID 获取大屏
func (r *DashboardScreenRepository) GetByID(id uint) (*model.DashboardScreen, error) {
	var screen model.DashboardScreen
	err := r.db.First(&screen, id).Error
	return &screen, err
}

// GetAll 获取所有大屏列表
func (r *DashboardScreenRepository) GetAll(page, pageSize int) ([]*model.DashboardScreen, int64, error) {
	var screens []*model.DashboardScreen
	var total int64

	r.db.Model(&model.DashboardScreen{}).Count(&total)
	err := r.db.
		Order("created_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&screens).Error

	return screens, total, err
}

// GetByCode 根据编码获取大屏
func (r *DashboardScreenRepository) GetByCode(code string) (*model.DashboardScreen, error) {
	var screen model.DashboardScreen
	err := r.db.Where("code = ?", code).First(&screen).Error
	return &screen, err
}

// Update 更新大屏
func (r *DashboardScreenRepository) Update(screen *model.DashboardScreen) error {
	return r.db.Save(screen).Error
}

// Delete 删除大屏
func (r *DashboardScreenRepository) Delete(id uint) error {
	return r.db.Delete(&model.DashboardScreen{}, id).Error
}

// IncrementViewCount 增加访问次数
func (r *DashboardScreenRepository) IncrementViewCount(id uint) error {
	return r.db.Model(&model.DashboardScreen{}).Where("id = ?", id).Update("view_count", gorm.Expr("view_count + 1")).Error
}

// DashboardWidgetRepository 大屏 Widget 仓库
type DashboardWidgetRepository struct {
	db *gorm.DB
}

// NewDashboardWidgetRepository 创建大屏 Widget 仓库实例
func NewDashboardWidgetRepository() *DashboardWidgetRepository {
	return &DashboardWidgetRepository{
		db: _db.GetDB(),
	}
}

// Create 创建 Widget
func (r *DashboardWidgetRepository) Create(widget *model.DashboardWidget) error {
	return r.db.Create(widget).Error
}

// GetByScreenID 根据大屏 ID 获取 Widgets
func (r *DashboardWidgetRepository) GetByScreenID(screenID uint) ([]*model.DashboardWidget, error) {
	var widgets []*model.DashboardWidget
	err := r.db.Where("screen_id = ?", screenID).Order("sort_order, y, x").Find(&widgets).Error
	return widgets, err
}

// DeleteByScreenID 删除大屏下所有 Widgets
func (r *DashboardWidgetRepository) DeleteByScreenID(screenID uint) error {
	return r.db.Where("screen_id = ?", screenID).Delete(&model.DashboardWidget{}).Error
}

// MarketTemplateRepository 模板市场仓库
type MarketTemplateRepository struct {
	db *gorm.DB
}

// NewMarketTemplateRepository 创建模板市场仓库实例
func NewMarketTemplateRepository() *MarketTemplateRepository {
	return &MarketTemplateRepository{
		db: _db.GetDB(),
	}
}

// GetList 获取模板列表
func (r *MarketTemplateRepository) GetList(category, templateType string, page, pageSize int) ([]*contentmodel.MarketTemplate, int64, error) {
	var templates []*contentmodel.MarketTemplate
	var total int64

	query := r.db.Model(&contentmodel.MarketTemplate{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if templateType != "" {
		query = query.Where("type = ?", templateType)
	}

	query.Count(&total)
	err := query.Order("download_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// GetByID 根据 ID 获取模板
func (r *MarketTemplateRepository) GetByID(id uint) (*contentmodel.MarketTemplate, error) {
	var template contentmodel.MarketTemplate
	err := r.db.First(&template, id).Error
	return &template, err
}

// IncrementDownload 增加下载次数
func (r *MarketTemplateRepository) IncrementDownload(id uint) error {
	return r.db.Model(&contentmodel.MarketTemplate{}).Where("id = ?", id).Update("download_count", gorm.Expr("download_count + 1")).Error
}

// GetOfficialTemplates 获取官方模板
func (r *MarketTemplateRepository) GetOfficialTemplates(page, pageSize int) ([]*contentmodel.MarketTemplate, int64, error) {
	var templates []*contentmodel.MarketTemplate
	var total int64

	r.db.Model(&contentmodel.MarketTemplate{}).Where("is_official = ?", true).Count(&total)
	err := r.db.Where("is_official = ?", true).
		Order("download_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// SearchTemplates 搜索模板
// 使用 ILIKE 实现大小写不敏感的搜索（PostgreSQL 原生支持）。
func (r *MarketTemplateRepository) SearchTemplates(keyword string, page, pageSize int) ([]*contentmodel.MarketTemplate, int64, error) {
	var templates []*contentmodel.MarketTemplate
	var total int64

	pattern := "%" + keyword + "%"
	query := r.db.Model(&contentmodel.MarketTemplate{}).
		Where("name ILIKE ? OR description ILIKE ?", pattern, pattern)
	query.Count(&total)
	err := query.Order("download_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// Create 创建模板
func (r *MarketTemplateRepository) Create(template *contentmodel.MarketTemplate) error {
	return r.db.Create(template).Error
}

// UpdateRating 更新模板评分（基于新评分重新计算平均评分）
func (r *MarketTemplateRepository) UpdateRating(id uint, rating float64) error {
	return r.db.Model(&contentmodel.MarketTemplate{}).Where("id = ?", id).
		Update("rating", rating).Error
}

// MarketTemplateDownloadRepository 模板下载记录仓库
type MarketTemplateDownloadRepository struct {
	db *gorm.DB
}

// NewMarketTemplateDownloadRepository 创建模板下载记录仓库实例
func NewMarketTemplateDownloadRepository() *MarketTemplateDownloadRepository {
	return &MarketTemplateDownloadRepository{
		db: _db.GetDB(),
	}
}

// Create 创建下载记录
func (r *MarketTemplateDownloadRepository) Create(record *contentmodel.MarketTemplateDownload) error {
	return r.db.Create(record).Error
}

// GetAll 获取所有下载记录
func (r *MarketTemplateDownloadRepository) GetAll(page, pageSize int) ([]*contentmodel.MarketTemplateDownload, int64, error) {
	var records []*contentmodel.MarketTemplateDownload
	var total int64

	r.db.Model(&contentmodel.MarketTemplateDownload{}).Count(&total)
	err := r.db.
		Order("downloaded_at DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&records).Error

	return records, total, err
}
