package migrations

// multilingual_i18n_p13_migration.go v1.2 出海多语言方案 P1-3 - 知识库预翻译字段
//
// 背景：
//   v3.12.0 多语言 i18n 方案已为 knowledge_chunks 增加 source_language 字段。
//   P1-3 监控看板 + 知识库预翻译支持需要新增 translated_versions JSONB 字段，
//   用于存储高频条目的预翻译版本（按 lang 索引），加速低资源语言召回。
//
// 设计要点：
//   - ADD COLUMN IF NOT EXISTS（PG 9.6+ 幂等可重入）
//   - 私域独立部署：无 merchant_id
//   - 不影响现有数据：新字段默认 NULL，未预翻译时 GetTranslated 返回原文

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// MultilingualI18nP13Migration 多语言 i18n P1-3 迁移（v3.12.1）
type MultilingualI18nP13Migration struct {
	db *gorm.DB
}

// NewMultilingualI18nP13Migration 创建迁移实例
func NewMultilingualI18nP13Migration(db *gorm.DB) *MultilingualI18nP13Migration {
	return &MultilingualI18nP13Migration{db: db}
}

// Version 返回版本号
func (m *MultilingualI18nP13Migration) Version() string { return "v3.12.1" }

// Name 返回迁移名称
func (m *MultilingualI18nP13Migration) Name() string {
	return "多语言 i18n P1-3 - knowledge_chunks.translated_versions 字段"
}

// Description 返回迁移描述
func (m *MultilingualI18nP13Migration) Description() string {
	return "v1.2 出海多语言方案 P1-3：为 knowledge_chunks 新增 translated_versions JSONB 字段，存储高频条目预翻译版本"
}

// Up 执行升级
func (m *MultilingualI18nP13Migration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	// ADD COLUMN IF NOT EXISTS 幂等可重入
	stmts := []string{
		// knowledge_chunks 预翻译版本字段（按 lang 存储翻译后的 content）
		// 格式：{"en": "translated content", "ja": "..."}
		`ALTER TABLE knowledge_chunks ADD COLUMN IF NOT EXISTS translated_versions JSONB`,
		// GIN 索引：加速按 lang 查询翻译版本是否存在
		`CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_translated_versions ON knowledge_chunks USING GIN (translated_versions)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("multilingual_i18n_p13 执行失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// Down 回滚
func (m *MultilingualI18nP13Migration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`DROP INDEX IF EXISTS idx_knowledge_chunks_translated_versions`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS translated_versions`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("multilingual_i18n_p13 回滚失败 (%s): %w", truncate(s, 60), err)
		}
	}
	return nil
}

// 编译期接口断言
var _ migration.Migration = (*MultilingualI18nP13Migration)(nil)
