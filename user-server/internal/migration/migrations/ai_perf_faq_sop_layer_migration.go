package migrations

// ai_perf_faq_sop_layer_migration.go AI 智能体性能优化
//
// 背景：
// 1. 当前 user-server 客服对话 wall time 平均 19.6s（7B Q5 本地 LLM 实测），主要由 Step 3
//    意图识别 LLM 串行阻塞造成。
// 2. 改造方案：双层架构 (Layer1 FAQ/SOP 模板 <100ms → SkipLLM / Layer2 LLM 兜底 1-3s)
//    + Go 并行化 (errgroup Phase 0 fan-out) + WebSocket 流式输出 (LCP <500ms)。
// 3. 本次迁移新增 3 张表支撑该方案：
//    - faq_entries          Layer1 FAQ 知识库
//    - sop_templates        Layer1 SOP 模板（Go text/template 变量替换）
//    - layer_decision_logs  Layer 决策可观测性日志（私域: 落库审计 + SQL 巡检）
// 4. 3 张表均带 GIN 索引或常规索引，PG AutoMigrate 友好，幂等可重入。

import (
	"context"
	"fmt"

	"marketing/internal/migration"

	"gorm.io/gorm"
)

// AIPerfFAQSOPLayerMigration AI 性能优化 - FAQ / SOP / Layer 日志三表
type AIPerfFAQSOPLayerMigration struct {
	db *gorm.DB
}

// NewAIPerfFAQSOPLayerMigration 创建迁移实例
func NewAIPerfFAQSOPLayerMigration(db *gorm.DB) *AIPerfFAQSOPLayerMigration {
	return &AIPerfFAQSOPLayerMigration{db: db}
}

// Version 返回版本号
func (m *AIPerfFAQSOPLayerMigration) Version() string { return "v3.13.0" }

// Name 返回迁移名称
func (m *AIPerfFAQSOPLayerMigration) Name() string {
	return "AI 性能优化 - FAQ/SOP 知识库 + Layer 决策日志"
}

// Description 返回迁移描述
func (m *AIPerfFAQSOPLayerMigration) Description() string {
	return "2026-07-31 AI 智能体性能优化：创建 faq_entries / sop_templates / layer_decision_logs 三张表，支撑双层架构 (Layer1 FAQ/SOP 模板 <100ms + Layer2 LLM 兜底)，同时提供 Layer 决策可观测性"
}

