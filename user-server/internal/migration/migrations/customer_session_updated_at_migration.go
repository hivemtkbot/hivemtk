package migrations

// customer_session_updated_at_migration.go customer_sessions 补齐 updated_at 列 (v2.12.0)
//
// 五层架构归属: L5 数据层
// 问题: model.CustomerSession 演进后应有 UpdatedAt 字段（与全库其他模型一致，
//       GORM 的 Save/Update 会自动维护 updated_at），但历史 initial_schema 创建的
//       customer_sessions 表缺少该列，导致：
//         1) repository.UpsertByOneID 的 raw INSERT 显式写入 updated_at →
//            `column "updated_at" of relation "customer_sessions" does not exist` (SQLSTATE 42703)
//         2) SmartCSOrchestrator 降级路径 r.db.Save(session) 也引用 updated_at → 同样报错，
//            被迫降级用 stale session_id（会话实体不稳定）。
//       会话 upsert 在所有渠道（TG/网页/企微...）入站时都会触发，故影响全渠道。
//
// 本迁移要点（幂等，可重入）：
//   ALTER TABLE customer_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP
//
// 幂等性：ADD COLUMN 使用 IF NOT EXISTS；表不存在则跳过。

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// CustomerSessionUpdatedAtMigration customer_sessions 补齐 updated_at 列迁移
type CustomerSessionUpdatedAtMigration struct {
	db *gorm.DB
}

// NewCustomerSessionUpdatedAtMigration 创建迁移实例
func NewCustomerSessionUpdatedAtMigration(db *gorm.DB) *CustomerSessionUpdatedAtMigration {
	return &CustomerSessionUpdatedAtMigration{db: db}
}

// Version 返回版本号
func (m *CustomerSessionUpdatedAtMigration) Version() string { return "v2.12.0" }

// Name 返回迁移名称
func (m *CustomerSessionUpdatedAtMigration) Name() string {
	return "customer_sessions 补齐 updated_at 列"
}

// Description 返回迁移描述
func (m *CustomerSessionUpdatedAtMigration) Description() string {
	return "补齐 customer_sessions 缺失的 updated_at 列，修复全渠道会话 upsert 的 SQLSTATE 42703"
}

// Up 执行升级
func (m *CustomerSessionUpdatedAtMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.Migrator().HasTable("customer_sessions") {
		return nil
	}

	stmts := []string{
		`ALTER TABLE customer_sessions ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`,
	}
	if err := execAllCS(ctx, m.db, stmts); err != nil {
		return fmt.Errorf("补齐 customer_sessions.updated_at 失败: %w", err)
	}
	return nil
}

// Down 执行降级（不删除列，避免误删数据；如需回滚请手动处理）
func (m *CustomerSessionUpdatedAtMigration) Down(ctx context.Context) error {
	return nil
}

func execAllCS(ctx context.Context, db *gorm.DB, stmts []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*CustomerSessionUpdatedAtMigration)(nil)
