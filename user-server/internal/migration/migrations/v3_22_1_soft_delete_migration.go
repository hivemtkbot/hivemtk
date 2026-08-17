package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// SoftDeleteMigration OPT-DB-07: 软删除扩到核心业务表
//
// 现状：核心业务表（ai_agents、knowledge_bases、sop_agents 等）缺少 deleted_at 字段，
// 无法实现软删除，数据一旦被删除无法恢复。
//
// 本迁移为以下核心业务表添加 deleted_at timestamptz 列：
//   - ai_agents
//   - knowledge_bases
//   - sop_agents
//   - sop_templates
//   - message_hub
//   - intent_records
//   - channel_agent_bindings
//   - customer_service_agents
//   - inbox_conversations
//   - feedback_events
//   - sop_executions
//   - sop_node_transitions
//   - optimization_suggestions
//   - faq_entries
//   - platform_account_configs
//   - community_groups
//   - customer_sessions
//   - live_codes
//   - sla_policies
//   - sla_violations
//   - agent_statuses
//   - feedback_records
//   - layer_decision_logs
//   - memory_items
//   - business_memories
//   - sop_state_memories
//
// 幂等安全：先检查列是否存在，不存在才 ADD。
type SoftDeleteMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*SoftDeleteMigration)(nil)

func NewSoftDeleteMigration(db *gorm.DB) *SoftDeleteMigration {
	return &SoftDeleteMigration{db: db}
}

func (m *SoftDeleteMigration) Version() string { return "v3.22.1" }

func (m *SoftDeleteMigration) Name() string { return "核心业务表软删除扩展" }

func (m *SoftDeleteMigration) Description() string {
	return "为 ai_agents / knowledge_bases / sop_agents 等核心业务表添加 deleted_at 列，支持软删除"
}

// coreTables 需要添加 deleted_at 的核心业务表列表
var coreTables = []string{
	"ai_agents",
	"knowledge_bases",
	"sop_agents",
	"sop_templates",
	"message_hub",
	"intent_records",
	"channel_agent_bindings",
	"customer_service_agents",
	"inbox_conversations",
	"feedback_events",
	"sop_executions",
	"sop_node_transitions",
	"optimization_suggestions",
	"faq_entries",
	"platform_account_configs",
	"community_groups",
	"customer_sessions",
	"live_codes",
	"sla_policies",
	"sla_violations",
	"agent_statuses",
	"feedback_records",
	"layer_decision_logs",
	"memory_items",
	"business_memories",
	"sop_state_memories",
}

func (m *SoftDeleteMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	for _, table := range coreTables {
		// 检查表是否存在
		var exists bool
		err := m.db.WithContext(ctx).Raw(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`,
			table,
		).Scan(&exists).Error
		if err != nil {
			return fmt.Errorf("检查表 %s 是否存在失败: %w", table, err)
		}
		if !exists {
			continue
		}

		// 检查列是否已存在
		var colExists bool
		err = m.db.WithContext(ctx).Raw(
			`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ? AND column_name = 'deleted_at')`,
			table,
		).Scan(&colExists).Error
		if err != nil {
			return fmt.Errorf("检查表 %s.deleted_at 失败: %w", table, err)
		}
		if colExists {
			continue
		}

		// 添加 deleted_at 列
		sql := fmt.Sprintf(`ALTER TABLE %q ADD COLUMN deleted_at timestamptz`,
			table)
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("添加 %s.deleted_at 失败: %w", table, err)
		}

		// 添加索引
		idxSQL := fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_deleted_at ON %q (deleted_at)`,
			table, table)
		_ = m.db.WithContext(ctx).Exec(idxSQL).Error
	}

	return nil
}

func (m *SoftDeleteMigration) Down(ctx context.Context) error {
	return nil
}