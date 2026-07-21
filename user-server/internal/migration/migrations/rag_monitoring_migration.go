package migrations

// rag_monitoring_migration.go RAG 监控与风控迁移 v2.8.0
//
// 五层架构归属: L5 数据层
// 设计依据: docs/核心链路优化.md 第十四章 §14.6 召回率监控 / §14.6.3 风控预警 / §14.6.4 健康度
// 私域独立部署: 无 merchant_id 字段
//
// 本迁移补充 C 域 P1 缺口 #2/#3/#4 所需的数据库基础设施：
//  1. rag_query_logs 表（每次检索的明细日志，用于召回率/P99/低召回样本分析）
//  2. rag_metrics_daily 表（5 分钟窗口聚合的指标快照，用于趋势图和预警）
//  3. rag_alerts 表（风控预警记录，按类型/严重度/状态管理）
//
// 幂等性: 所有 DDL 使用 IF NOT EXISTS，可重入
// 依赖: v1.1.0（InitialSchemaMigration）

import (
	"context"
	"fmt"

	"marketing/internal/migration"
	"marketing/internal/pkg/utils/logger"

	"gorm.io/gorm"
)

// RagMonitoringMigration RAG 监控与风控迁移 v2.8.0
type RagMonitoringMigration struct {
	db *gorm.DB
}

// NewRagMonitoringMigration 创建迁移实例
func NewRagMonitoringMigration(db *gorm.DB) *RagMonitoringMigration {
	return &RagMonitoringMigration{db: db}
}

// Version 返回版本号
func (m *RagMonitoringMigration) Version() string { return "v2.8.0" }

// Name 返回迁移名称
func (m *RagMonitoringMigration) Name() string {
	return "RAG 监控与风控（召回率日志/聚合/预警）"
}

// Description 返回迁移描述
func (m *RagMonitoringMigration) Description() string {
	return "新建 rag_query_logs / rag_metrics_daily / rag_alerts 表，支撑召回率监控、5 分钟聚合、风控预警与健康度评分"
}

// Up 执行升级
func (m *RagMonitoringMigration) Up(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("db is nil")
	}

	if err := m.createRagQueryLogs(ctx); err != nil {
		return fmt.Errorf("create rag_query_logs 失败: %w", err)
	}
	if err := m.createRagMetricsDaily(ctx); err != nil {
		return fmt.Errorf("create rag_metrics_daily 失败: %w", err)
	}
	if err := m.createRagAlerts(ctx); err != nil {
		return fmt.Errorf("create rag_alerts 失败: %w", err)
	}
	return nil
}

// createRagQueryLogs 创建 rag_query_logs 表
//
// 用途：每次检索的明细日志（query + retrieved + relevant + precision/recall/latency）
// 写入时机：检索完成后异步写入（RagMetricsService.RecordQuery）
// 查询场景：
//   - GetRecallMetrics 聚合时间窗口指标
//   - GetLowRecallQueries 查询低召回样本（调优）
//   - P99 延迟计算（偏移法）
func (m *RagMonitoringMigration) createRagQueryLogs(ctx context.Context) error {
	stmt := `
		CREATE TABLE IF NOT EXISTS rag_query_logs (
			id                BIGSERIAL    PRIMARY KEY,
			query             TEXT         NOT NULL,
			query_hash        VARCHAR(64),
			session_id        VARCHAR(128),
			product_id        BIGINT       DEFAULT 0,
			retrieved_doc_ids TEXT,
			relevant_doc_ids  TEXT,
			retrieved_count   INT          DEFAULT 0,
			relevant_count    INT          DEFAULT 0,
			hit_count         INT          DEFAULT 0,
			precision         DECIMAL(6,4) DEFAULT 0,
			recall            DECIMAL(6,4) DEFAULT 0,
			latency_ms        BIGINT       DEFAULT 0,
			top_k             INT          DEFAULT 5,
			source            VARCHAR(32)  DEFAULT 'hybrid',
			created_at        TIMESTAMP    NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_rag_query_logs_created ON rag_query_logs(created_at);
		CREATE INDEX IF NOT EXISTS idx_rag_query_logs_session ON rag_query_logs(session_id);
		CREATE INDEX IF NOT EXISTS idx_rag_query_logs_product ON rag_query_logs(product_id);
		CREATE INDEX IF NOT EXISTS idx_rag_query_logs_hash ON rag_query_logs(query_hash);
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// createRagMetricsDaily 创建 rag_metrics_daily 表
//
// 用途：每 5 分钟聚合 rag_query_logs 写入的指标快照
// 写入时机：RagMetricsCron 每 5 分钟调用 AggregateLastWindow
// 查询场景：
//   - GetLatestMetrics 趋势图
//   - 健康度评分（最近窗口的 recall/precision/p99）
//   - 风控预警判断（recall < 0.3 等）
func (m *RagMonitoringMigration) createRagMetricsDaily(ctx context.Context) error {
	stmt := `
		CREATE TABLE IF NOT EXISTS rag_metrics_daily (
			id               BIGSERIAL     PRIMARY KEY,
			window_start     TIMESTAMP     NOT NULL,
			window_end       TIMESTAMP     NOT NULL,
			total_queries    BIGINT        DEFAULT 0,
			avg_recall       DECIMAL(6,4)  DEFAULT 0,
			avg_precision    DECIMAL(6,4)  DEFAULT 0,
			avg_latency_ms   DECIMAL(10,2) DEFAULT 0,
			p99_latency_ms   BIGINT        DEFAULT 0,
			zero_hit_count   BIGINT        DEFAULT 0,
			low_recall_count BIGINT        DEFAULT 0,
			created_at       TIMESTAMP     NOT NULL DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_rag_metrics_daily_window ON rag_metrics_daily(window_start);
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// createRagAlerts 创建 rag_alerts 表
//
// 用途：风控预警记录（按类型/严重度/状态管理）
// 写入时机：RagAlertService.CheckAndAlert 检测到指标超阈值时创建
// 4 种预警类型：low_recall / embedding_failure / high_latency / zero_hit
// 3 种严重度：message / warning / critical
func (m *RagMonitoringMigration) createRagAlerts(ctx context.Context) error {
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
		CREATE INDEX IF NOT EXISTS idx_rag_alerts_type ON rag_alerts(alert_type, created_at);
		CREATE INDEX IF NOT EXISTS idx_rag_alerts_severity ON rag_alerts(severity);
		CREATE INDEX IF NOT EXISTS idx_rag_alerts_resolved ON rag_alerts(resolved);
		CREATE INDEX IF NOT EXISTS idx_rag_alerts_created ON rag_alerts(created_at);
	`
	return m.db.WithContext(ctx).Exec(stmt).Error
}

// Down 执行降级
//
// 注意：删除监控表会丢失历史数据，但业务核心数据（knowledge_documents 等）不受影响
func (m *RagMonitoringMigration) Down(ctx context.Context) error {
	tables := []string{"rag_alerts", "rag_metrics_daily", "rag_query_logs"}
	for _, tbl := range tables {
		if err := m.db.WithContext(ctx).Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", tbl)).Error; err != nil {
			logger.Infof("[RagMonitoringMigration] drop %s 提示: %v", tbl, err)
		}
	}
	return nil
}

// compile-time 接口断言
var _ migration.Migration = (*RagMonitoringMigration)(nil)
