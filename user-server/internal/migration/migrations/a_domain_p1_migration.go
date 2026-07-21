package migrations

// a_domain_p1_migration.go A 域 P1 缺口修复迁移 (v2.11.0)
//
// 五层架构归属: L5 数据层
// 设计依据: docs/standards/MASTER_RULES.md「私域独立部署，无 merchant_id 字段」
//          A 域 P1 缺口修复 (2026-07-21)
//          - P1-1 MFA 多因素认证（已有 auth_security_migration，本版本不再重复）
//          - P1-2 异常登录预警（已有 auth_security_migration，本版本不再重复）
//          - P1-3 密码策略（已有 auth_security_migration，本版本不再重复）
//          - P1-4 数据行级权限（team_user 新增 data_scope / department_id / team_id / custom_dept_ids）
//
// 本迁移要点：
//   1. team_users 表新增 4 个字段：
//      - data_scope SMALLINT DEFAULT 3  （1=全部 2=本部门 3=本人 4=自定义）
//      - department_id BIGINT DEFAULT 0
//      - team_id BIGINT DEFAULT 0
//      - custom_dept_ids TEXT
//   2. 字段已通过 GORM 模型同步（TeamUser struct）；本迁移是幂等兜底
//   3. 为部门/团队字段添加索引（加速按部门/团队过滤）
//   4. 老数据回填：admin 角色 → data_scope=1，其他 → 3（self）
//
// 幂等性：使用 ADD COLUMN IF NOT EXISTS，可重入

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// ADomainP1Migration A 域 P1 缺口修复迁移
type ADomainP1Migration struct {
	db *gorm.DB
}

// NewADomainP1Migration 创建迁移实例
func NewADomainP1Migration(db *gorm.DB) *ADomainP1Migration {
	return &ADomainP1Migration{db: db}
}

// Version 返回版本号
func (m *ADomainP1Migration) Version() string { return "v2.11.0" }

// Name 返回迁移名称
func (m *ADomainP1Migration) Name() string { return "A 域 P1 缺口修复 - team_user 数据范围" }

// Description 返回迁移描述
func (m *ADomainP1Migration) Description() string {
	return "team_users 表新增 data_scope / department_id / team_id / custom_dept_ids 4 字段，支持 P1-4 行级权限"
}

// Up 执行升级
func (m *ADomainP1Migration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1. 确认 team_users 表存在（依赖 v1.2.0 TeamUserSchemaMigration）
	if !m.db.Migrator().HasTable("team_users") {
		// 表不存在则直接返回（不强制创建，让 initial_schema 负责）
		return nil
	}

	// 2. 新增列（PG 9.6+ 支持 IF NOT EXISTS）
	stmts := []string{
		`ALTER TABLE team_users ADD COLUMN IF NOT EXISTS data_scope SMALLINT NOT NULL DEFAULT 3`,
		`ALTER TABLE team_users ADD COLUMN IF NOT EXISTS department_id BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE team_users ADD COLUMN IF NOT EXISTS team_id BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE team_users ADD COLUMN IF NOT EXISTS custom_dept_ids TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_team_users_data_scope ON team_users(data_scope)`,
		`CREATE INDEX IF NOT EXISTS idx_team_users_department_id ON team_users(department_id)`,
		`CREATE INDEX IF NOT EXISTS idx_team_users_team_id ON team_users(team_id)`,
	}
	if err := execAllA(ctx, m.db, stmts); err != nil {
		return fmt.Errorf("添加 team_users 行级权限字段失败: %w", err)
	}

	// 3. 老数据回填：admin 角色 → data_scope=1（全部）
	if err := m.db.WithContext(ctx).Exec(`
		UPDATE team_users
		SET data_scope = 1
		WHERE role = 'admin' AND data_scope = 3
	`).Error; err != nil {
		return fmt.Errorf("admin 角色 data_scope 回填失败: %w", err)
	}

	return nil
}

// Down 执行降级（不删除列，避免误删数据；如需回滚请手动处理）
func (m *ADomainP1Migration) Down(ctx context.Context) error {
	return nil
}

// execAllA 批量执行 SQL（出错即返回）
func execAllA(ctx context.Context, db *gorm.DB, stmts []string) error {
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
var _ migration.Migration = (*ADomainP1Migration)(nil)
