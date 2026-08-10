package migrations

// bridge_account_migration.go 创建 bridge_accounts 表
//
// 背景：网页私信桥接（抖音/小红书/TikTok）扩展连接时（register 帧）自动注册账号，
// 并记录归属用户、绑定智能体、在线状态与最后同步时间。该表同时驱动桥接账号管理
// 路由（列出/人工代发）与账号归属校验（G4 防水平越权）。
//
// 采用 AutoMigrate 建表，由 GORM 依据 struct tag 自动推导列与索引；幂等可重入。

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"

	"gorm.io/gorm"
)

// BridgeAccountMigration 创建 bridge_accounts 表
type BridgeAccountMigration struct {
	db *gorm.DB
}

func NewBridgeAccountMigration(db *gorm.DB) *BridgeAccountMigration {
	return &BridgeAccountMigration{db: db}
}

func (m *BridgeAccountMigration) Version() string { return "v3.11.0" }

func (m *BridgeAccountMigration) Name() string { return "创建 bridge_accounts 表" }

func (m *BridgeAccountMigration) Description() string {
	return "创建 bridge_accounts（网页私信桥接账号）表，支撑抖音/小红书/TikTok 私信桥接的账号注册、归属校验与管理"
}

func (m *BridgeAccountMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).AutoMigrate(&model.BridgeAccount{}); err != nil {
		return fmt.Errorf("bridge_accounts 建表失败: %w", err)
	}
	return nil
}

func (m *BridgeAccountMigration) Down(ctx context.Context) error {
	if err := m.db.WithContext(ctx).Migrator().DropTable(&model.BridgeAccount{}); err != nil {
		return fmt.Errorf("bridge_accounts 回滚失败: %w", err)
	}
	return nil
}

var _ migration.Migration = (*BridgeAccountMigration)(nil)
