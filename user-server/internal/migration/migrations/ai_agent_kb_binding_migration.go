package migrations

// ai_agent_kb_binding_migration.go 2026-07-31 AI 智能体知识库绑定
//
// 背景：
// 1. 现状：faq_entries / sop_templates 是全局共享表，未与 ai_agents 建立绑定关系。
// 2. 同一 FAQ（"包邮吗"）淘宝客服 / WhatsApp 英文客服需要不同答案，但共享同一条。
// 3. 设计：把 FAQ / SOP 模板视为「知识库资产」，在 ai_agents 上加 faq_entry_ids / sop_template_ids 数组字段。
//    - 与现有 rag_product_ids / sop_ids / script_library_ids / decision_strategy_ids / ab_experiment_ids 统一模式
//    - 一个智能体可绑多个 FAQ / SOP 模板，一个 FAQ / SOP 模板可被多个智能体共享（多对多关系）
//    - 数组为空 = 全局共享（向后兼容旧数据）
// 4. Layer1 匹配时按 agent_id 过滤：未绑定 = 全局命中；绑定了 = 只在绑定的 IDs 内匹配。

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// AIAgentKBBindingMigration AI 智能体知识库绑定迁移
type AIAgentKBBindingMigration struct {
	db *gorm.DB
}

// NewAIAgentKBBindingMigration 创建迁移实例
func NewAIAgentKBBindingMigration(db *gorm.DB) *AIAgentKBBindingMigration {
	return &AIAgentKBBindingMigration{db: db}
}

// Version 返回版本号
func (m *AIAgentKBBindingMigration) Version() string { return "v3.14.0" }

// Name 返回迁移名称
func (m *AIAgentKBBindingMigration) Name() string {
	return "AI 智能体知识库绑定 - faq_entry_ids / sop_template_ids"
}

// Description 返回迁移描述
func (m *AIAgentKBBindingMigration) Description() string {
	return "2026-07-31：在 ai_agents 表新增 faq_entry_ids / sop_template_ids 两个 text[] 字段，绑定关系与现有 rag_product_ids / sop_ids / script_library_ids 一致；Layer1 匹配按 agent_id 过滤实现多渠道/多场景知识库隔离"
}

// Up 执行迁移
func (m *AIAgentKBBindingMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		// ai_agents 表新增 2 个绑定字段
		// 数组为空表示"全局共享"（向后兼容旧 agent），非空表示"专属绑定"
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS faq_entry_ids TEXT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS sop_template_ids TEXT[] NOT NULL DEFAULT '{}'`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("ai_agent_kb_binding 迁移失败: %w (SQL: %s)", err, s)
		}
	}
	return nil
}

// Down 回滚
func (m *AIAgentKBBindingMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS sop_template_ids`,
		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS faq_entry_ids`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("ai_agent_kb_binding 回滚失败: %w (SQL: %s)", err, s)
		}
	}
	return nil
}

// Ensure AIAgentKBBindingMigration implements Migration interface
var _ migration.Migration = (*AIAgentKBBindingMigration)(nil)
