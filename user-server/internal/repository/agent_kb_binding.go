package repository

import (
	"context"
	"errors"

	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// AgentKBBindingRepository 智能体知识库绑定仓储
type AgentKBBindingRepository struct {
	db *gorm.DB
}

// NewAgentKBBindingRepository 创建仓储
func NewAgentKBBindingRepository(db *gorm.DB) *AgentKBBindingRepository {
	return &AgentKBBindingRepository{db: db}
}

// Create 新增绑定
//
// 注意: UNIQUE(agent_id, kb_id) 约束, 重复绑定会返回错误
func (r *AgentKBBindingRepository) Create(ctx context.Context, b *model.AgentKBBinding) error {
	return r.db.WithContext(ctx).Select("*").Create(b).Error
}

// Delete 按 ID 删除
func (r *AgentKBBindingRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.AgentKBBinding{}).Error
}

// DeleteByAgentKB 按 (agent_id, kb_id) 删除
func (r *AgentKBBindingRepository) DeleteByAgentKB(ctx context.Context, agentID, kbID uint) error {
	return r.db.WithContext(ctx).
		Where("agent_id = ? AND kb_id = ?", agentID, kbID).
		Delete(&model.AgentKBBinding{}).Error
}

// DeleteByAgentAndKB 按 (agent_id, kb_id) 删除 (DeleteByAgentKB 的别名, 兼容老 service 调用)
func (r *AgentKBBindingRepository) DeleteByAgentAndKB(ctx context.Context, agentID, kbID uint) error {
	return r.DeleteByAgentKB(ctx, agentID, kbID)
}

// DeleteByAgent 删除某智能体的所有绑定 (业务级联: 智能体删除时调用)
func (r *AgentKBBindingRepository) DeleteByAgent(ctx context.Context, agentID uint) error {
	if agentID == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Delete(&model.AgentKBBinding{}).Error
}

// DeleteByKB 删除某知识库的所有绑定 (业务级联: 知识库删除时调用)
func (r *AgentKBBindingRepository) DeleteByKB(ctx context.Context, kbID uint) error {
	if kbID == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("kb_id = ?", kbID).
		Delete(&model.AgentKBBinding{}).Error
}

// ListByAgent 列出某智能体的全部绑定 (按 priority DESC 排序)
// kbType 为空时不过滤类型
func (r *AgentKBBindingRepository) ListByAgent(ctx context.Context, agentID uint, kbType string) ([]model.AgentKBBinding, error) {
	q := r.db.WithContext(ctx).Where("agent_id = ? AND enabled = ?", agentID, true)
	if kbType != "" {
		q = q.Where("kb_type = ?", kbType)
	}
	var bindings []model.AgentKBBinding
	err := q.Order("priority DESC, id DESC").Find(&bindings).Error
	return bindings, err
}

// ListByAgentAll 列出某智能体的全部绑定 (不限制 enabled, 不过滤 kb_type, 业务级联删除用)
func (r *AgentKBBindingRepository) ListByAgentAll(ctx context.Context, agentID uint) ([]model.AgentKBBinding, error) {
	var bindings []model.AgentKBBinding
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("priority DESC, id DESC").
		Find(&bindings).Error
	return bindings, err
}

// ListByKB 列出引用某 KB 的全部智能体绑定
func (r *AgentKBBindingRepository) ListByKB(ctx context.Context, kbID uint) ([]model.AgentKBBinding, error) {
	var bindings []model.AgentKBBinding
	err := r.db.WithContext(ctx).
		Where("kb_id = ? AND enabled = ?", kbID, true).
		Order("priority DESC, id DESC").
		Find(&bindings).Error
	return bindings, err
}

// CheckExists 检查 (agent, kb) 是否已绑定
func (r *AgentKBBindingRepository) CheckExists(ctx context.Context, agentID, kbID uint) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AgentKBBinding{}).
		Where("agent_id = ? AND kb_id = ?", agentID, kbID).
		Count(&count).Error
	return count > 0, err
}

// GetByAgentKB 获取单条绑定
func (r *AgentKBBindingRepository) GetByAgentKB(ctx context.Context, agentID, kbID uint) (*model.AgentKBBinding, error) {
	var b model.AgentKBBinding
	if err := r.db.WithContext(ctx).
		Where("agent_id = ? AND kb_id = ?", agentID, kbID).
		First(&b).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// Update 更新绑定
func (r *AgentKBBindingRepository) Update(ctx context.Context, id uint, b *model.AgentKBBinding) error {
	return r.db.WithContext(ctx).Model(&model.AgentKBBinding{}).
		Where("id = ?", id).
		Select("*").Updates(b).Error
}
