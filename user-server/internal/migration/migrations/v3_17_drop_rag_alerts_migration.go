package migrations

import (
	"context"
	"fmt"
	"log"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// RagAlertsDropMigration 清理孤儿表 rag_alerts (v3.17)
type RagAlertsDropMigration struct {
	db *gorm.DB
}

// NewRagAlertsDropMigration 创建迁移实例
func NewRagAlertsDropMigration(db *gorm.DB) *RagAlertsDropMigration {
	return &RagAlertsDropMigration{db: db}
}

// Version 返回版本号
func (m *RagAlertsDropMigration) Version() string { return "v3.17.0" }

// Name 返回迁移名称
func (m *RagAlertsDropMigration) Name() string {
	return "清理孤儿表 rag_alerts (commit 4 二次审查)"
}

// Description 返回迁移描述
func (m *RagAlertsDropMigration) Description() string {
	return "2026-08-01 二次深度审查: RagAlertService 已删, rag_alerts 表无任何 Go 引用, 4 个索引全是孤儿。DROP 4 索引 + DROP 表。"
}

// Up 执行迁移
func (m *RagAlertsDropMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Println("[v3.17] 开始清理孤儿表 rag_alerts ...")

	indexes := []string{
		"idx_rag_alerts_type",
		"idx_rag_alerts_severity",
		"idx_rag_alerts_resolved",
		"idx_rag_alerts_created",
	}
	for i, idx := range indexes {
		log.Printf("[v3.17] %d/5 DROP INDEX %s ...", i+1, idx)
		if err := m.db.WithContext(ctx).Exec(
			fmt.Sprintf(`DROP INDEX IF EXISTS %s`, idx),
		).Error; err != nil {
			return fmt.Errorf("v3.17 DROP 索引 %s 失败: %w", idx, err)
		}
		log.Printf("[v3.17] ✓ %s 已删除", idx)
	}

	log.Println("[v3.17] 5/5 DROP TABLE rag_alerts ...")
	if err := m.db.WithContext(ctx).Exec(
		`DROP TABLE IF EXISTS rag_alerts`,
	).Error; err != nil {
		return fmt.Errorf("v3.17 5/5 DROP 表 rag_alerts 失败: %w", err)
	}
	log.Println("[v3.17] ✓ rag_alerts 表已删除")

	log.Println("[v3.17] 孤儿表清理完成")
	return nil
}

// Down 执行回滚 (谨慎: 重建空表, 不补任何数据)
func (m *RagAlertsDropMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Println("[v3.17] 回滚: 重建空表 rag_alerts ...")
	stmt := `
CREATE TABLE IF NOT EXISTS rag_alerts (
    id            BIGSERIAL    PRIMARY KEY,
    alert_type    VARCHAR(32)  NOT NULL,
    severity      VARCHAR(16)  NOT NULL DEFAULT 'message',
    metric_value  DECIMAL(10,4) NOT NULL,
    threshold     DECIMAL(10,4) NOT NULL,
    message       TEXT         NOT NULL,
    window_start  TIMESTAMP    NOT NULL,
    window_end    TIMESTAMP    NOT NULL,
    resolved      BOOLEAN      NOT NULL DEFAULT FALSE,
    resolved_at   TIMESTAMP,
    resolved_by   VARCHAR(64),
    resolve_note  TEXT,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_rag_alerts_type     ON rag_alerts (alert_type, created_at);
CREATE INDEX IF NOT EXISTS idx_rag_alerts_severity ON rag_alerts (severity);
CREATE INDEX IF NOT EXISTS idx_rag_alerts_resolved ON rag_alerts (resolved);
CREATE INDEX IF NOT EXISTS idx_rag_alerts_created  ON rag_alerts (created_at);
`
	if err := m.db.WithContext(ctx).Exec(stmt).Error; err != nil {
		return fmt.Errorf("v3.17 回滚重建 rag_alerts 失败: %w", err)
	}
	log.Println("[v3.17] ✓ 回滚完成")
	return nil
}

var _ migration.Migration = (*RagAlertsDropMigration)(nil)
