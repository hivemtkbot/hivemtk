package migrations

// compliance_d_e_migration.go D+E 域合规迁移 v2.10.0
//
// 五层架构归属: L5 数据层
// 设计依据:
//   - 《互联网电子邮件服务管理办法》第十三条（邮件退订 + 追踪）
//   - 《通信短消息服务管理规定》第十八条（短信退订 + 追踪）
// 私域独立部署: 无 merchant_id 字段
//
// 本迁移创建 D+E 域（邮件+短信）合规所需的 6 张新表：
//  1. email_unsubscribes    - 邮件退订名单（email 唯一）
//  2. email_tracking_events - 邮件打开/点击/退订/退信事件追踪
//  3. email_job_metrics     - 邮件任务聚合指标（每 job_id 一条）
//  4. sms_unsubscribes      - 短信退订名单（phone 唯一）
//  5. sms_delivery_statuses - 短信送达状态记录（每 message_id 一条）
//  6. sms_job_metrics       - 短信任务聚合指标（每 job_id 一条）
//
// 幂等性: 所有 DDL 使用 IF NOT EXISTS，可重入
// 依赖: 无（独立表）

import (
	"context"
	"fmt"

	"hivemtk-user/internal/migration"

	"gorm.io/gorm"
)

// ComplianceDEMigration D+E 域合规迁移 v2.10.0
type ComplianceDEMigration struct {
	db *gorm.DB
}

// NewComplianceDEMigration 创建迁移实例
func NewComplianceDEMigration(db *gorm.DB) *ComplianceDEMigration {
	return &ComplianceDEMigration{db: db}
}

// Version 返回版本号
func (m *ComplianceDEMigration) Version() string { return "v2.10.0" }

// Name 返回迁移名称
func (m *ComplianceDEMigration) Name() string {
	return "D+E 域合规（邮件+短信退订与追踪 6 张表）"
}

// Description 返回迁移描述
func (m *ComplianceDEMigration) Description() string {
	return "创建 email_unsubscribes / email_tracking_events / email_job_metrics / sms_unsubscribes / sms_delivery_statuses / sms_job_metrics 6 张表"
}

// Up 执行升级
func (m *ComplianceDEMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	// 1. email_unsubscribes 表（邮件退订名单）
	if err := m.createEmailUnsubscribes(ctx); err != nil {
		return fmt.Errorf("create email_unsubscribes 失败: %w", err)
	}

	// 2. email_tracking_events 表（邮件追踪事件）
	if err := m.createEmailTrackingEvents(ctx); err != nil {
		return fmt.Errorf("create email_tracking_events 失败: %w", err)
	}

	// 3. email_job_metrics 表（邮件任务指标）
	if err := m.createEmailJobMetrics(ctx); err != nil {
		return fmt.Errorf("create email_job_metrics 失败: %w", err)
	}

	// 4. sms_unsubscribes 表（短信退订名单）
	if err := m.createSmsUnsubscribes(ctx); err != nil {
		return fmt.Errorf("create sms_unsubscribes 失败: %w", err)
	}

	// 5. sms_delivery_statuses 表（短信送达状态）
	if err := m.createSmsDeliveryStatuses(ctx); err != nil {
		return fmt.Errorf("create sms_delivery_statuses 失败: %w", err)
	}

	// 6. sms_job_metrics 表（短信任务指标）
	if err := m.createSmsJobMetrics(ctx); err != nil {
		return fmt.Errorf("create sms_job_metrics 失败: %w", err)
	}

	return nil
}

