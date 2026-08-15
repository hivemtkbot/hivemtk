package migrations

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/model"
)

// LiveCodeClickLogsMigration 补齐活码点击审计日志表：
//   - live_code_click_logs：活码维度逐条点击日志（IP / UA / Referrer），用于安全审计与溯源。
//   - qr_code_click_logs：二维码维度逐条点击日志。
//
// 两张表此前仅有 GORM 模型与仓储，从未在任何迁移中创建，导致审计日志无法落库。
// 使用 AutoMigrate 幂等创建，已部署库与全新库均安全。
type LiveCodeClickLogsMigration struct {
	db *gorm.DB
}

// 编译期断言实现 migration.Migration 接口
var _ migration.Migration = (*LiveCodeClickLogsMigration)(nil)

// NewLiveCodeClickLogsMigration 构造函数
func NewLiveCodeClickLogsMigration(db *gorm.DB) *LiveCodeClickLogsMigration {
	return &LiveCodeClickLogsMigration{db: db}
}

func (m *LiveCodeClickLogsMigration) Version() string { return "v3.20.0" }

func (m *LiveCodeClickLogsMigration) Name() string {
	return "创建活码点击审计日志表"
}

func (m *LiveCodeClickLogsMigration) Description() string {
	return "AutoMigrate live_code_click_logs / qr_code_click_logs 两张审计日志表"
}

func (m *LiveCodeClickLogsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).AutoMigrate(
		&model.LiveCodeClickLog{},
		&model.QRCodeClickLog{},
	); err != nil {
		return fmt.Errorf("LiveCodeClickLogsMigration failed: %w", err)
	}
	return nil
}

func (m *LiveCodeClickLogsMigration) Down(ctx context.Context) error {
	return nil
}

