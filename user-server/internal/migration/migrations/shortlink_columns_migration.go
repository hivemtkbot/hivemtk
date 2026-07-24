package migrations

// shortlink_columns_migration.go short_links 表字段补齐迁移 (v2.11.1)
//
// 五层架构归属: L5 数据层
// 问题: model.ShortLink 已演进，新增 Title / Description / DomainID / Password 字段，
//       且 ExpireTime 经 GORM 默认列名映射为 expire_time；但历史 initial_schema 创建的
//       short_links 表仅有原始列（expire_at 等），缺少上述列，导致
//       INSERT 报 `column "title" of relation "short_links" does not exist`，
//       短链创建（API 与 UI「添加短链」）直接 500 失败。
//
// 本迁移要点（全部幂等，可重入）：
//   1. 若历史列 expire_at 存在则重命名为 expire_time（与模型 GORM 默认列名对齐）
//   2. 补齐缺失列：expire_time / title / description / domain_id / password / updated_at
//
// 幂等性：RENAME 包在 IF EXISTS 内；ADD COLUMN 均使用 IF NOT EXISTS

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// ShortLinkColumnsMigration short_links 字段补齐迁移
type ShortLinkColumnsMigration struct {
	db *gorm.DB
}

// NewShortLinkColumnsMigration 创建迁移实例
func NewShortLinkColumnsMigration(db *gorm.DB) *ShortLinkColumnsMigration {
	return &ShortLinkColumnsMigration{db: db}
}

// Version 返回版本号
func (m *ShortLinkColumnsMigration) Version() string { return "v2.11.1" }

// Name 返回迁移名称
func (m *ShortLinkColumnsMigration) Name() string {
	return "short_links 字段补齐 (title/description/domain_id/password/expire_time)"
}

// Description 返回迁移描述
func (m *ShortLinkColumnsMigration) Description() string {
	return "补齐 short_links 缺失列，修复短链创建 500（column title does not exist）"
}

// Up 执行升级
func (m *ShortLinkColumnsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if !m.db.Migrator().HasTable("short_links") {
		return nil
	}

	stmts := []string{
		// 历史列 expire_at 重命名为模型期望的 expire_time（仅当存在时）
		`DO $$ BEGIN
      IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name='short_links' AND column_name='expire_at') THEN
        ALTER TABLE short_links RENAME COLUMN expire_at TO expire_time;
      END IF;
    END $$;`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS expire_time TIMESTAMP`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS title VARCHAR(255)`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS description TEXT`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS domain_id BIGINT`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS password VARCHAR(255)`,
		`ALTER TABLE short_links ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP`,
	}
	if err := execAllShortLink(ctx, m.db, stmts); err != nil {
		return fmt.Errorf("补齐 short_links 字段失败: %w", err)
	}
	return nil
}

// Down 执行降级（不删除列，避免误删数据；如需回滚请手动处理）
func (m *ShortLinkColumnsMigration) Down(ctx context.Context) error {
	return nil
}

func execAllShortLink(ctx context.Context, db *gorm.DB, stmts []string) error {
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
var _ migration.Migration = (*ShortLinkColumnsMigration)(nil)
