package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// SOPHeatmapIndexMigration SOP 热力图复合索引迁移 v3.27.1
//
// sop_executions 表的热力图查询模式：WHERE sop_id = ? ORDER BY created_at DESC LIMIT N
// 现有索引仅覆盖 sop_id，排序需回表 filesort；复合索引 (sop_id, created_at DESC) 实现索引覆盖。
type SOPHeatmapIndexMigration struct {
	db *gorm.DB
}

// NewSOPHeatmapIndexMigration 创建迁移实例
func NewSOPHeatmapIndexMigration(db *gorm.DB) *SOPHeatmapIndexMigration {
	return &SOPHeatmapIndexMigration{db: db}
}

// Version 返回版本号
func (m *SOPHeatmapIndexMigration) Version() string { return "v3.27.1" }

// Name 返回迁移名称
func (m *SOPHeatmapIndexMigration) Name() string {
	return "SOP 热力图复合索引 (sop_id, created_at DESC)"
}

// Description 返回迁移描述
func (m *SOPHeatmapIndexMigration) Description() string {
	return "为 sop_executions 创建复合索引 idx_sop_executions_sop_created，优化热力图查询 WHERE sop_id = ? ORDER BY created_at DESC LIMIT N"
}

// Up 执行升级
func (m *SOPHeatmapIndexMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 复合索引：sop_id 精确匹配 + created_at 倒序
	// 使用 CONCURRENTLY 避免锁表（需非事务块，gorm 默认开启事务需注意）
	// 这里使用 NOT IN TRANSACTION 检查：gorm.Exec 在单语句时不显式开启事务
	if err := m.db.WithContext(ctx).Exec(`
		CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_sop_executions_sop_created
		ON sop_executions(sop_id, created_at DESC)
	`).Error; err != nil {
		return fmt.Errorf("create composite index failed: %w", err)
	}
	return nil
}

// Down 执行降级
func (m *SOPHeatmapIndexMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	_ = m.db.WithContext(ctx).Exec(`DROP INDEX CONCURRENTLY IF EXISTS idx_sop_executions_sop_created`).Error
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*SOPHeatmapIndexMigration)(nil)