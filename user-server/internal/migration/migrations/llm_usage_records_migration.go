package migrations


import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// LLMUsageRecordsMigration 创建 llm_usage_records 表
type LLMUsageRecordsMigration struct {
	db *gorm.DB
}

// NewLLMUsageRecordsMigration 创建迁移实例
func NewLLMUsageRecordsMigration(db *gorm.DB) *LLMUsageRecordsMigration {
	return &LLMUsageRecordsMigration{db: db}
}

// Version 返回版本号
func (m *LLMUsageRecordsMigration) Version() string { return "v3.4.0" }

// Name 返回迁移名称
func (m *LLMUsageRecordsMigration) Name() string { return "创建 llm_usage_records 表" }

// Description 返回迁移描述
func (m *LLMUsageRecordsMigration) Description() string {
	return "创建 llm_usage_records 表（id, total_tokens, cost, created_at），支撑 AI 生产力 LLM 成本统计"
}

// Up 执行升级
func (m *LLMUsageRecordsMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS llm_usage_records (
			id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			total_tokens bigint      NOT NULL DEFAULT 0,
			cost         numeric(14,4) NOT NULL DEFAULT 0,
			created_at   timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_usage_records_created_at ON llm_usage_records (created_at)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("创建 llm_usage_records 失败: %w", err)
		}
	}
	return nil
}

// Down 回滚
func (m *LLMUsageRecordsMigration) Down(ctx context.Context) error {
	if err := m.db.WithContext(ctx).Exec("DROP TABLE IF EXISTS llm_usage_records").Error; err != nil {
		return fmt.Errorf("删除 llm_usage_records 失败: %w", err)
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*LLMUsageRecordsMigration)(nil)

