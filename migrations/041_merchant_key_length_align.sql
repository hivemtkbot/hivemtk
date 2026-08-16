-- =============================================================================
-- Migration 041: api_logs.merchant_key VARCHAR 长度对齐（OPT-DB-03）
-- 2026-08-16
--
-- 背景：merchants.merchant_key 是 VARCHAR(64)，api_logs.merchant_key 是 VARCHAR(32)。
--       跨端 join 存在截断隐患。
-- 策略：ALTER COLUMN 长度 32 → 64（PostgreSQL 不需要 rewrite，仅元数据变更）
-- =============================================================================

BEGIN;

-- 1. api_logs.merchant_key: 32 → 64
ALTER TABLE api_logs ALTER COLUMN merchant_key TYPE VARCHAR(64);

-- 2. audit_logs.merchant_key: 同样检查并对齐
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'audit_logs' AND column_name = 'merchant_key'
      AND character_maximum_length = 32
  ) THEN
    ALTER TABLE audit_logs ALTER COLUMN merchant_key TYPE VARCHAR(64);
    RAISE NOTICE 'audit_logs.merchant_key 32 → 64';
  END IF;
END $$;

-- 3. 平台端 api_logs.merchant_key（如果是独立平台 schema）
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'platform_api_logs' AND column_name = 'merchant_key'
      AND character_maximum_length = 32
  ) THEN
    ALTER TABLE platform_api_logs ALTER COLUMN merchant_key TYPE VARCHAR(64);
    RAISE NOTICE 'platform_api_logs.merchant_key 32 → 64';
  END IF;
END $$;

COMMIT;