// Up 执行迁移
func (m *AIPerfFAQSOPLayerMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		// 1. faq_entries - Layer1 FAQ 知识库
		`CREATE TABLE IF NOT EXISTS faq_entries (
			id          BIGSERIAL PRIMARY KEY,
			question    TEXT            NOT NULL,
			answer      TEXT            NOT NULL,
			keywords    TEXT[]          NOT NULL DEFAULT '{}',
			category    VARCHAR(64)     NOT NULL DEFAULT '',
			intent      VARCHAR(64)     NOT NULL DEFAULT '',
			confidence  NUMERIC(5,4)    NOT NULL DEFAULT 0,
			hit_count   BIGINT          NOT NULL DEFAULT 0,
			enabled     BOOLEAN         NOT NULL DEFAULT TRUE,
			created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_faq_entries_enabled ON faq_entries (enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_faq_entries_intent ON faq_entries (intent)`,
		`CREATE INDEX IF NOT EXISTS idx_faq_entries_category ON faq_entries (category)`,
		`CREATE INDEX IF NOT EXISTS idx_faq_entries_hit_count ON faq_entries (hit_count DESC) WHERE enabled = TRUE`,
		// GIN 索引: 全文检索 (MatchByKeyword 兜底)
		`CREATE INDEX IF NOT EXISTS idx_faq_entries_question_gin ON faq_entries USING gin(to_tsvector('simple', question))`,

		// 2. sop_templates - Layer1 SOP 模板
		`CREATE TABLE IF NOT EXISTS sop_templates (
			id          BIGSERIAL PRIMARY KEY,
			name        VARCHAR(100)    NOT NULL,
			intent      VARCHAR(64)     NOT NULL,
			stage       VARCHAR(32)     NOT NULL,
			template    TEXT            NOT NULL,
			vars        TEXT            NOT NULL DEFAULT '',
			priority    INT             NOT NULL DEFAULT 0,
			confidence  NUMERIC(5,4)    NOT NULL DEFAULT 0.8,
			enabled     BOOLEAN         NOT NULL DEFAULT TRUE,
			hit_count   BIGINT          NOT NULL DEFAULT 0,
			created_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ     NOT NULL DEFAULT NOW()
		)`,
		`ALTER TABLE sop_templates ADD COLUMN IF NOT EXISTS hit_count BIGINT NOT NULL DEFAULT 0`,
		`CREATE INDEX IF NOT EXISTS idx_sop_templates_intent_stage ON sop_templates (intent, stage) WHERE enabled = TRUE`,
		`CREATE INDEX IF NOT EXISTS idx_sop_templates_enabled ON sop_templates (enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_sop_templates_priority ON sop_templates (priority DESC) WHERE enabled = TRUE`,

		// 3. layer_decision_logs - Layer 决策可观测性
		`CREATE TABLE IF NOT EXISTS layer_decision_logs (
			id           BIGSERIAL PRIMARY KEY,
			trace_id     VARCHAR(64)     NOT NULL,
			session_id   VARCHAR(50)     NOT NULL DEFAULT '',
			customer_id  VARCHAR(64)     NOT NULL DEFAULT '',
			layer        VARCHAR(32)     NOT NULL,
			reason       VARCHAR(64)     NOT NULL,
			intent       VARCHAR(64)     NOT NULL DEFAULT '',
			conf_in      NUMERIC(5,4)    NOT NULL DEFAULT 0,
			conf_out     NUMERIC(5,4)    NOT NULL DEFAULT 0,
			wall_ms      INT             NOT NULL DEFAULT 0,
			llm_skipped  BOOLEAN         NOT NULL DEFAULT FALSE,
			extra        TEXT            NOT NULL DEFAULT '',
			created_at   TIMESTAMPTZ     NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_layer_decision_logs_trace_id ON layer_decision_logs (trace_id)`,
		`CREATE INDEX IF NOT EXISTS idx_layer_decision_logs_session_id ON layer_decision_logs (session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_layer_decision_logs_created_at ON layer_decision_logs (created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_layer_decision_logs_layer ON layer_decision_logs (layer)`,
		`CREATE INDEX IF NOT EXISTS idx_layer_decision_logs_intent ON layer_decision_logs (intent)`,
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("ai_perf_faq_sop_layer 建表失败: %w (SQL: %s)", err, s)
		}
	}
	return nil
}

// Down 回滚
func (m *AIPerfFAQSOPLayerMigration) Down(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}
	stmts := []string{
		// layer_decision_logs 索引 + 表
		"DROP INDEX IF EXISTS idx_layer_decision_logs_intent",
		"DROP INDEX IF EXISTS idx_layer_decision_logs_layer",
		"DROP INDEX IF EXISTS idx_layer_decision_logs_created_at",
		"DROP INDEX IF EXISTS idx_layer_decision_logs_session_id",
		"DROP INDEX IF EXISTS idx_layer_decision_logs_trace_id",
		"DROP TABLE IF EXISTS layer_decision_logs",
		// sop_templates 索引 + 表
		"DROP INDEX IF EXISTS idx_sop_templates_priority",
		"DROP INDEX IF EXISTS idx_sop_templates_enabled",
		"DROP INDEX IF EXISTS idx_sop_templates_intent_stage",
		"DROP TABLE IF EXISTS sop_templates",
		// faq_entries 索引 + 表
		"DROP INDEX IF EXISTS idx_faq_entries_question_gin",
		"DROP INDEX IF EXISTS idx_faq_entries_hit_count",
		"DROP INDEX IF EXISTS idx_faq_entries_category",
		"DROP INDEX IF EXISTS idx_faq_entries_intent",
		"DROP INDEX IF EXISTS idx_faq_entries_enabled",
		"DROP TABLE IF EXISTS faq_entries",
	}
	for _, s := range stmts {
		if err := m.db.WithContext(ctx).Exec(s).Error; err != nil {
			return fmt.Errorf("ai_perf_faq_sop_layer 回滚失败: %w (SQL: %s)", err, s)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*AIPerfFAQSOPLayerMigration)(nil)
