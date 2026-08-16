-- =============================================================================
-- Migration 050: intent_records 与 intent_logs 整合视图（OPT-DB-10 阶段一）
-- 2026-08-16
--
-- 背景：两表结构 95% 相同，区分写入方但读取时需要 UNION ALL
-- 策略：创建 intent_view 兼容层，应用层统一读取
--
-- 完整整合方案：[architecture/intent_unify_plan.md](../../docs/architecture/intent_unify_plan.md)
-- 阶段二（v3.20.0）：废弃 intent_logs，全部数据迁移到 intent_records
-- =============================================================================

BEGIN;

-- 1. 兼容性视图（仅当两表都存在时创建）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_records')
     AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_logs') THEN
    EXECUTE '
      CREATE OR REPLACE VIEW intent_view AS
        SELECT 
          ''records''::VARCHAR(20) AS source,
          id, session_id, customer_id,
          intent_type, intent_subtype, confidence, method, created_at
        FROM intent_records
      UNION ALL
        SELECT 
          ''logs''::VARCHAR(20) AS source,
          id, session_id, customer_id,
          intent_type, intent_subtype, confidence, method, created_at
        FROM intent_logs
    ';
    RAISE NOTICE 'Created intent_view compatibility view';
  ELSE
    RAISE NOTICE 'One or both intent tables missing, skipping view creation';
  END IF;
END $$;

-- 2. 创建统一索引（加速 intent_view 查询）
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.views WHERE table_name = 'intent_view') THEN
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_intent_records_session ON intent_records (session_id, created_at DESC)';
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_intent_records_customer_created ON intent_records (customer_id, created_at DESC)';
    
    -- 检查 intent_logs 是否存在并加索引
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'intent_logs') THEN
      EXECUTE 'CREATE INDEX IF NOT EXISTS idx_intent_logs_session ON intent_logs (session_id, created_at DESC)';
      EXECUTE 'CREATE INDEX IF NOT EXISTS idx_intent_logs_customer_created ON intent_logs (customer_id, created_at DESC)';
    END IF;
    
    RAISE NOTICE 'Created intent indexes';
  END IF;
END $$;

COMMIT;

-- 应用层使用：
// db.Table("intent_view").Where("customer_id = ?", cid).Order("created_at DESC").Limit(50).Find(&results)
// 替代之前的两表分别查询 + 内存合并
