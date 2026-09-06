package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/pkg/metrics"

	"gorm.io/gorm"
)

// BridgeMetricsMigration 补齐桥接指标落库表：
//   - bridge_metrics：指标定时落库（追加式时间序列），供 SQL 巡检按 (metric_name, ts) 查询趋势。
//
// 私域部署无外部监控，指标通过「应用层日志 + 数据库表查询」
// 两条通道使用；本迁移与 migrations/038_bridge_metrics.sql 严格对齐，AutoMigrate 幂等创建，
// 已部署库与全新库均安全。
type BridgeMetricsMigration struct {
	db *gorm.DB
}

var _ migration.Migration = (*BridgeMetricsMigration)(nil)

// NewBridgeMetricsMigration 构造函数
func NewBridgeMetricsMigration(db *gorm.DB) *BridgeMetricsMigration {
	return &BridgeMetricsMigration{db: db}
}

func (m *BridgeMetricsMigration) Version() string { return "v3.21.0" }

func (m *BridgeMetricsMigration) Name() string {
	return "创建桥接指标落库表"
}

func (m *BridgeMetricsMigration) Description() string {
	return "AutoMigrate bridge_metrics 表（指标定时落库，SQL 巡检趋势查询）"
}

func (m *BridgeMetricsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	if err := m.db.WithContext(ctx).AutoMigrate(&metrics.BridgeMetricRow{}); err != nil {
		return fmt.Errorf("BridgeMetricsMigration failed: %w", err)
	}
	return nil
}

func (m *BridgeMetricsMigration) Down(ctx context.Context) error {
	return nil
}
