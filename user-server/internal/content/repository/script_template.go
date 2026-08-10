package repository

import (
	"hivemtk-user/internal/content/model"
	_db "hivemtk-user/internal/pkg/db"
	"time"

	"gorm.io/gorm"
)

// ScriptTemplateRepository 话术模板仓库
type ScriptTemplateRepository struct {
	db *gorm.DB
}

// NewScriptTemplateRepository 创建话术模板仓库实例
func NewScriptTemplateRepository() *ScriptTemplateRepository {
	return &ScriptTemplateRepository{
		db: _db.GetDB(),
	}
}

// Create 创建话术模板
func (r *ScriptTemplateRepository) Create(template *model.ScriptTemplate) error {
	return r.db.Create(template).Error
}

// GetByID 根据 ID 获取话术模板
func (r *ScriptTemplateRepository) GetByID(id uint) (*model.ScriptTemplate, error) {
	var template model.ScriptTemplate
	err := r.db.First(&template, id).Error
	return &template, err
}

// GetAll 获取所有话术模板列表
func (r *ScriptTemplateRepository) GetAll(category string, page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	var templates []*model.ScriptTemplate
	var total int64

	query := r.db.Model(&model.ScriptTemplate{})
	if category != "" {
		query = query.Where("category = ?", category)
	}

	query.Count(&total)
	err := query.Order("usage_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// Update 更新话术模板
func (r *ScriptTemplateRepository) Update(template *model.ScriptTemplate) error {
	return r.db.Save(template).Error
}

// Delete 删除话术模板
func (r *ScriptTemplateRepository) Delete(id uint) error {
	return r.db.Delete(&model.ScriptTemplate{}, id).Error
}

// IncrementUsage 增加使用次数
func (r *ScriptTemplateRepository) IncrementUsage(id uint) error {
	return r.db.Model(&model.ScriptTemplate{}).Where("id = ?", id).Update("usage_count", gorm.Expr("usage_count + 1")).Error
}

// GetPublicTemplates 获取公开话术模板
func (r *ScriptTemplateRepository) GetPublicTemplates(page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	var templates []*model.ScriptTemplate
	var total int64

	r.db.Model(&model.ScriptTemplate{}).Where("is_public = ?", true).Count(&total)
	err := r.db.Where("is_public = ?", true).
		Order("usage_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// SearchTemplates 搜索话术模板
func (r *ScriptTemplateRepository) SearchTemplates(keyword string, page, pageSize int) ([]*model.ScriptTemplate, int64, error) {
	var templates []*model.ScriptTemplate
	var total int64

	query := r.db.Model(&model.ScriptTemplate{}).
		Where("title LIKE ? OR content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	query.Count(&total)
	err := query.Order("usage_count DESC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&templates).Error

	return templates, total, err
}

// ScriptCategoryRepository 话术分类仓库
type ScriptCategoryRepository struct {
	db *gorm.DB
}

// NewScriptCategoryRepository 创建话术分类仓库实例
func NewScriptCategoryRepository() *ScriptCategoryRepository {
	return &ScriptCategoryRepository{
		db: _db.GetDB(),
	}
}

// Create 创建分类
func (r *ScriptCategoryRepository) Create(category *model.ScriptCategory) error {
	return r.db.Create(category).Error
}

// GetAll 获取所有分类列表
func (r *ScriptCategoryRepository) GetAll() ([]*model.ScriptCategory, error) {
	var categories []*model.ScriptCategory
	err := r.db.Order("sort_order, id").Find(&categories).Error
	return categories, err
}

// Update 更新分类
func (r *ScriptCategoryRepository) Update(category *model.ScriptCategory) error {
	return r.db.Save(category).Error
}

// Delete 删除分类
func (r *ScriptCategoryRepository) Delete(id uint) error {
	return r.db.Delete(&model.ScriptCategory{}, id).Error
}

// ScriptRecommendRepository 话术推荐仓库
type ScriptRecommendRepository struct {
	db *gorm.DB
}

// NewScriptRecommendRepository 创建话术推荐仓库实例
func NewScriptRecommendRepository() *ScriptRecommendRepository {
	return &ScriptRecommendRepository{
		db: _db.GetDB(),
	}
}

// Create 创建推荐记录
func (r *ScriptRecommendRepository) Create(record *model.ScriptRecommend) error {
	return r.db.Create(record).Error
}

// GetBySessionID 根据会话 ID 获取推荐记录
func (r *ScriptRecommendRepository) GetBySessionID(sessionID string) ([]*model.ScriptRecommend, error) {
	var records []*model.ScriptRecommend
	err := r.db.Where("session_id = ?", sessionID).Order("created_at DESC").Find(&records).Error
	return records, err
}

// MarkAsUsed 标记为已使用
func (r *ScriptRecommendRepository) MarkAsUsed(id uint) error {
	now := make(map[string]any)
	now["is_used"] = true
	now["used_at"] = time.Now()
	return r.db.Model(&model.ScriptRecommend{}).Where("id = ?", id).Updates(now).Error
}
