package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// ============================================================================
// 方向9：资产包模式迁移
// ----------------------------------------------------------------------------
// 创建两张表：
//  1. asset_bundles              - 资产包主表（OpenAI 兼容 messages JSONB 存储）
//  2. asset_bundle_version_logs  - 资产包版本变更日志
//
// 设计文档：docs/企业级架构优化/资产包模式.md
// 关键约束：
//   - asset_id 业务唯一键 (uniqueIndex)，用于 Weave 时的快速索引
//   - messages 字段是 JSONB，100% 遵守 OpenAI ChatML 协议
//   - tags 字段是 PostgreSQL text[]，支持 && 包含查询
//   - 软删除使用 gorm.DeletedAt
// ============================================================================

// AssetBundleMigration 资产包模式迁移
type AssetBundleMigration struct {
	db *gorm.DB
}

// NewAssetBundleMigration 创建迁移实例
func NewAssetBundleMigration(db *gorm.DB) *AssetBundleMigration {
	return &AssetBundleMigration{db: db}
}

// Version 返回版本号
func (m *AssetBundleMigration) Version() string {
	return "v2.3.0"
}

// Name 返回迁移名称
func (m *AssetBundleMigration) Name() string {
	return "资产包模式 - AssetBundle CRUD + Weave 织布算法"
}

// Description 返回迁移描述
func (m *AssetBundleMigration) Description() string {
	return "创建 asset_bundles / asset_bundle_version_logs 表，支持 OpenAI 兼容 messages 资产包、Weave 织布算法、商户低代码表单"
}

// Up 执行迁移
func (m *AssetBundleMigration) Up(ctx context.Context) error {
	// 1. 创建 asset_bundles 表（使用 AutoMigrate 让 GORM 处理 PostgreSQL 数组类型 + JSONB）
	if err := m.db.AutoMigrate(&model.AssetBundle{}); err != nil {
		return fmt.Errorf("migrate asset_bundles failed: %w", err)
	}

	// 2. 创建 asset_bundle_version_logs 表
	if err := m.db.AutoMigrate(&model.AssetBundleVersionLog{}); err != nil {
		return fmt.Errorf("migrate asset_bundle_version_logs failed: %w", err)
	}

	return nil
}

// Down 回滚
func (m *AssetBundleMigration) Down(ctx context.Context) error {
	// 按依赖顺序反向删除
	if m.db.Migrator().HasTable("asset_bundle_version_logs") {
		_ = m.db.Migrator().DropTable("asset_bundle_version_logs")
	}
	if m.db.Migrator().HasTable("asset_bundles") {
		_ = m.db.Migrator().DropTable("asset_bundles")
	}
	return nil
}

// 编译期接口断言
var _ migration.Migration = (*AssetBundleMigration)(nil)
