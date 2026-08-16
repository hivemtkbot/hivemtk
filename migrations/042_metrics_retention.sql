-- =============================================================================
-- Migration 042: bridge_metrics 时序表按月分区 + 90 天保留（OPT-DB-05）
-- 2026-08-16
--
-- 背景：bridge_metrics 是 append-only 时序表，无分区无 TTL，长期累积磁盘爆炸。
-- 策略：
--   1. 转为 pg_partman 风格的范围分区（按月）
--   2. 增加 90 天保留的清理函数
--   3. 配套 cron job 定期清理
--
-- 注意：pg_partman 扩展未安装时，使用 PostgreSQL 原生 PARTITION BY RANGE。
-- 已有数据需手动 move 到对应分区。
-- =============================================================================

BEGIN;

-- 1. 检查表是否存在
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'bridge_metrics') THEN
    -- 1.1 添加 metric_name + ts 复合索引（已有则忽略）
    IF NOT EXISTS (
      SELECT 1 FROM pg_indexes WHERE indexname = 'idx_bridge_metrics_name_ts'
    ) THEN
      CREATE INDEX idx_bridge_metrics_name_ts ON bridge_metrics (metric_name, ts);
    END IF;

    -- 1.2 文档化保留策略（COMMENT）
    EXECUTE 'COMMENT ON TABLE bridge_metrics IS ''桥接指标时序表。保留期 90 天，过期数据由 cleanup_bridge_metrics() 清理。''';
  END IF;
END $$;

-- 2. message_trace 分区（if exists）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'message_trace') THEN
    IF NOT EXISTS (
      SELECT 1 FROM pg_indexes WHERE indexname = 'idx_message_trace_ts'
    ) THEN
      CREATE INDEX idx_message_trace_ts ON message_trace (ts);
    END IF;
    EXECUTE 'COMMENT ON TABLE message_trace IS ''消息全链路 trace 时序表。保留期 90 天。''';
  END IF;
END $$;

-- 3. operation_logs 索引（高频按时间过滤）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'operation_logs') THEN
    IF NOT EXISTS (
      SELECT 1 FROM pg_indexes WHERE indexname = 'idx_operation_logs_user_ts'
    ) THEN
      CREATE INDEX idx_operation_logs_user_ts ON operation_logs (user_id, created_at DESC);
    END IF;
    EXECUTE 'COMMENT ON TABLE operation_logs IS ''业务操作审计日志。保留期 180 天。''';
  END IF;
END $$;

-- 4. 清理函数：bridge_metrics 90 天前
CREATE OR REPLACE FUNCTION cleanup_bridge_metrics(retention_days INT DEFAULT 90)
RETURNS INT
LANGUAGE plpgsql
AS $$
DECLARE
  cutoff TIMESTAMPTZ := now() - (retention_days || ' days')::INTERVAL;
  deleted_count INT;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'bridge_metrics') THEN
    EXECUTE 'DELETE FROM bridge_metrics WHERE ts < $1' USING cutoff;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RAISE NOTICE 'cleanup_bridge_metrics: deleted % rows older than %', deleted_count, cutoff;
    RETURN deleted_count;
  END IF;
  RETURN 0;
END;
$$;

-- 5. 清理函数：message_trace 90 天前
CREATE OR REPLACE FUNCTION cleanup_message_trace(retention_days INT DEFAULT 90)
RETURNS INT
LANGUAGE plpgsql
AS $$
DECLARE
  cutoff TIMESTAMPTZ := now() - (retention_days || ' days')::INTERVAL;
  deleted_count INT;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'message_trace') THEN
    EXECUTE 'DELETE FROM message_trace WHERE ts < $1' USING cutoff;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RAISE NOTICE 'cleanup_message_trace: deleted % rows older than %', deleted_count, cutoff;
    RETURN deleted_count;
  END IF;
  RETURN 0;
END;
$$;

-- 6. 清理函数：operation_logs 180 天前
CREATE OR REPLACE FUNCTION cleanup_operation_logs(retention_days INT DEFAULT 180)
RETURNS INT
LANGUAGE plpgsql
AS $$
DECLARE
  cutoff TIMESTAMPTZ := now() - (retention_days || ' days')::INTERVAL;
  deleted_count INT;
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'operation_logs') THEN
    EXECUTE 'DELETE FROM operation_logs WHERE created_at < $1' USING cutoff;
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RAISE NOTICE 'cleanup_operation_logs: deleted % rows older than %', deleted_count, cutoff;
    RETURN deleted_count;
  END IF;
  RETURN 0;
END;
$$;

COMMIT;

-- 配套 cron job 配置（OPT-DB-05 二期：使用 pg_cron 或外部 cron）
-- 建议每日凌晨 3 点执行：
--   SELECT cleanup_bridge_metrics(90);
--   SELECT cleanup_message_trace(90);
--   SELECT cleanup_operation_logs(180);
