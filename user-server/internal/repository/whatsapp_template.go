package repository

import (
	"context"
	"hivemtk-user/internal/model"
	_db "hivemtk-user/internal/pkg/db"

	"gorm.io/gorm"
)

// WhatsappTemplateRepository WhatsApp 消息模板仓库
type WhatsappTemplateRepository struct {
	db *gorm.DB
}

// NewWhatsappTemplateRepository 创建 WhatsApp 消息模板仓库实例
func NewWhatsappTemplateRepository() *WhatsappTemplateRepository {
	return &WhatsappTemplateRepository{db: _db.GetDB()}
}

// NewWhatsappTemplateRepositoryWithDB 创建指定数据库连接的实例（用于测试）
func NewWhatsappTemplateRepositoryWithDB(db *gorm.DB) *WhatsappTemplateRepository {
	return &WhatsappTemplateRepository{db: db}
}

// SetDB 注入 db（用于测试）
func (r *WhatsappTemplateRepository) SetDB(ctx context.Context, db *gorm.DB) {
	if db != nil {
		r.db = db
	}
}

// Create 创建模板
func (r *WhatsappTemplateRepository) Create(ctx context.Context, template *model.WhatsappMessageTemplate) error {
	return r.db.Create(template).Error
}

// Save 保存模板（全字段更新）
func (r *WhatsappTemplateRepository) Save(ctx context.Context, template *model.WhatsappMessageTemplate) error {
	return r.db.Save(template).Error
}

// GetByID 按 ID 获取模板
func (r *WhatsappTemplateRepository) GetByID(ctx context.Context, id string) (*model.WhatsappMessageTemplate, error) {
	var template model.WhatsappMessageTemplate
	err := r.db.Where("id = ?", id).First(&template).Error
	if err != nil {
		return nil, err
	}
	return &template, nil
}

// ListByFilters 按条件列出模板
func (r *WhatsappTemplateRepository) ListByFilters(ctx context.Context, category string, isActive *bool) ([]*model.WhatsappMessageTemplate, error) {
	var templates []*model.WhatsappMessageTemplate
	query := r.db.Model(&model.WhatsappMessageTemplate{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if isActive != nil {
		query = query.Where("is_active = ?", *isActive)
	}
	if err := query.Order("created_at DESC").Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

// DeleteByID 按 ID 删除模板，返回受影响行数
func (r *WhatsappTemplateRepository) DeleteByID(ctx context.Context, id string) (int64, error) {
	result := r.db.Where("id = ?", id).Delete(&model.WhatsappMessageTemplate{})
	return result.RowsAffected, result.Error
}

