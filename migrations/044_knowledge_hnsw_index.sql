-- =============================================================================
-- Migration 044: knowledge_embeddings HNSW 向量索引（OPT-DB-06）
-- 2026-08-16
--
-- 背景：knowledge_embeddings 是 1024 维 pgvector 表，10w+ chunks 后全表顺序扫描会变慢
-- 策略：HNSW 索引（Hierarchical Navigable Small World）
--   - 比 IVFFlat 召回率更高
--   - 建索引时间较长（10w+ chunks 约 1-2 分钟），但查询快
--   - m=16, ef_construction=64 是常用初始值
--
-- 注：
--   1. 已有数据时 CREATE INDEX CONCURRENTLY 避免锁表
--   2. 索引会占额外磁盘（约为原表大小 30-50%）
-- =============================================================================

BEGIN;

-- 1. 检查 pgvector 扩展
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
    RAISE EXCEPTION 'pgvector extension not installed. Run: CREATE EXTENSION vector;';
  END IF;
END $$;

-- 2. 检查 knowledge_embeddings 表
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'knowledge_embeddings') THEN
    RAISE NOTICE 'knowledge_embeddings table does not exist, skipping';
    RETURN;
  END IF;

  -- 3. 创建 HNSW 索引（向量余弦相似度）
  -- 使用 CONCURRENTLY 避免长时间锁表
  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes WHERE indexname = 'idx_knowledge_embeddings_hnsw'
  ) THEN
    EXECUTE 'CREATE INDEX CONCURRENTLY idx_knowledge_embeddings_hnsw
      ON knowledge_embeddings
      USING hnsw (embedding vector_cosine_ops)
      WITH (m = 16, ef_construction = 64)';
    RAISE NOTICE 'Created HNSW index on knowledge_embeddings';
  END IF;
END $$;

-- 4. 调整 HNSW 查询 ef_search 参数（影响召回率 vs 速度）
-- 建议值：ef_search=40（默认；高召回用 100+）
-- 此参数为 session 级，每次连接可单独设置
COMMENT ON INDEX idx_knowledge_embeddings_hnsw IS
  'HNSW 向量索引。session 级参数 SET hnsw.ef_search = 40 (默认) / 100 (高召回)';

COMMIT;

-- 应用层建议（golang/pgvector）：
// db.Exec("SET hnsw.ef_search = 40")  // 普通查询
// db.Exec("SET hnsw.ef_search = 100") // 高精度 RAG
