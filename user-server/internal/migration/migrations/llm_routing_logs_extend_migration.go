package migrations

// llm_routing_logs_extend_migration.go 扩展 llm_routing_logs 表
//
// 背景：
//   v3.6.0 创建 llm_routing_logs 时仅含基础字段（trace_id/scenario/provider/model/
//   prompt_tokens/completion_tokens/total_tokens/cost/latency_ms/success/error_msg/from_cache）。
//   但项目硬约束要求每次 dispatch 落库必须携带：
//     - model_type     （local / cloud）
//     - vendor         （厂商维度归集，inferVendor(BaseURL) 自动判定）
//     - base_url       （出域审计）
//     - is_fallback    （降级率统计 / 出域审计）
//     - prompt_cost    （prompt 单价计费）
//     - completion_cost（completion 单价计费）
//     - token_source   （actual / estimated / missing，用于 API 完整性告警）
//     - estimator      （empty_fallback / char_weight 等估算器标识）
//     - source         （dispatch / cache / fallback / null）
//     - scenario_provider（聚合键，加快 GROUP BY 查询）
//
// 设计：ADD COLUMN IF NOT EXISTS（PG 9.6+），幂等可重入，老数据不补 NULL 默认值。

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// LLMRoutingLogsExtendMigration 扩展 llm_routing_logs 表字段（v3.7.0）
type LLMRoutingLogsExtendMigration struct {
	db *gorm.DB
}

// NewLLMRoutingLogsExtendMigration 创建扩展迁移实例
func NewLLMRoutingLogsExtendMigration(db *gorm.DB) *LLMRoutingLogsExtendMigration {
	return &LLMRoutingLogsExtendMigration{db: db}
}

// Version 返回版本号
func (m *LLMRoutingLogsExtendMigration) Version() string { return "v3.7.0" }

// Name 返回迁移名称
func (m *LLMRoutingLogsExtendMigration) Name() string {
	return "扩展 llm_routing_logs 字段（model_type / vendor / base_url / is_fallback / cost_split / token_source）"
}

// Description 描述
func (m *LLMRoutingLogsExtendMigration) Description() string {
	return "为支撑本地/云端 token 计量三档（actual/estimated/missing）、厂商成本归集、出域审计、降级率统计，扩展 llm_routing_logs 表的 10 个字段"
}

// Up 执行升级
func (m *LLMRoutingLogsExtendMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	// PostgreSQL 9.6+ 支持 ADD COLUMN IF NOT EXISTS，幂等
	stmts := []string{
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS model_type      VARCHAR(16)  NOT NULL DEFAULT 'local'`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS vendor         VARCHAR(64)  NOT NULL DEFAULT 'unknown'`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS base_url       VARCHAR(512) NOT NULL DEFAULT ''`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS is_fallback    BOOLEAN      NOT NULL DEFAULT FALSE`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS prompt_cost      NUMERIC(14,6) NOT NULL DEFAULT 0`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS completion_cost  NUMERIC(14,6) NOT NULL DEFAULT 0`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS token_source   VARCHAR(16)  NOT NULL DEFAULT 'missing'`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS estimator      VARCHAR(32)  NOT NULL DEFAULT ''`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS source         VARCHAR(32)  NOT NULL DEFAULT 'dispatch'`,
		`ALTER TABLE llm_routing_logs ADD COLUMN IF NOT EXISTS scenario_provider VARCHAR(160) NOT NULL DEFAULT ''`,
		// 复合索引：场景 + 厂商 + 时间（按厂商维度统计 / 出域审计）
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_scenario_provider ON llm_routing_logs (scenario, provider, created_at DESC)`,
		// 复合索引：本地/云端分类 + 时间（按 model_type 维度统计）
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_model_type_created ON llm_routing_logs (model_type, created_at DESC)`,
		// 复合索引：token_source + 时间（监控 missing 占比）
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_token_source ON llm_routing_logs (token_source, created_at DESC)`,
		// 复合索引：vendor + 时间（按厂商聚合成本）
		`CREATE INDEX IF NOT EXISTS idx_llm_routing_logs_vendor ON llm_routing_logs (vendor, created_at DESC)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("llm_routing_logs 扩展字段失败 (%s): %w", s[:min(40, len(s))], err)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Down 回滚
func (m *LLMRoutingLogsExtendMigration) Down(ctx context.Context) error {
	stmts := []string{
		"DROP INDEX IF EXISTS idx_llm_routing_logs_vendor",
		"DROP INDEX IF EXISTS idx_llm_routing_logs_token_source",
		"DROP INDEX IF EXISTS idx_llm_routing_logs_model_type_created",
		"DROP INDEX IF EXISTS idx_llm_routing_logs_scenario_provider",
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS scenario_provider`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS source`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS estimator`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS token_source`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS completion_cost`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS prompt_cost`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS is_fallback`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS base_url`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS vendor`,
		`ALTER TABLE llm_routing_logs DROP COLUMN IF EXISTS model_type`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("llm_routing_logs 扩展回滚失败: %w", err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*LLMRoutingLogsExtendMigration)(nil)
