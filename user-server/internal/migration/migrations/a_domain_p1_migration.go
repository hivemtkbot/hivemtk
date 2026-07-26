package migrations

// a_domain_p1_migration.go A 域 P1 缺口修复占位（v2.11.0 版本号占位，Up/Down 为 no-op）
//
// data_scope 字段保留在 system_users 表。

import (
	"context"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// ADomainP1Migration A 域 P1 缺口修复迁移（no-op）
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
func (m *ADomainP1Migration) Name() string {
	return "A 域 P1 缺口修复 - 团队用户表已并入 system_users"
}

// Description 返回迁移描述
func (m *ADomainP1Migration) Description() string {
	return "阶段 1 单表化重构：team_users 表已 DROP，data_scope 字段保留在 system_users"
}

// Up 执行升级（no-op，team_users 已 DROP）
func (m *ADomainP1Migration) Up(ctx context.Context) error {
	// team_users 表已在 025 单表化迁移中 DROP，本迁移保留版本号占位即可
	return nil
}

// Down 执行降级（no-op）
func (m *ADomainP1Migration) Down(ctx context.Context) error {
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*ADomainP1Migration)(nil)
