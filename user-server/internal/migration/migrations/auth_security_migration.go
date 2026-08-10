package migrations

// auth_security_migration.go 认证与安全相关表结构迁移 v2.10.0
//
// 五层架构归属: L5 数据层
// 设计依据: docs/standards/MASTER_RULES.md「私域独立部署，无 merchant_id 字段」
// MFA / 异常登录预警 / 密码策略 / 行级权限
//
// 本迁移创建 4 张表：
//  1. user_mfa          用户 MFA 配置（TOTP/HOTP）
//  2. login_events      登录事件审计（含风险评估）
//  3. security_alerts   安全告警（high/critical 风险触发）
//  4. password_history  密码历史（forbid_reuse 策略）
//
// 幂等性: 全部使用 CREATE TABLE IF NOT EXISTS，可重入
// 依赖: system_users 表已存在（由 initial_schema 创建）
// 私域部署: 无 merchant_id 字段

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// AuthSecurityMigration 认证与安全相关表结构迁移
type AuthSecurityMigration struct {
	db *gorm.DB
}

// NewAuthSecurityMigration 创建迁移实例
func NewAuthSecurityMigration(db *gorm.DB) *AuthSecurityMigration {
	return &AuthSecurityMigration{db: db}
}

// Version 返回版本号
func (m *AuthSecurityMigration) Version() string { return "v2.10.0" }

// Name 返回迁移名称
func (m *AuthSecurityMigration) Name() string { return "认证与安全相关表结构" }

// Description 返回迁移描述
func (m *AuthSecurityMigration) Description() string {
	return "创建 user_mfa / login_events / security_alerts / password_history 4 张表，支持 MFA / 异常登录预警 / 密码策略 / 行级权限"
}

// Up 执行升级
func (m *AuthSecurityMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1. user_mfa 表（MFA 多因素认证）
	if err := m.createUserMFATable(ctx); err != nil {
		return fmt.Errorf("创建 user_mfa 表失败: %w", err)
	}

	// 2. login_events 表（异常登录预警 - 事件审计）
	if err := m.createLoginEventsTable(ctx); err != nil {
		return fmt.Errorf("创建 login_events 表失败: %w", err)
	}

	// 3. security_alerts 表（异常登录预警 - 告警）
	if err := m.createSecurityAlertsTable(ctx); err != nil {
		return fmt.Errorf("创建 security_alerts 表失败: %w", err)
	}

	// 4. password_history 表（密码策略 - 历史密码复用检查）
	if err := m.createPasswordHistoryTable(ctx); err != nil {
		return fmt.Errorf("创建 password_history 表失败: %w", err)
	}

	// 5. system_config_kv 表（密码策略 - 存储密码策略配置 JSON）
	if err := m.createSystemConfigKVTable(ctx); err != nil {
		return fmt.Errorf("创建 system_config_kv 表失败: %w", err)
	}

	return nil
}

// createUserMFATable 创建 user_mfa 表
func (m *AuthSecurityMigration) createUserMFATable(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS user_mfa (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL UNIQUE,
			mfa_secret VARCHAR(255) NOT NULL,
			mfa_enabled BOOLEAN DEFAULT FALSE,
			mfa_type VARCHAR(20) DEFAULT 'totp',
			backup_codes TEXT,
			last_used_at TIMESTAMP,
			last_used_code VARCHAR(20),
			enabled_at TIMESTAMP,
			disabled_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_user_mfa_user_id ON user_mfa(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_mfa_enabled ON user_mfa(mfa_enabled)`,
	}
	return execAll(ctx, m.db, stmts)
}

// createLoginEventsTable 创建 login_events 表
func (m *AuthSecurityMigration) createLoginEventsTable(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS login_events (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL DEFAULT 0,
			username VARCHAR(50),
			ip VARCHAR(50) NOT NULL,
			user_agent VARCHAR(512),
			device_fingerprint VARCHAR(128),
			login_at TIMESTAMP NOT NULL,
			success BOOLEAN DEFAULT FALSE,
			risk_level VARCHAR(20) DEFAULT 'low',
			location VARCHAR(255),
			reason VARCHAR(255),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_user_id ON login_events(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_username ON login_events(username)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_ip ON login_events(ip)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_login_at ON login_events(login_at)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_risk_level ON login_events(risk_level)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_device_fingerprint ON login_events(device_fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_login_events_success ON login_events(success)`,
	}
	return execAll(ctx, m.db, stmts)
}

// createSecurityAlertsTable 创建 security_alerts 表
func (m *AuthSecurityMigration) createSecurityAlertsTable(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS security_alerts (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT DEFAULT 0,
			username VARCHAR(50),
			alert_type VARCHAR(50) NOT NULL,
			risk_level VARCHAR(20) NOT NULL,
			title VARCHAR(255) NOT NULL,
			description TEXT,
			ip VARCHAR(50),
			location VARCHAR(255),
			login_event_id BIGINT DEFAULT 0,
			notified BOOLEAN DEFAULT FALSE,
			status VARCHAR(20) DEFAULT 'open',
			resolved_at TIMESTAMP,
			resolved_by BIGINT DEFAULT 0,
			resolve_note TEXT,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_user_id ON security_alerts(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_username ON security_alerts(username)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_alert_type ON security_alerts(alert_type)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_risk_level ON security_alerts(risk_level)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_status ON security_alerts(status)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_ip ON security_alerts(ip)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_login_event_id ON security_alerts(login_event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_security_alerts_created_at ON security_alerts(created_at)`,
	}
	return execAll(ctx, m.db, stmts)
}

// createPasswordHistoryTable 创建 password_history 表
func (m *AuthSecurityMigration) createPasswordHistoryTable(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS password_history (
			id BIGSERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			changed_at TIMESTAMP NOT NULL,
			source VARCHAR(50) DEFAULT 'change_password',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_password_history_user_id ON password_history(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_password_history_changed_at ON password_history(changed_at)`,
		`CREATE INDEX IF NOT EXISTS idx_password_history_source ON password_history(source)`,
	}
	return execAll(ctx, m.db, stmts)
}

// createSystemConfigKVTable 创建 system_config_kv 表
// 用于存储密码策略等键值对配置（独立于 SystemConfig 模型，避免 schema 冲突）
func (m *AuthSecurityMigration) createSystemConfigKVTable(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS system_config_kv (
			key VARCHAR(100) PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	return execAll(ctx, m.db, stmts)
}

// Down 执行降级（不删除表，避免误删数据）
// 如需回滚，请手动执行 DROP TABLE
func (m *AuthSecurityMigration) Down(ctx context.Context) error {
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*AuthSecurityMigration)(nil)
