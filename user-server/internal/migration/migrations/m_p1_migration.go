package migrations


import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// MP1Migration 缺口修复迁移 v3.1.0
type MP1Migration struct {
	db *gorm.DB
}

// NewMP1Migration 创建迁移实例
func NewMP1Migration(db *gorm.DB) *MP1Migration {
	return &MP1Migration{db: db}
}

// Version 返回版本号
func (m *MP1Migration) Version() string { return "v3.1.0" }

// Name 返回迁移名称
func (m *MP1Migration) Name() string { return "M 域 P1 缺口修复（4 张表）" }

// Description 返回迁移描述
func (m *MP1Migration) Description() string {
	return "创建 provider_health / system_kv_config / intent_logs / trace_events 4 张表，支撑 LLM Provider 降级、精细意图识别、全链路追踪"
}

// Up 执行升级
func (m *MP1Migration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.createSystemKVConfig(ctx); err != nil {
		return fmt.Errorf("create system_kv_config 失败: %w", err)
	}

	if err := m.createProviderHealth(ctx); err != nil {
		return fmt.Errorf("create provider_health 失败: %w", err)
	}

	if err := m.createIntentLogs(ctx); err != nil {
		return fmt.Errorf("create intent_logs 失败: %w", err)
	}

	if err := m.createTraceEvents(ctx); err != nil {
		return fmt.Errorf("create trace_events 失败: %w", err)
	}

	if err := m.seedDefaultFailoverPolicy(ctx); err != nil {
		_ = err
	}

	return nil
}

// createSystemKVConfig 创建 system_kv_config 表（键值配置存储）
func (m *MP1Migration) createSystemKVConfig(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS system_kv_config (
			id          BIGSERIAL PRIMARY KEY,
			key         VARCHAR(128) NOT NULL UNIQUE,
			value       TEXT NOT NULL DEFAULT '',
			value_type  VARCHAR(20) NOT NULL DEFAULT 'json',
			description VARCHAR(500) DEFAULT '',
			updated_by  BIGINT DEFAULT 0,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_kv_config_key ON system_kv_config(key)`,
	}
	return execAllMP1(ctx, m.db, stmts)
}

// createProviderHealth 创建 provider_health 表
// 注意：运行期 ProviderFailover 使用内存 map 维护健康状态，
// 此表用于跨进程共享/重启恢复场景（可选持久化）
func (m *MP1Migration) createProviderHealth(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS provider_health (
			id                  BIGSERIAL PRIMARY KEY,
			provider_name       VARCHAR(64) NOT NULL UNIQUE,
			status              VARCHAR(20) NOT NULL DEFAULT 'up',
			last_check          TIMESTAMPTZ,
			last_error          TEXT DEFAULT '',
			consecutive_failures INT NOT NULL DEFAULT 0,
			circuit_open_until  TIMESTAMPTZ,
			latency_p95_ms      BIGINT DEFAULT 0,
			metadata            JSONB DEFAULT '{}',
			created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_provider_health_status ON provider_health(status)`,
	}
	return execAllMP1(ctx, m.db, stmts)
}

// createIntentLogs 创建 intent_logs 表
func (m *MP1Migration) createIntentLogs(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS intent_logs (
			id           BIGSERIAL PRIMARY KEY,
			customer_id  VARCHAR(64) NOT NULL DEFAULT '',
			session_id   VARCHAR(50) NOT NULL DEFAULT '',
			message      TEXT NOT NULL,
			intent_major VARCHAR(32) NOT NULL,
			intent_minor VARCHAR(32) NOT NULL,
			confidence   DECIMAL(5,4) NOT NULL DEFAULT 0,
			method       VARCHAR(16) NOT NULL DEFAULT 'rule',
			latency_ms   INT NOT NULL DEFAULT 0,
			reasoning    TEXT DEFAULT '',
			trace_id     VARCHAR(64) DEFAULT '',
			timestamp    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_customer ON intent_logs(customer_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_session ON intent_logs(session_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_major ON intent_logs(intent_major, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_minor ON intent_logs(intent_minor, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_trace ON intent_logs(trace_id) WHERE trace_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_intent_logs_timestamp ON intent_logs(timestamp DESC)`,
	}
	return execAllMP1(ctx, m.db, stmts)
}

// createTraceEvents 创建 trace_events 表
func (m *MP1Migration) createTraceEvents(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS trace_events (
			id              BIGSERIAL PRIMARY KEY,
			trace_id        VARCHAR(64) NOT NULL,
			span_id         VARCHAR(64) NOT NULL UNIQUE,
			parent_span_id  VARCHAR(64) DEFAULT '',
			kind            VARCHAR(32) NOT NULL,
			service         VARCHAR(64) NOT NULL,
			operation       VARCHAR(128) NOT NULL,
			duration_ms     BIGINT NOT NULL DEFAULT 0,
			status          VARCHAR(16) NOT NULL DEFAULT 'ok',
			metadata        TEXT DEFAULT '{}',
			timestamp       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_trace ON trace_events(trace_id, timestamp ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_parent ON trace_events(parent_span_id) WHERE parent_span_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_kind ON trace_events(kind, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_service ON trace_events(service, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_status ON trace_events(status) WHERE status != 'ok'`,
		`CREATE INDEX IF NOT EXISTS idx_trace_events_timestamp ON trace_events(timestamp DESC)`,
	}
	return execAllMP1(ctx, m.db, stmts)
}

// seedDefaultFailoverPolicy 种子化默认降级策略
// 幂等：仅当 key 不存在时插入
func (m *MP1Migration) seedDefaultFailoverPolicy(ctx context.Context) error {
	policyJSON := `{"config":{"health_check_interval":30,"failure_threshold":5,"circuit_open_duration":60,"degraded_latency_ms":3000,"local_fallback_provider":"default","template_reply":"抱歉，当前服务暂时繁忙，请稍后再试或联系人工客服。","health_check_path":"/health"},"scenarios":{"intent_recognize":["default","deepseek","qwen"],"sop_reply":["default","gpt-4o","glm-4"],"objection":["default","gpt-4o","glm-4"],"friendly_chat":["default","deepseek"],"long_summary":["default","kimi","qwen"],"high_quality":["default","gpt-4o","glm-4"],"low_cost":["default","deepseek"]}}`

	return m.db.WithContext(ctx).Exec(
		`INSERT INTO system_kv_config (key, value, value_type, description) 
		 SELECT 'llm_provider_failover', $1, 'json', 'LLM Provider 降级策略配置'
		 WHERE NOT EXISTS (SELECT 1 FROM system_kv_config WHERE key = 'llm_provider_failover')`,
		policyJSON,
	).Error
}

// Down 回滚
func (m *MP1Migration) Down(ctx context.Context) error {
	stmts := []string{
		`DROP TABLE IF EXISTS trace_events`,
		`DROP TABLE IF EXISTS intent_logs`,
		`DROP TABLE IF EXISTS provider_health`,
	}
	return execAllMP1(ctx, m.db, stmts)
}

// execAllMP1 批量执行 SQL（出错即返回）
func execAllMP1(ctx context.Context, db *gorm.DB, stmts []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	for _, sql := range stmts {
		if err := db.WithContext(ctx).Exec(sql).Error; err != nil {
			return fmt.Errorf("exec failed (%s): %w", sql, err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*MP1Migration)(nil)

