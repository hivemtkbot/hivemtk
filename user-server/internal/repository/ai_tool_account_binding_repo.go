package repository

import (
	"context"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AIToolAccountBindingRepository AI工具-账号绑定仓储
type AIToolAccountBindingRepository struct {
	db *gorm.DB
}

// NewAIToolAccountBindingRepository 创建AI工具-账号绑定仓储
func NewAIToolAccountBindingRepository(db *gorm.DB) *AIToolAccountBindingRepository {
	return &AIToolAccountBindingRepository{db: db}
}

// ListByTool 获取工具绑定的账号列表
func (r *AIToolAccountBindingRepository) ListByTool(ctx context.Context, toolName string) ([]model.AIToolAccountBinding, error) {
	var bindings []model.AIToolAccountBinding
	err := r.db.WithContext(ctx).Where("tool_name = ?", toolName).Find(&bindings).Error
	return bindings, err
}

// Get 获取绑定关系
func (r *AIToolAccountBindingRepository) Get(ctx context.Context, toolName, accountType, accountID string) (*model.AIToolAccountBinding, error) {
	var binding model.AIToolAccountBinding
	err := r.db.WithContext(ctx).Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// Create 创建绑定关系
func (r *AIToolAccountBindingRepository) Create(ctx context.Context, binding *model.AIToolAccountBinding) error {
	return r.db.WithContext(ctx).Create(binding).Error
}

// Delete 删除绑定关系
func (r *AIToolAccountBindingRepository) Delete(ctx context.Context, toolName, accountType, accountID string) error {
	return r.db.WithContext(ctx).Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).Delete(&model.AIToolAccountBinding{}).Error
}

// SetPrimary 设置主账号
func (r *AIToolAccountBindingRepository) SetPrimary(ctx context.Context, toolName, accountType, accountID string) error {
	err := r.db.WithContext(ctx).Model(&model.AIToolAccountBinding{}).
		Where("tool_name = ? AND account_type = ?", toolName, accountType).
		Update("is_primary", false).Error
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&model.AIToolAccountBinding{}).
		Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).
		Update("is_primary", true).Error
}

// GetPrimary 获取主账号
func (r *AIToolAccountBindingRepository) GetPrimary(ctx context.Context, toolName, accountType string) (*model.AIToolAccountBinding, error) {
	var binding model.AIToolAccountBinding
	err := r.db.WithContext(ctx).Where("tool_name = ? AND account_type = ? AND is_primary = true", toolName, accountType).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// ListByAccountType 按账号类型获取绑定的工具
func (r *AIToolAccountBindingRepository) ListByAccountType(ctx context.Context, accountType string) ([]model.AIToolAccountBinding, error) {
	var bindings []model.AIToolAccountBinding
	err := r.db.WithContext(ctx).Where("account_type = ?", accountType).Find(&bindings).Error
	return bindings, err
}