// createEmailUnsubscribes 创建 email_unsubscribes 表
//
// 合规依据：《互联网电子邮件服务管理办法》第十三条
// email 唯一索引：保证同一邮箱仅一条退订记录；重新订阅时 DELETE 该行
func (m *ComplianceDEMigration) createEmailUnsubscribes(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS email_unsubscribes (
			id BIGSERIAL PRIMARY KEY,
			email VARCHAR(255) NOT NULL UNIQUE,
			reason VARCHAR(255) DEFAULT '',
			unsubscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_link VARCHAR(512) DEFAULT '',
			ip VARCHAR(64) DEFAULT '',
			ua VARCHAR(512) DEFAULT '',
			job_id VARCHAR(36) DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_unsubscribes_email ON email_unsubscribes(email)`,
		`CREATE INDEX IF NOT EXISTS idx_email_unsubscribes_job ON email_unsubscribes(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_unsubscribes_deleted ON email_unsubscribes(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// createEmailTrackingEvents 创建 email_tracking_events 表
//
// 每条事件记录邮件打开/点击/退订/退信行为
// event_id 唯一索引：保证 webhook 重放幂等
// (email + job_id) 联合索引：支持按任务和邮箱查询
func (m *ComplianceDEMigration) createEmailTrackingEvents(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS email_tracking_events (
			id BIGSERIAL PRIMARY KEY,
			event_id VARCHAR(36) NOT NULL UNIQUE,
			email VARCHAR(255) NOT NULL,
			job_id VARCHAR(36) DEFAULT '',
			event_type VARCHAR(20) NOT NULL,
			user_agent VARCHAR(512) DEFAULT '',
			ip VARCHAR(64) DEFAULT '',
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_event_id ON email_tracking_events(event_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_email ON email_tracking_events(email)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_job ON email_tracking_events(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_type ON email_tracking_events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_time ON email_tracking_events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tracking_deleted ON email_tracking_events(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// createEmailJobMetrics 创建 email_job_metrics 表
//
// 每 job_id 一条聚合指标
// open_rate  = total_opened / total_sent * 100
// click_rate = total_clicked / total_sent * 100
func (m *ComplianceDEMigration) createEmailJobMetrics(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS email_job_metrics (
			id BIGSERIAL PRIMARY KEY,
			job_id VARCHAR(36) NOT NULL UNIQUE,
			total_sent BIGINT NOT NULL DEFAULT 0,
			total_opened BIGINT NOT NULL DEFAULT 0,
			total_clicked BIGINT NOT NULL DEFAULT 0,
			total_bounced BIGINT NOT NULL DEFAULT 0,
			total_unsubscribed BIGINT NOT NULL DEFAULT 0,
			open_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
			click_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_job_metrics_job ON email_job_metrics(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_job_metrics_updated ON email_job_metrics(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_email_job_metrics_deleted ON email_job_metrics(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// createSmsUnsubscribes 创建 sms_unsubscribes 表
//
// 合规依据：《通信短消息服务管理规定》第十八条
// phone 唯一索引：保证同一手机号仅一条退订记录
func (m *ComplianceDEMigration) createSmsUnsubscribes(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sms_unsubscribes (
			id BIGSERIAL PRIMARY KEY,
			phone VARCHAR(20) NOT NULL UNIQUE,
			reason VARCHAR(255) DEFAULT '',
			unsubscribed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			source_message_id VARCHAR(64) DEFAULT '',
			keyword_matched VARCHAR(20) DEFAULT '',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_unsubscribes_phone ON sms_unsubscribes(phone)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_unsubscribes_keyword ON sms_unsubscribes(keyword_matched)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_unsubscribes_deleted ON sms_unsubscribes(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// createSmsDeliveryStatuses 创建 sms_delivery_statuses 表
//
// 每条记录对应一次短信发送的送达状态
// message_id 唯一索引：webhook 多次推送同一消息时更新而非新建
// is_retryable + retry_count + max_retry + status 联合查询：定时重试任务使用
func (m *ComplianceDEMigration) createSmsDeliveryStatuses(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sms_delivery_statuses (
			id BIGSERIAL PRIMARY KEY,
			message_id VARCHAR(64) NOT NULL UNIQUE,
			phone VARCHAR(20) NOT NULL,
			job_id VARCHAR(36) DEFAULT '',
			provider VARCHAR(50) DEFAULT '',
			status VARCHAR(20) NOT NULL,
			error_code VARCHAR(20) DEFAULT '',
			error_msg TEXT DEFAULT '',
			is_retryable BOOLEAN NOT NULL DEFAULT FALSE,
			retry_count INT NOT NULL DEFAULT 0,
			max_retry INT NOT NULL DEFAULT 3,
			sent_at TIMESTAMPTZ,
			delivered_at TIMESTAMPTZ,
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_message ON sms_delivery_statuses(message_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_phone ON sms_delivery_statuses(phone)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_job ON sms_delivery_statuses(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_status ON sms_delivery_statuses(status)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_retry ON sms_delivery_statuses(is_retryable, retry_count, max_retry, status)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_received ON sms_delivery_statuses(received_at)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_delivery_deleted ON sms_delivery_statuses(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// createSmsJobMetrics 创建 sms_job_metrics 表
//
// 每 job_id 一条聚合指标
// delivery_rate = total_delivered / total_sent * 100
// failure_rate  = total_failed   / total_sent * 100
func (m *ComplianceDEMigration) createSmsJobMetrics(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sms_job_metrics (
			id BIGSERIAL PRIMARY KEY,
			job_id VARCHAR(36) NOT NULL UNIQUE,
			total_sent BIGINT NOT NULL DEFAULT 0,
			total_delivered BIGINT NOT NULL DEFAULT 0,
			total_failed BIGINT NOT NULL DEFAULT 0,
			total_retried BIGINT NOT NULL DEFAULT 0,
			delivery_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
			failure_rate NUMERIC(5,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_job_metrics_job ON sms_job_metrics(job_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_job_metrics_updated ON sms_job_metrics(updated_at)`,
		`CREATE INDEX IF NOT EXISTS idx_sms_job_metrics_deleted ON sms_job_metrics(deleted_at)`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// Down 执行降级（删除 6 张表）
func (m *ComplianceDEMigration) Down(ctx context.Context) error {
	stmts := []string{
		`DROP TABLE IF EXISTS sms_job_metrics`,
		`DROP TABLE IF EXISTS sms_delivery_statuses`,
		`DROP TABLE IF EXISTS sms_unsubscribes`,
		`DROP TABLE IF EXISTS email_job_metrics`,
		`DROP TABLE IF EXISTS email_tracking_events`,
		`DROP TABLE IF EXISTS email_unsubscribes`,
	}
	return execAllComplianceDE(ctx, m.db, stmts)
}

// execAllComplianceDE 批量执行 SQL（出错即返回）
func execAllComplianceDE(ctx context.Context, db *gorm.DB, stmts []string) error {
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
var _ migration.Migration = (*ComplianceDEMigration)(nil)
