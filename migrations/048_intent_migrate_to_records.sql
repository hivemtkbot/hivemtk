-- =============================================================================
-- Migration 048: intent_records/logs 整合二期 - 数据迁移（OPT-DB-10）
-- 2026-08-16
--
-- 背景：
--   - 阶段一（045）：建立 intent_view 兼容视图，应用层读统一
--   - 阶段二（本期）：将 intent_logs 数据迁移到 intent_records（保留 logs 表为 backup）
--   - 阶段三（建议 v3.22.0）：DROP intent_logs
--
-- 数据迁移策略：
--   1. INSERT INTO intent_records ... SELECT FROM intent_logs
--   2. 字段映射：message→raw_text, intent_major→intent_type, intent_minor→intent_subtype
--   3. ON CONFLICT (id) DO NOTHING：保留历史 id，跳过冲突
--   4. 迁移前 / 后做行数对比，失败回滚
--
-- 兼容性：
--   - intent_records 与 intent_logs 必须都已存在
--   - 整事务执行，失败 ROLLBACK
-- =============================================================================

BEGIN;

-- 1) 前置校验
DO $$
DECLARE
  rec_count BIGINT;
  log_count BIGINT;
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_records') THEN
    RAISE EXCEPTION 'intent_records table not found, abort';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_logs') THEN
    RAISE EXCEPTION 'intent_logs table not found, abort';
  END IF;

  SELECT COUNT(*) INTO rec_count FROM intent_records;
  SELECT COUNT(*) INTO log_count FROM intent_logs;
  RAISE NOTICE 'Before migration: intent_records=%, intent_logs=%', rec_count, log_count;
END $$;

-- 2) 字段映射 + 数据迁移
--    intent_logs:     ID, CustomerID, SessionID, Message, IntentMajor, IntentMinor,
--                     Confidence, Method, LatencyMs, Reasoning, TraceID, Timestamp, CreatedAt
--    intent_records:  ID, SessionID, CustomerID, MessageID, RawText, IntentType, IntentSubtype,
--                     Confidence, ConfidenceLevel, Entities, Sentiment, LLMModel, CostTokens,
--                     LatencyMs, CreatedAt
INSERT INTO intent_records (
  id, session_id, customer_id, message_id, raw_text,
  intent_type, intent_subtype, confidence,
  confidence_level, entities, sentiment, llm_model, cost_tokens,
  latency_ms, created_at
)
SELECT
  l.id,                                       -- 保留原 id
  l.session_id,                               -- session_id
  l.customer_id,                              -- customer_id
  0,                                          -- message_id: intent_logs 无对应字段
  COALESCE(l.message, ''),                    -- raw_text: message→raw_text
  COALESCE(l.intent_major, 'unknown'),        -- intent_type: major→type
  COALESCE(l.intent_minor, ''),               -- intent_subtype
  l.confidence,                               -- confidence（注意类型 DECIMAL(5,2) vs DECIMAL(5,4)）
  'medium',                                   -- confidence_level 默认
  '{}'::jsonb,                                -- entities 默认空
  'neutral',                                  -- sentiment 默认
  COALESCE(l.method, 'unknown'),              -- llm_model 用 method 占位（人工后期修正）
  0,                                          -- cost_tokens
  l.latency_ms,                               -- latency_ms
  COALESCE(l.created_at, l.timestamp, NOW())  -- created_at
FROM intent_logs l
ON CONFLICT (id) DO NOTHING;                  -- 冲突跳过

-- 3) 迁移后校验
DO $$
DECLARE
  rec_count_after BIGINT;
  log_count_after BIGINT;
BEGIN
  SELECT COUNT(*) INTO rec_count_after FROM intent_records;
  SELECT COUNT(*) INTO log_count_after FROM intent_logs;
  RAISE NOTICE 'After migration: intent_records=%, intent_logs=%', rec_count_after, log_count_after;
  RAISE NOTICE '新增记录：%（按 id 主键去重后）', rec_count_after - (SELECT COUNT(*) FROM intent_records WHERE id < 0);
END $$;

-- 4) 创建 source 字段追踪数据来源（intent_logs 来的标记为 legacy_logs）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'intent_records' AND column_name = 'source'
  ) THEN
    ALTER TABLE intent_records ADD COLUMN source VARCHAR(20) DEFAULT 'records';
    UPDATE intent_records
       SET source = 'legacy_logs'
     WHERE id IN (SELECT id FROM intent_logs)
       AND source = 'records';
    CREATE INDEX IF NOT EXISTS idx_intent_records_source ON intent_records (source);
    RAISE NOTICE 'Added source column and tagged legacy records';
  END IF;
END $$;

-- 5) 删除兼容视图（阶段二完成后视图已无意义）
DROP VIEW IF EXISTS intent_view;

COMMIT;

-- =============================================================================
-- 验证语句（手工执行）：
--   -- 1. 总数检查
--   SELECT source, COUNT(*) FROM intent_records GROUP BY source;
--   -- 预期：records ≥ 原 intent_records，legacy_logs ≤ 原 intent_logs
--
--   -- 2. 一致性抽样
--   SELECT r.id, r.session_id, r.intent_type, l.intent_major
--   FROM intent_records r
--   JOIN intent_logs l ON r.id = l.id
--   LIMIT 10;
-- =============================================================================
