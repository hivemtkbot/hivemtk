package repository

import (
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AIToolConfigRepository AI工具配置仓储
type AIToolConfigRepository struct {
	db *gorm.DB
}

// NewAIToolConfigRepository 创建AI工具配置仓储
func NewAIToolConfigRepository(db *gorm.DB) *AIToolConfigRepository {
	return &AIToolConfigRepository{db: db}
}

// List 获取工具列表
func (r *AIToolConfigRepository) List(category string, enabled *bool, page, pageSize int) ([]model.AIToolConfig, int64, error) {
	var tools []model.AIToolConfig
	var total int64

	query := r.db.Model(&model.AIToolConfig{})

	if category != "" {
		query = query.Where("category = ?", category)
	}
	if enabled != nil {
		query = query.Where("is_enabled = ?", *enabled)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset((page - 1) * pageSize).Limit(pageSize).Order("display_order ASC").Find(&tools).Error; err != nil {
		return nil, 0, err
	}

	return tools, total, nil
}

// GetByName 按名称获取工具
func (r *AIToolConfigRepository) GetByName(name string) (*model.AIToolConfig, error) {
	var tool model.AIToolConfig
	if err := r.db.Where("tool_name = ?", name).First(&tool).Error; err != nil {
		return nil, err
	}
	return &tool, nil
}

// Create 创建工具配置
func (r *AIToolConfigRepository) Create(tool *model.AIToolConfig) error {
	return r.db.Create(tool).Error
}

// Update 更新工具配置
func (r *AIToolConfigRepository) Update(tool *model.AIToolConfig) error {
	return r.db.Save(tool).Error
}

// UpdateStatus 更新工具状态
func (r *AIToolConfigRepository) UpdateStatus(name string, enabled bool) error {
	return r.db.Model(&model.AIToolConfig{}).Where("tool_name = ?", name).Update("is_enabled", enabled).Error
}

// BatchUpdateStatus 批量更新工具状态
func (r *AIToolConfigRepository) BatchUpdateStatus(names []string, enabled bool) error {
	return r.db.Model(&model.AIToolConfig{}).Where("tool_name IN ?", names).Update("is_enabled", enabled).Error
}

// Count 统计工具数量
func (r *AIToolConfigRepository) Count() (int64, error) {
	var count int64
	err := r.db.Model(&model.AIToolConfig{}).Count(&count).Error
	return count, err
}

// CountByCategory 按分类统计
func (r *AIToolConfigRepository) CountByCategory(category string) (int64, error) {
	var count int64
	err := r.db.Model(&model.AIToolConfig{}).Where("category = ?", category).Count(&count).Error
	return count, err
}

// CountEnabled 统计启用数量
func (r *AIToolConfigRepository) CountEnabled() (int64, error) {
	var count int64
	err := r.db.Model(&model.AIToolConfig{}).Where("is_enabled = true").Count(&count).Error
	return count, err
}
