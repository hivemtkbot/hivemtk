package migrations


import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// MultilingualI18nMigration 多语言 i18n 方案迁移（v3.12.0）
type MultilingualI18nMigration struct {
	db *gorm.DB
}

// NewMultilingualI18nMigration 创建迁移实例
func NewMultilingualI18nMigration(db *gorm.DB) *MultilingualI18nMigration {
	return &MultilingualI18nMigration{db: db}
}

// Version 返回版本号
func (m *MultilingualI18nMigration) Version() string { return "v3.12.0" }

// Name 返回迁移名称
func (m *MultilingualI18nMigration) Name() string {
	return "多语言 i18n 方案 - glossaries 表 + 5 张表扩展字段"
}

// Description 返回迁移描述
func (m *MultilingualI18nMigration) Description() string {
	return "v1.2 出海多语言方案：新建 glossaries 术语表；扩展 ai_agents/chat_channels/llm_routing_logs/knowledge_chunks/asset_bundles 多语言字段"
}

// Up 执行升级
func (m *MultilingualI18nMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.db.WithContext(ctx).AutoMigrate(&model.Glossary{}); err != nil {
		return fmt.Errorf("migrate glossaries failed: %w", err)
	}

	stmts := []string{
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS internal_language VARCHAR(8) NOT NULL DEFAULT 'zh'`,
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS target_language   VARCHAR(8) NOT NULL DEFAULT ''`,

		`ALTER TABLE chat_channels ADD COLUMN IF NOT EXISTS target_language VARCHAR(8) NOT NULL DEFAULT ''`,

		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS internal_lang     VARCHAR(8)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS target_lang       VARCHAR(8)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS cross_lingual     BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS glossary_version  VARCHAR(32)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS cache_hit         BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS quality_score     NUMERIC(4,3)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS validation_issues JSONB`,
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_cross_lingual ON llm_routing_logs (cross_lingual, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_target_lang ON llm_routing_logs (target_lang, created_at DESC)`,

		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS source_language VARCHAR(8) NOT NULL DEFAULT 'zh'`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_source_language ON knowledge_chunks (source_language)`,

		`ALTER TABLE asset_bundles ADD COLUMN IF NOT EXISTS examples            JSONB`,
		`ALTER TABLE asset_bundles ADD COLUMN IF NOT EXISTS supported_languages TEXT[]`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("multilingual_i18n 执行失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// Down 回滚
func (m *MultilingualI18nMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`ALTER TABLE asset_bundles DROP COLUMN IF EXISTS supported_languages`,
		`ALTER TABLE asset_bundles DROP COLUMN IF EXISTS examples`,

		`DROP INDEX IF EXISTS idx_knowledge_chunks_source_language`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS source_language`,

		`DROP INDEX IF EXISTS idx_llm_routing_logs_target_lang`,
		`DROP INDEX IF EXISTS idx_llm_routing_logs_cross_lingual`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS validation_issues`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS quality_score`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS cache_hit`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS glossary_version`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS cross_lingual`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS target_lang`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS internal_lang`,

		`ALTER TABLE chat_channels DROP COLUMN IF EXISTS target_language`,

		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS target_language`,
		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS internal_language`,

		`DROP TABLE IF EXISTS glossaries`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("multilingual_i18n 回滚失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// truncate 截断字符串用于错误信息
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// 编译期接口断言
var _ migration.Migration = (*MultilingualI18nMigration)(nil)

