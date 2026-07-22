package migrations

import (
	"context"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// ============================================================================
// ai_agents 表扩展迁移
// ----------------------------------------------------------------------------
// 设计依据：ADR-008 §2.3
// 背景：智能体需要挂载"多逻辑"——除已有 sop_ids / script_library_ids 外，
//       新增 decision_strategy_ids（决策策略） / ab_experiment_ids（A/B 实验）。
// 私域独立部署：无 merchant_id 字段
// 兼容性：仅新增 2 字段，已存在数据无影响
// ============================================================================

// AIAgentExtensionMigration ai_agents 表扩展迁移
type AIAgentExtensionMigration struct {
	db *gorm.DB
}

// NewAIAgentExtensionMigration 创建迁移实例
func NewAIAgentExtensionMigration(db *gorm.DB) *AIAgentExtensionMigration {
	return &AIAgentExtensionMigration{db: db}
}

// Version 返回版本号（必须全局唯一）
func (m *AIAgentExtensionMigration) Version() string {
	return "v2.2.1"
}

// Name 返回迁移名称
func (m *AIAgentExtensionMigration) Name() string {
	return "ai_agents 扩展 2 字段（决策策略 / A/B 实验）"
}

// Description 返回迁移描述
func (m *AIAgentExtensionMigration) Description() string {
	return "为 ai_agents 表新增 decision_strategy_ids / ab_experiment_ids 字段（TEXT[]），支持 智能体挂载多逻辑"
}

// Up 执行迁移
func (m *AIAgentExtensionMigration) Up(ctx context.Context) error {
	// 幂等检查：表不存在则跳过（依赖 v2.2.0 创建 ai_agents 表）
	if !m.db.Migrator().HasTable("ai_agents") {
		return nil
	}

	// 检查并新增 decision_strategy_ids 字段
	if !m.db.Migrator().HasColumn(&struct {
		DecisionStrategyIDs string `gorm:"type:text[]"`
	}{}, "DecisionStrategyIDs") {
		if err := m.db.Exec("ALTER TABLE ai_agents ADD COLUMN decision_strategy_ids TEXT[] DEFAULT '{}'").Error; err != nil {
			return err
		}
	}

	// 检查并新增 ab_experiment_ids 字段
	if !m.db.Migrator().HasColumn(&struct {
		ABExperimentIDs string `gorm:"type:text[]"`
	}{}, "ABExperimentIDs") {
		if err := m.db.Exec("ALTER TABLE ai_agents ADD COLUMN ab_experiment_ids TEXT[] DEFAULT '{}'").Error; err != nil {
			return err
		}
	}

	return nil
}

// Down 回滚
func (m *AIAgentExtensionMigration) Down(ctx context.Context) error {
	if !m.db.Migrator().HasTable("ai_agents") {
		return nil
	}
	_ = m.db.Exec("ALTER TABLE ai_agents DROP COLUMN IF EXISTS ab_experiment_ids").Error
	_ = m.db.Exec("ALTER TABLE ai_agents DROP COLUMN IF EXISTS decision_strategy_ids").Error
	return nil
}

// 编译期接口断言
var _ migration.Migration = (*AIAgentExtensionMigration)(nil)
