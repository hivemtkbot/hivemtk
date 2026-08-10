package migrations

// multilingual_i18n_migration.go v1.2 出海多语言方案 - Model 层 + Migration 层
//
// 背景：
//   v3.11.0 之前所有表结构默认中文单语，未考虑出海场景。本次新增多语言支撑：
//   1. 新建 glossaries 表：保护 SKU/价格/品牌名不被 LLM 翻译（term_id 业务唯一键）
//   2. ai_agents 新增 internal_language / target_language：商户内部语言 + 对外目标语言
//   3. chat_channels 新增 target_language：渠道级目标语言（覆盖智能体配置）
//   4. llm_routing_logs 新增 7 个多语言字段：internal_lang / target_lang / cross_lingual /
//      glossary_version / cache_hit / quality_score / validation_issues
//   5. knowledge_chunks 新增 source_language：知识库源语言
//   6. asset_bundles 新增 examples / supported_languages：多语言 few-shot 示例 + 支持语言声明
//
// 设计要点：
//   - glossaries 用 GORM AutoMigrate 建表（与 asset_bundles 一致）
//   - 其余表用 ALTER TABLE ADD COLUMN IF NOT EXISTS（PG 9.6+ 幂等可重入）
//   - 私域独立部署：无 merchant_id
//   - 不修改 service/controller/aiagent 层（仅 Model + Migration）

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

	// 1. 新建 glossaries 表（用 GORM AutoMigrate，处理 JSONB + 软删除 + 唯一索引）
	if err := m.db.WithContext(ctx).AutoMigrate(&model.Glossary{}); err != nil {
		return fmt.Errorf("migrate glossaries failed: %w", err)
	}

	// 2. 其余表 ALTER TABLE ADD COLUMN IF NOT EXISTS（幂等可重入）
	stmts := []string{
		// 2.1 ai_agents 多语言配置
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS internal_language VARCHAR(8) NOT NULL DEFAULT 'zh'`,
		`ALTER TABLE ai_agents ADD COLUMN IF NOT EXISTS target_language   VARCHAR(8) NOT NULL DEFAULT ''`,

		// 2.2 chat_channels 渠道目标语言
		`ALTER TABLE chat_channels ADD COLUMN IF NOT EXISTS target_language VARCHAR(8) NOT NULL DEFAULT ''`,

		// 2.3 llm_routing_logs 多语言扩展字段
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS internal_lang     VARCHAR(8)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS target_lang       VARCHAR(8)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS cross_lingual     BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS glossary_version  VARCHAR(32)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS cache_hit         BOOLEAN NOT NULL DEFAULT FALSE`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS quality_score     NUMERIC(4,3)`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS validation_issues JSONB`,
		// 跨语言生成 + 时间复合索引：监控出海流量占比
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_cross_lingual ON llm_routing_logs (cross_lingual, created_at DESC)`,
		// 目标语言 + 时间复合索引：按语种统计请求量
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_target_lang ON llm_routing_logs (target_lang, created_at DESC)`,

		// 2.4 knowledge_chunks 知识库源语言
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS source_language VARCHAR(8) NOT NULL DEFAULT 'zh'`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_source_language ON knowledge_chunks (source_language)`,

		// 2.5 asset_bundles 多语言 few-shot 示例 + 支持语言声明
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
		// 2.5 asset_bundles
		`ALTER TABLE asset_bundles DROP COLUMN IF EXISTS supported_languages`,
		`ALTER TABLE asset_bundles DROP COLUMN IF EXISTS examples`,

		// 2.4 knowledge_chunks
		`DROP INDEX IF EXISTS idx_knowledge_chunks_source_language`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS source_language`,

		// 2.3 llm_routing_logs
		`DROP INDEX IF EXISTS idx_llm_routing_logs_target_lang`,
		`DROP INDEX IF EXISTS idx_llm_routing_logs_cross_lingual`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS validation_issues`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS quality_score`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS cache_hit`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS glossary_version`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS cross_lingual`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS target_lang`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS internal_lang`,

		// 2.2 chat_channels
		`ALTER TABLE chat_channels DROP COLUMN IF EXISTS target_language`,

		// 2.1 ai_agents
		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS target_language`,
		`ALTER TABLE ai_agents DROP COLUMN IF EXISTS internal_language`,

		// 1. glossaries
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
