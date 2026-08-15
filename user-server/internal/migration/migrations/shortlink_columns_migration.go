package migrations


import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

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

