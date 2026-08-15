-- ============================================================
-- 038_bridge_metrics.sql
-- 2026-08-15 桥接指标落库表（M3-P1-E4 指标采集）
--
-- 私域部署约定：无外部监控（不接 Prometheus / Grafana），指标通过
-- 「应用层日志 + 数据库表查询」两条通道使用。本表为第二条通道：
-- 应用层定时（每 60s）把注册表中各指标当前值追加写入，构成追加式时间序列，
-- 供 SQL 巡检按 (metric_name, ts) 范围查询趋势。
--
-- 与 Go migration `BridgeMetricsMigration`（v3.21.0）以及
-- internal/pkg/metrics/bridge_sink.go 的 BridgeMetricRow 严格对齐：
--   - labels:    TEXT（GORM string 字段，存储 label 键值对 JSON 字符串）
--   - value:     DOUBLE PRECISION（counter/gauge 原始值；histogram 为 count 或 sum）
--   - metric_type: counter | gauge | histogram
--   - agg:       直方图区分 count / sum（写入 labels JSON 的 agg 键）
-- ============================================================

CREATE TABLE IF NOT EXISTS bridge_metrics (
    id          BIGSERIAL PRIMARY KEY,
    metric_name VARCHAR(128)   NOT NULL,
    labels      TEXT           NOT NULL DEFAULT '{}',
    value       DOUBLE PRECISION NOT NULL DEFAULT 0,
    metric_type VARCHAR(32)    NOT NULL DEFAULT '',
    ts          TIMESTAMPTZ    NOT NULL DEFAULT now()
);

-- 巡检查询：按 (metric_name, ts) 范围取趋势；SQL 巡检首选该索引。
CREATE INDEX IF NOT EXISTS idx_bridge_metrics_metric_ts ON bridge_metrics (metric_name, ts);
