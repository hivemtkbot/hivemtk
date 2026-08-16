-- =============================================================================
-- Migration 045: 收敛 customer_id 字段类型（OPT-DB-04）
-- 2026-08-16
--
-- 背景：审计发现 19 域 customer_id 字段类型不统一：
--   - 部分表用 BIGINT（customer.id 主键风格）
--   - 部分表用 VARCHAR(64)（OneID 字符串风格）
--   - 部分表用 VARCHAR(50)（兼容旧数据）
--
-- 策略：
--   1. 统一对外（API/Service 层）使用 VARCHAR(64) = OneID
--   2. 内部关联使用 BIGINT（customer.id）性能更优
--   3. 同时存在 OneID + CustomerID 两列的表，统一规范
--
-- 涉及表（按出现频率）：
--   - dialogue_memories（已用 BIGINT）
--   - sop_executions（VARCHAR 50）
--   - sales_intent_scores（VARCHAR 64）
--   - ai_sales_logs（VARCHAR 64）
--   - inbox_conversations（VARCHAR 64）
--   - inbox_assignments（VARCHAR 64）
--   - ab_conversion_events（VARCHAR 64）
--   - 等等 ~10 个表
--
-- 方案：分阶段统一（本期统一为 VARCHAR(64)，下期考虑迁 BIGINT）
-- =============================================================================

BEGIN;

-- 1. customer_id 字段从 VARCHAR(50) → VARCHAR(64)
DO $$
DECLARE
  tab RECORD;
  cols_to_resize TEXT[] := ARRAY['customer_id'];
  col_name TEXT;
BEGIN
  FOR tab IN
    SELECT c.table_name
    FROM information_schema.columns c
    WHERE c.table_schema = 'public'
      AND c.column_name = 'customer_id'
      AND c.character_maximum_length < 64
      AND c.data_type = 'character varying'
  LOOP
    EXECUTE format('ALTER TABLE %I ALTER COLUMN customer_id TYPE VARCHAR(64)', tab.table_name);
    RAISE NOTICE 'Resized customer_id in % to VARCHAR(64)', tab.table_name;
  END LOOP;
END $$;

-- 2. 添加缺失的索引（如果 customer_id 上没索引）
DO $$
DECLARE
  tab RECORD;
BEGIN
  FOR tab IN
    SELECT t.table_name
    FROM information_schema.tables t
    WHERE t.table_schema = 'public'
      AND EXISTS (
        SELECT 1 FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.table_name = t.table_name
          AND c.column_name = 'customer_id'
      )
      AND NOT EXISTS (
        SELECT 1 FROM pg_indexes i
        WHERE i.schemaname = 'public'
          AND i.tablename = t.table_name
          AND i.indexdef LIKE '%customer_id%'
      )
  LOOP
    EXECUTE format('CREATE INDEX IF NOT EXISTS idx_%I_customer_id ON %I (customer_id)', tab.table_name, tab.table_name);
    RAISE NOTICE 'Created idx_%_customer_id', tab.table_name;
  END LOOP;
END $$;

-- 3. 复合索引：customer_id + created_at DESC（覆盖 RFM / 旅程 / 漏斗查询）
DO $$
DECLARE
  tab RECORD;
  idx_name TEXT;
BEGIN
  FOR tab IN
    SELECT t.table_name
    FROM information_schema.tables t
    WHERE t.table_schema = 'public'
      AND EXISTS (
        SELECT 1 FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.table_name = t.table_name
          AND c.column_name = 'customer_id'
      )
      AND EXISTS (
        SELECT 1 FROM information_schema.columns c
        WHERE c.table_schema = 'public'
          AND c.table_name = t.table_name
          AND c.column_name = 'created_at'
      )
      AND NOT EXISTS (
        SELECT 1 FROM pg_indexes i
        WHERE i.schemaname = 'public'
          AND i.tablename = t.table_name
          AND i.indexdef LIKE '%customer_id%'
          AND i.indexdef LIKE '%created_at%'
      )
  LOOP
    idx_name := 'idx_' || tab.table_name || '_customer_created';
    EXECUTE format('CREATE INDEX IF NOT EXISTS %I ON %I (customer_id, created_at DESC)', idx_name, tab.table_name);
    RAISE NOTICE 'Created %', idx_name;
  END LOOP;
END $$;

COMMIT;

-- 应用层建议（golang/models）：
//   1. customer_id 字段统一用 string 类型（GORM tag: type:varchar(64); index; comment:客户OneID或主键）
//   2. 与 customer.id (BIGINT) 关联时使用 ConvertCustomerIDToInt64() 工具函数
//   3. API 返回时统一转字符串（避免 JS 精度丢失）
