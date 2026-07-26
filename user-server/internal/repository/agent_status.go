package repository

import (
	"context"
	"marketing/internal/model"
	_db "marketing/internal/pkg/utils/db"
	"time"

	"gorm.io/gorm"
)

// AgentStatusRepository 客服状态仓库
type AgentStatusRepository struct {
	db *gorm.DB
}

// NewAgentStatusRepository 创建客服状态仓库实例
func NewAgentStatusRepository() *AgentStatusRepository {
	return &AgentStatusRepository{
		db: _db.GetDB(),
	}
}

// NewAgentStatusRepositoryWithDB 创建指定数据库连接的客服状态仓库实例
func NewAgentStatusRepositoryWithDB(db *gorm.DB) *AgentStatusRepository {
	return &AgentStatusRepository{db: db}
}

// Create 创建客服状态
func (r *AgentStatusRepository) Create(ctx context.Context, status *model.AgentStatus) error {
	return r.db.Create(status).Error
}

// Update 更新客服状态
func (r *AgentStatusRepository) Update(ctx context.Context, status *model.AgentStatus) error {
	return r.db.Save(status).Error
}

// GetByAgentID 根据客服ID获取状态
func (r *AgentStatusRepository) GetByAgentID(ctx context.Context, agentID uint) (*model.AgentStatus, error) {
	var status model.AgentStatus
	err := r.db.Where("agent_id = ?", agentID).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

// GetOnlineAgents 获取在线客服列表
func (r *AgentStatusRepository) GetOnlineAgents(ctx context.Context) ([]*model.AgentStatus, error) {
	var agents []*model.AgentStatus
	err := r.db.Where("status IN ? AND active_sessions < max_sessions", []string{"online", "busy"}).
		Order("active_sessions ASC").Find(&agents).Error
	return agents, err
}

// ListAllAgents 列出全部客服（不分在线/离线），用于客服监管控制台
func (r *AgentStatusRepository) ListAllAgents(ctx context.Context) ([]*model.AgentStatus, error) {
	var agents []*model.AgentStatus
	err := r.db.Order("agent_id ASC").Find(&agents).Error
	return agents, err
}

// UpdateStatus 更新客服状态
func (r *AgentStatusRepository) UpdateStatus(ctx context.Context, agentID uint, status string) error {
	updates := map[string]any{"status": status}
	now := time.Now()
	if status == "online" {
		updates["online_at"] = &now
	} else if status == "offline" {
		updates["offline_at"] = &now
	}
	updates["last_active_at"] = &now
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).Updates(updates).Error
}

// IncrementActiveSessions 增加活跃会话数
func (r *AgentStatusRepository) IncrementActiveSessions(ctx context.Context, agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).
		Updates(map[string]any{
			"active_sessions": gorm.Expr("active_sessions + 1"),
			"today_sessions":  gorm.Expr("today_sessions + 1"),
		}).Error
}

// DecrementActiveSessions 减少活跃会话数
func (r *AgentStatusRepository) DecrementActiveSessions(ctx context.Context, agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ? AND active_sessions > 0", agentID).
		Update("active_sessions", gorm.Expr("active_sessions - 1")).Error
}

// IncrementTodayMessages 增加今日消息数
func (r *AgentStatusRepository) IncrementTodayMessages(ctx context.Context, agentID uint) error {
	return r.db.Model(&model.AgentStatus{}).Where("agent_id = ?", agentID).
		Update("today_messages", gorm.Expr("today_messages + 1")).Error
}

// CountOnlineAgents 统计在线坐席数（status IN ['online', 'busy']）
//
// 用于访客侧 OpenSession / SendMessage 时广播新会话通知前的快速判断：
// 仅返回数量，不返回坐席详情（避免不必要的扫描）。
func (r *AgentStatusRepository) CountOnlineAgents(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AgentStatus{}).
		Where("status IN ?", []string{"online", "busy"}).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}
