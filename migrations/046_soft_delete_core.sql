-- =============================================================================
-- Migration 046: 软删除扩到核心业务表（OPT-DB-07）
-- 2026-08-16
--
-- 目标：为 customers / orders / sales_logs / leads / opportunities
--       添加 deleted_at 字段并创建 GORM 软删除所需的部分索引
--
-- 设计原则：
--   1. deleted_at 字段类型为 TIMESTAMPTZ，NULL 表示"未删除"
--   2. 每个表都加 idx_<table>_deleted_at 部分索引（仅当删除时才有数据）
--   3. 仅添加字段与索引，**不**创建触发器或修改业务逻辑
--   4. 应用层（model/...）同步添加 gorm.DeletedAt 字段（参考 047 提交）
--
-- 兼容性：
--   - PostgreSQL 11+ / 12+ / 13+ 通用
--   - 若已存在 deleted_at 字段则 IF NOT EXISTS 跳过（幂等）
--   - 索引使用 CREATE INDEX IF NOT EXISTS（幂等）
-- =============================================================================

BEGIN;

-- 1) customers
ALTER TABLE IF EXISTS customers
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname = current_schema() AND indexname = 'idx_customers_deleted_at'
  ) THEN
    CREATE INDEX idx_customers_deleted_at ON customers (deleted_at) WHERE deleted_at IS NOT NULL;
  END IF;
END $$;

-- 2) orders（订单表）
ALTER TABLE IF EXISTS orders
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE schemaname = current_schema() AND indexname = 'idx_orders_deleted_at'
  ) THEN
    CREATE INDEX idx_orders_deleted_at ON orders (deleted_at) WHERE deleted_at IS NOT NULL;
  END IF;
END $$;

-- 3) sales_logs（销售跟进日志）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'sales_logs') THEN
    ALTER TABLE sales_logs ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_sales_logs_deleted_at') THEN
      CREATE INDEX idx_sales_logs_deleted_at ON sales_logs (deleted_at) WHERE deleted_at IS NOT NULL;
    END IF;
    RAISE NOTICE 'Added deleted_at to sales_logs';
  ELSE
    RAISE NOTICE 'sales_logs table not found, skipped';
  END IF;
END $$;

-- 4) leads（线索，等同于 clues 表）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'leads') THEN
    ALTER TABLE leads ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_leads_deleted_at') THEN
      CREATE INDEX idx_leads_deleted_at ON leads (deleted_at) WHERE deleted_at IS NOT NULL;
    END IF;
    RAISE NOTICE 'Added deleted_at to leads';
  ELSE
    RAISE NOTICE 'leads table not found, will be handled by clues (alias)';
  END IF;
END $$;

-- 同时为 clues 兼容表也加（v1 命名是 clues）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'clues') THEN
    ALTER TABLE clues ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_clues_deleted_at') THEN
      CREATE INDEX idx_clues_deleted_at ON clues (deleted_at) WHERE deleted_at IS NOT NULL;
    END IF;
    RAISE NOTICE 'Added deleted_at to clues (alias for leads)';
  END IF;
END $$;

-- 5) opportunities（商机表）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'opportunities') THEN
    ALTER TABLE opportunities ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;
    IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_opportunities_deleted_at') THEN
      CREATE INDEX idx_opportunities_deleted_at ON opportunities (deleted_at) WHERE deleted_at IS NOT NULL;
    END IF;
    RAISE NOTICE 'Added deleted_at to opportunities';
  ELSE
    RAISE NOTICE 'opportunities table not found, skipped (will create in 050)';
  END IF;
END $$;

COMMIT;

-- =============================================================================
-- 验证语句（手工执行，CI 不跑）：
--   SELECT table_name, column_name FROM information_schema.columns
--    WHERE column_name = 'deleted_at' AND table_name IN
--    ('customers','orders','sales_logs','leads','clues','opportunities')
--    ORDER BY table_name;
-- =============================================================================
