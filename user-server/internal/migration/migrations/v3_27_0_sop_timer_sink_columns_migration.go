package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// SOPTimerSinkColumnsMigration M4 sop_timers 列下沉迁移 v3.27.0
//
// expires_at / max_wait_at / claim_count 原先仅存于 Payload JSONB，无法建索引、
// 无法用 SQL 条件扫描。本迁移补齐实体列并创建 pending 状态部分索引；
// 列定义与 model.SOPTimer 一致（AutoMigrate 也会自动补列，本迁移额外负责
// 历史数据回填与部分索引——AutoMigrate 不支持部分索引）。
type SOPTimerSinkColumnsMigration struct {
	db *gorm.DB
}

// NewSOPTimerSinkColumnsMigration 创建迁移实例
func NewSOPTimerSinkColumnsMigration(db *gorm.DB) *SOPTimerSinkColumnsMigration {
	return &SOPTimerSinkColumnsMigration{db: db}
}

// Version 返回版本号
func (m *SOPTimerSinkColumnsMigration) Version() string { return "v3.27.0" }

// Name 返回迁移名称
func (m *SOPTimerSinkColumnsMigration) Name() string {
	return "sop_timers 列下沉（expires_at/max_wait_at/claim_count + 部分索引）"
}

// Description 返回迁移描述
func (m *SOPTimerSinkColumnsMigration) Description() string {
	return "sop_timers 补齐 expires_at/max_wait_at/claim_count 实体列、回填历史数据、建 pending 部分索引"
}

// Up 执行升级
func (m *SOPTimerSinkColumnsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		// 1. 补列（幂等）
		`ALTER TABLE sop_timers ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ`,
		`ALTER TABLE sop_timers ADD COLUMN IF NOT EXISTS max_wait_at TIMESTAMPTZ`,
		`ALTER TABLE sop_timers ADD COLUMN IF NOT EXISTS claim_count INTEGER NOT NULL DEFAULT 0`,
		// 2. 历史数据回填：仅回填列值为 NULL 的行（payload 字段非法时保持 NULL，扫描时回退 payload）
		`UPDATE sop_timers SET expires_at = (payload->>'expires_at')::timestamptz
		 WHERE expires_at IS NULL AND payload->>'expires_at' ~ '^\d{4}-\d{2}-\d{2}T'`,
		`UPDATE sop_timers SET max_wait_at = (payload->>'max_wait_at')::timestamptz
		 WHERE max_wait_at IS NULL AND payload->>'max_wait_at' ~ '^\d{4}-\d{2}-\d{2}T'`,
		`UPDATE sop_timers SET claim_count = (payload->>'claim_count')::int
		 WHERE claim_count = 0 AND payload->>'claim_count' ~ '^\d+$'`,
		// 3. 部分索引：只索引 pending 行（M4 目标——过期/死信扫描可走索引）
		`CREATE INDEX IF NOT EXISTS idx_sop_timers_pending_wait_until ON sop_timers(wait_until) WHERE status = 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_sop_timers_pending_max_wait_at ON sop_timers(max_wait_at) WHERE status = 'pending' AND max_wait_at IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS idx_sop_timers_pending_claim_count ON sop_timers(claim_count) WHERE status = 'pending' AND claim_count >= 1`,
	}
	for _, sql := range stmts {
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

// Down 执行降级（删除部分索引；下沉列保留——删除会丢数据，降级仅移除索引收益）
func (m *SOPTimerSinkColumnsMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`DROP INDEX IF EXISTS idx_sop_timers_pending_claim_count`,
		`DROP INDEX IF EXISTS idx_sop_timers_pending_max_wait_at`,
		`DROP INDEX IF EXISTS idx_sop_timers_pending_wait_until`,
	}
	for _, sql := range stmts {
		if err := m.db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*SOPTimerSinkColumnsMigration)(nil)
