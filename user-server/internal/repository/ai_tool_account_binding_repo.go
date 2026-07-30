package repository

import (
	"marketing/internal/model"

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
func (r *AIToolAccountBindingRepository) ListByTool(toolName string) ([]model.AIToolAccountBinding, error) {
	var bindings []model.AIToolAccountBinding
	err := r.db.Where("tool_name = ?", toolName).Find(&bindings).Error
	return bindings, err
}

// Get 获取绑定关系
func (r *AIToolAccountBindingRepository) Get(toolName, accountType, accountID string) (*model.AIToolAccountBinding, error) {
	var binding model.AIToolAccountBinding
	err := r.db.Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// Create 创建绑定关系
func (r *AIToolAccountBindingRepository) Create(binding *model.AIToolAccountBinding) error {
	return r.db.Create(binding).Error
}

// Delete 删除绑定关系
func (r *AIToolAccountBindingRepository) Delete(toolName, accountType, accountID string) error {
	return r.db.Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).Delete(&model.AIToolAccountBinding{}).Error
}

// SetPrimary 设置主账号
func (r *AIToolAccountBindingRepository) SetPrimary(toolName, accountType, accountID string) error {
	// 先取消该工具该类型的所有主账号
	err := r.db.Model(&model.AIToolAccountBinding{}).
		Where("tool_name = ? AND account_type = ?", toolName, accountType).
		Update("is_primary", false).Error
	if err != nil {
		return err
	}
	// 再设置指定账号为主账号
	return r.db.Model(&model.AIToolAccountBinding{}).
		Where("tool_name = ? AND account_type = ? AND account_id = ?", toolName, accountType, accountID).
		Update("is_primary", true).Error
}

// GetPrimary 获取主账号
func (r *AIToolAccountBindingRepository) GetPrimary(toolName, accountType string) (*model.AIToolAccountBinding, error) {
	var binding model.AIToolAccountBinding
	err := r.db.Where("tool_name = ? AND account_type = ? AND is_primary = true", toolName, accountType).First(&binding).Error
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

// ListByAccountType 按账号类型获取绑定的工具
func (r *AIToolAccountBindingRepository) ListByAccountType(accountType string) ([]model.AIToolAccountBinding, error) {
	var bindings []model.AIToolAccountBinding
	err := r.db.Where("account_type = ?", accountType).Find(&bindings).Error
	return bindings, err
}