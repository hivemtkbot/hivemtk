package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"hivemtk-user/internal/migration"
)

// DropCdpAutoReplyMigration 清理 CDP/无头浏览器自动回复功能删除后残留的孤儿 schema：
//   - account 表（Account 模型表名为 account，单数）的 5 个无头模式开关列：
//     douyin_headless/kuaishou_headless/xiaohongshu_headless/xianyu_headless/tiktok_headless，
//     随 AutoReplyAccount/无头浏览器自动回复功能一并删除，已无任何 Go 代码读写。
//   - system_config 表的 auto_reply_headless 全局开关列（对应已删除的 SystemConfig.AutoReplyHeadless 字段）。
//   - auto_reply_accounts / auto_reply_rules / auto_reply_logs 三张孤儿表：
//     无任何 GORM 模型绑定，initial_schema 也不再创建，仅被旧迁移与失效测试引用。
//
// 全部使用 IF EXISTS 并对目标表存在性做防御，幂等可重入；已部署库与全新库均安全。
type DropCdpAutoReplyMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*DropCdpAutoReplyMigration)(nil)

// NewDropCdpAutoReplyMigration 构造函数
func NewDropCdpAutoReplyMigration(db *gorm.DB) *DropCdpAutoReplyMigration {
	return &DropCdpAutoReplyMigration{db: db}
}

func (m *DropCdpAutoReplyMigration) Version() string { return "v3.19.0" }

func (m *DropCdpAutoReplyMigration) Name() string {
	return "清理 CDP 无头浏览器自动回复残留孤儿 schema"
}

func (m *DropCdpAutoReplyMigration) Description() string {
	return "删除 account 表 5 个无头模式开关列 + system_config.auto_reply_headless 列 + auto_reply_accounts/auto_reply_rules/auto_reply_logs 三张孤儿表"
}

func (m *DropCdpAutoReplyMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if tableExists(ctx, m.db, "account") {
		headlessCols := []string{
			"douyin_headless", "kuaishou_headless", "xiaohongshu_headless",
			"xianyu_headless", "tiktok_headless",
		}
		for _, col := range headlessCols {
			stmt := fmt.Sprintf("ALTER TABLE account DROP COLUMN IF EXISTS %s", col)
			if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
				return fmt.Errorf("DropCdpAutoReplyMigration failed on [%s]: %w", stmt, err)
			}
		}
	}

	if tableExists(ctx, m.db, "system_config") {
		stmt := "ALTER TABLE system_config DROP COLUMN IF EXISTS auto_reply_headless"
		if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("DropCdpAutoReplyMigration failed on [%s]: %w", stmt, err)
		}
	}

	orphanTables := []string{"auto_reply_accounts", "auto_reply_rules", "auto_reply_logs"}
	for _, tbl := range orphanTables {
		stmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", tbl)
		if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
			return fmt.Errorf("DropCdpAutoReplyMigration failed on [%s]: %w", stmt, err)
		}
	}
	return nil
}

func (m *DropCdpAutoReplyMigration) Down(ctx context.Context) error {
	return nil
}

func tableExists(ctx context.Context, db *gorm.DB, name string) bool {
	var n int64
	if err := db.WithContext(ctx).
		Raw("SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name = ?", name).
		Scan(&n).Error; err != nil {
		return false
	}
	return n > 0
}
