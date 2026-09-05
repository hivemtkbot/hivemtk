package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// EnumOptimizeMigration OPT-DB-08: 高频 enum 改 PostgreSQL ENUM
//
// 现状：AgentType、AgentMode、ChannelType、KnowledgeBaseType 等高频枚举字段
// 使用 varchar(16/32) 存储，缺乏数据库层的值校验与类型安全。
//
// 本迁移为以下高频枚举创建 PostgreSQL 原生 ENUM 类型，并迁移对应列：
//   - agent_type:        sales / customer_service / hybrid
//   - agent_mode:        passive / active
//   - channel_type:      telegram / wecom / feishu / whatsapp / dingtalk / douyin /
//     xiaohongshu / kuaishou / xianyu / tiktok / web / web_embed
//   - kb_type:           faq / rag / sop
//   - kb_owner_type:     private / shared
//   - asset_bundle_scope: private / shared / official
//   - asset_bundle_status: draft / active / inactive / archived
//
// 幂等安全：CREATE TYPE IF NOT EXISTS 通过 DO $$ 块模拟。
type EnumOptimizeMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*EnumOptimizeMigration)(nil)

func NewEnumOptimizeMigration(db *gorm.DB) *EnumOptimizeMigration {
	return &EnumOptimizeMigration{db: db}
}

func (m *EnumOptimizeMigration) Version() string { return "v3.22.2" }

func (m *EnumOptimizeMigration) Name() string { return "高频 enum 改 PostgreSQL ENUM" }

func (m *EnumOptimizeMigration) Description() string {
	return "为 AgentType / AgentMode / ChannelType / KBType 等高频枚举创建 PostgreSQL 原生 ENUM"
}

func (m *EnumOptimizeMigration) createEnumIfNotExists(ctx context.Context, name string, values []string) error {

	var exists bool
	err := m.db.WithContext(ctx).Raw(
		`SELECT EXISTS (SELECT 1 FROM pg_type WHERE typname = ?)`,
		name,
	).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("检查 ENUM %s 是否存在失败: %w", name, err)
	}
	if exists {
		return nil
	}

	sql := fmt.Sprintf(`CREATE TYPE %q AS ENUM (`, name)
	for i, v := range values {
		if i > 0 {
			sql += ", "
		}
		sql += fmt.Sprintf(`'%s'`, v)
	}
	sql += `)`
	return m.db.WithContext(ctx).Exec(sql).Error
}

func (m *EnumOptimizeMigration) alterColumnToEnum(ctx context.Context, table, column, enumType, defaultVal string) error {
	stmts := []string{
		fmt.Sprintf(`ALTER TABLE %q ALTER COLUMN %q DROP DEFAULT`, table, column),
		fmt.Sprintf(`ALTER TABLE %q ALTER COLUMN %q TYPE %q USING %q::text::%q`, table, column, enumType, column, enumType),
	}
	if defaultVal != "" {
		stmts = append(stmts, fmt.Sprintf(`ALTER TABLE %q ALTER COLUMN %q SET DEFAULT '%s'::%q`, table, column, defaultVal, enumType))
	}
	for _, sql := range stmts {
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

func (m *EnumOptimizeMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	enums := map[string][]string{
		"agent_type_enum": {"sales", "customer_service", "hybrid"},
		"agent_mode_enum": {"passive", "active"},
		"channel_type_enum": {"telegram", "wecom", "feishu", "whatsapp", "dingtalk",
			"douyin", "xiaohongshu", "kuaishou", "xianyu", "tiktok", "web", "web_embed"},
		"kb_type_enum":             {"faq", "rag", "sop"},
		"kb_owner_type_enum":       {"private", "shared"},
		"asset_bundle_scope_enum":  {"private", "shared", "official"},
		"asset_bundle_status_enum": {"draft", "active", "inactive", "archived"},
	}

	for name, values := range enums {
		if err := m.createEnumIfNotExists(ctx, name, values); err != nil {
			return fmt.Errorf("创建 ENUM %s 失败: %w", name, err)
		}
	}

	type tableCol struct {
		table      string
		column     string
		enumType   string
		defaultVal string
	}

	conversions := []tableCol{

		{"ai_agents", "agent_type", "agent_type_enum", "sales"},
		{"ai_agents", "agent_mode", "agent_mode_enum", "passive"},

		{"channel_agent_bindings", "channel_type", "channel_type_enum", ""},

		{"knowledge_bases", "type", "kb_type_enum", ""},
		{"knowledge_bases", "owner_type", "kb_owner_type_enum", ""},

		{"asset_bundles", "scope", "asset_bundle_scope_enum", ""},
		{"asset_bundles", "status", "asset_bundle_status_enum", "draft"},

		{"asset_bundle_items", "status", "asset_bundle_status_enum", "draft"},
	}

	for _, c := range conversions {

		var tableExists bool
		err := m.db.WithContext(ctx).Raw(
			`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = ?)`,
			c.table,
		).Scan(&tableExists).Error
		if err != nil {
			return fmt.Errorf("检查表 %s 是否存在失败: %w", c.table, err)
		}
		if !tableExists {
			continue
		}

		var colExists bool
		err = m.db.WithContext(ctx).Raw(
			`SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ? AND column_name = ?)`,
			c.table, c.column,
		).Scan(&colExists).Error
		if err != nil {
			return fmt.Errorf("检查列 %s.%s 失败: %w", c.table, c.column, err)
		}
		if !colExists {
			continue
		}

		var dataType string
		err = m.db.WithContext(ctx).Raw(
			`SELECT data_type FROM information_schema.columns WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`,
			c.table, c.column,
		).Scan(&dataType).Error
		if err != nil {
			return fmt.Errorf("查询列 %s.%s 类型失败: %w", c.table, c.column, err)
		}
		if dataType == "USER-DEFINED" {

			continue
		}

		if err := m.alterColumnToEnum(ctx, c.table, c.column, c.enumType, c.defaultVal); err != nil {
			return fmt.Errorf("转换 %s.%s -> %s 失败: %w", c.table, c.column, c.enumType, err)
		}
	}

	return nil
}

func (m *EnumOptimizeMigration) Down(ctx context.Context) error {
	return nil
}
