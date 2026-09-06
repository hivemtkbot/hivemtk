package migrations

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// AlertRuleMigration 告警规则引擎表结构迁移（plan v3.1 §T8）
//
// 创建两张表：
//   - alert_rules: 规则定义（source/operator/threshold/window/cooldown/channels/targets）
//   - alert_histories: 触发快照历史（用于审计、恢复、查重）
//
// 幂等：CREATE TABLE IF NOT EXISTS，可重入。
type AlertRuleMigration struct {
	db *gorm.DB
}

// NewAlertRuleMigration 构造
func NewAlertRuleMigration(db *gorm.DB) *AlertRuleMigration {
	return &AlertRuleMigration{db: db}
}

// Version 返回版本号
func (m *AlertRuleMigration) Version() string { return "v3.23.0" }

// Name 返回迁移名称
func (m *AlertRuleMigration) Name() string { return "告警规则引擎" }

// Description 返回迁移描述
func (m *AlertRuleMigration) Description() string {
	return "创建 alert_rules / alert_histories 两张表，支持告警规则 CRUD、分级、冷却期、通知渠道与触发历史"
}

// Up 执行升级
func (m *AlertRuleMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.createAlertRulesTable(ctx); err != nil {
		return fmt.Errorf("创建 alert_rules 表失败: %w", err)
	}
	if err := m.createAlertHistoriesTable(ctx); err != nil {
		return fmt.Errorf("创建 alert_histories 表失败: %w", err)
	}
	return nil
}

// Down 执行降级（不删数据，仅 negocio 记录）
func (m *AlertRuleMigration) Down(ctx context.Context) error {

	return nil
}

func (m *AlertRuleMigration) createAlertRulesTable(_ context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id BIGSERIAL PRIMARY KEY,
			name VARCHAR(100) NOT NULL,
			description VARCHAR(500),
			source VARCHAR(100) NOT NULL,
			operator VARCHAR(10) NOT NULL DEFAULT 'gt',
			threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
			window_seconds INT NOT NULL DEFAULT 60,
			cooldown_seconds INT NOT NULL DEFAULT 300,
			severity VARCHAR(20) DEFAULT 'warning',
			channels JSONB DEFAULT '[]'::jsonb,
			targets JSONB DEFAULT '{}'::jsonb,
			enabled BOOLEAN DEFAULT TRUE NOT NULL,
			last_triggered_at TIMESTAMP,
			created_by BIGINT DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			CONSTRAINT uk_alert_rules_name UNIQUE (name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_source ON alert_rules(source)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_enabled ON alert_rules(enabled)`,
	}
	for _, s := range stmts {
		if err := m.db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

func (m *AlertRuleMigration) createAlertHistoriesTable(_ context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS alert_histories (
			id BIGSERIAL PRIMARY KEY,
			rule_id BIGINT NOT NULL,
			rule_name VARCHAR(100),
			source VARCHAR(100),
			value DOUBLE PRECISION,
			threshold DOUBLE PRECISION,
			severity VARCHAR(20),
			message TEXT,
			status VARCHAR(20) DEFAULT 'firing',
			channels JSONB DEFAULT '[]'::jsonb,
			notify_result TEXT,
			triggered_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			deleted_at TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_rule_id ON alert_histories(rule_id)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_source ON alert_histories(source)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_severity ON alert_histories(severity)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_status ON alert_histories(status)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_triggered_at ON alert_histories(triggered_at)`,
		`CREATE INDEX IF NOT EXISTS idx_alert_histories_deleted_at ON alert_histories(deleted_at)`,
	}
	for _, s := range stmts {
		if err := m.db.Exec(s).Error; err != nil {
			return err
		}
	}
	return nil
}

var _ migration.Migration = (*AlertRuleMigration)(nil)
