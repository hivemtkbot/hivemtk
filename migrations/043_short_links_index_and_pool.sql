-- =============================================================================
-- Migration 043: short_links 索引 + ConnMaxIdleTime 配置（OPT-DB-09 / OPT-DB-11）
-- 2026-08-16
--
-- 背景：
--   1. short_links 仅有 short_code UNIQUE，缺 created_at 索引 → 按时间范围查询会全表扫描
--   2. ConnMaxIdleTime 在 db.go 调用但 yaml 未显式配置 → 0 值 → 永不回收连接
-- =============================================================================

BEGIN;

-- 1. short_links 索引（OPT-DB-09）
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_short_links_created_at'
  ) THEN
    CREATE INDEX idx_short_links_created_at ON short_links (created_at DESC);
    RAISE NOTICE 'Created idx_short_links_created_at';
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_short_links_status_created_at'
  ) THEN
    CREATE INDEX idx_short_links_status_created_at ON short_links (status, created_at DESC);
    RAISE NOTICE 'Created idx_short_links_status_created_at';
  END IF;
END $$;

-- 2. short_link_access 索引
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_short_link_access_accessed_at'
  ) THEN
    CREATE INDEX idx_short_link_access_accessed_at ON short_link_access (accessed_at DESC);
    RAISE NOTICE 'Created idx_short_link_access_accessed_at';
  END IF;
END $$;

-- 3. knowledge_documents 索引（已有 product_id 索引但缺 status）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'knowledge_documents' AND column_name = 'status')
    AND NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_knowledge_documents_status') THEN
    CREATE INDEX idx_knowledge_documents_status ON knowledge_documents (status, created_at DESC);
    RAISE NOTICE 'Created idx_knowledge_documents_status';
  END IF;
END $$;

-- 4. operation_logs 索引（按 module/action 过滤优化）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'operation_logs' AND column_name = 'module') THEN
    CREATE INDEX IF NOT EXISTS idx_operation_logs_module_created ON operation_logs (module, created_at DESC);
  END IF;
END $$;

COMMIT;

-- OPT-DB-11：ConnMaxIdleTime 显式配置（推荐 5 分钟）
-- 配置文件示例（hivemtk/user-server/config.yaml）：
--   database:
--     pool:
--       max_idle_conns: 50
--       max_open_conns: 200
--       conn_max_idle_time: 300  # 5 分钟（OPT-DB-11 推荐值）
--       conn_max_lifetime: 3600  # 1 小时
