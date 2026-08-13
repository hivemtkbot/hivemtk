package migrations

// knowledge_search_log_product_optional_migration.go knowledge_search_logs.product_id 改为可空 (v2.13.0)
//
// 五层架构归属: L5 数据层
// 问题: rag.search 在全量检索（product_id == ""）时把 product_id 置为 NULL 写入
//       knowledge_search_logs，但历史 initial_schema 经 GORM AutoMigrate 后该列被建成
//       `varchar NOT NULL default ''`，导致：
//         `null value in column "product_id" of relation "knowledge_search_logs"
//          violates not-null constraint` (SQLSTATE 23502)
//       该错误每次 RAG 检索都会打印（best-effort 不阻断主流程，但污染日志且掩盖真实异常）。
//
// 本迁移要点（幂等，可重入）：
//   ALTER TABLE knowledge_search_logs ALTER COLUMN product_id DROP NOT NULL
//
// 幂等性：列不存在则跳过；已可空则为 no-op。

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// KnowledgeSearchLogProductOptionalMigration knowledge_search_logs.product_id 改为可空
type KnowledgeSearchLogProductOptionalMigration struct {
	db *gorm.DB
}

// NewKnowledgeSearchLogProductOptionalMigration 创建迁移实例
func NewKnowledgeSearchLogProductOptionalMigration(db *gorm.DB) *KnowledgeSearchLogProductOptionalMigration {
	return &KnowledgeSearchLogProductOptionalMigration{db: db}
}

// Version 返回版本号
func (m *KnowledgeSearchLogProductOptionalMigration) Version() string { return "v2.13.0" }

// Name 返回迁移名称
func (m *KnowledgeSearchLogProductOptionalMigration) Name() string {
	return "knowledge_search_logs.product_id 改为可空"
}

// Description 返回迁移描述
func (m *KnowledgeSearchLogProductOptionalMigration) Description() string {
	return "rag.search 全量检索时 product_id 为空写入 NULL，需放开 NOT NULL 约束"
}

// Up 执行升级
func (m *KnowledgeSearchLogProductOptionalMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.Migrator().HasTable("knowledge_search_logs") {
		return nil
	}
	if !m.db.Migrator().HasColumn("knowledge_search_logs", "product_id") {
		return nil
	}

	stmts := []string{
		`ALTER TABLE knowledge_search_logs ALTER COLUMN product_id DROP NOT NULL`,
	}
	if err := execAllCS(ctx, m.db, stmts); err != nil {
		return fmt.Errorf("放开 knowledge_search_logs.product_id NOT NULL 失败: %w", err)
	}
	return nil
}

// Down 执行降级（重新加回 NOT NULL 前需先清理 NULL 值，避免失败；此处保守不执行）
func (m *KnowledgeSearchLogProductOptionalMigration) Down(ctx context.Context) error {
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*KnowledgeSearchLogProductOptionalMigration)(nil)
