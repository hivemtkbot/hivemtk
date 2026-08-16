-- USR-AI-03: knowledge_embeddings 表加 tsvector 列 + GIN 索引（Hybrid 检索用）
-- 借鉴：https://qdrant.tech/course/essentials/day-1/chunking-strategies/

-- 1. 加 tsvector 列
ALTER TABLE knowledge_embeddings
ADD COLUMN IF NOT EXISTS content_tsv tsvector;

-- 2. 创建 GIN 索引（加速全文检索）
CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_tsv
ON knowledge_embeddings USING GIN (content_tsv)
WHERE content_tsv IS NOT NULL;

-- 3. 触发器：插入/更新时自动更新 tsvector
CREATE OR REPLACE FUNCTION knowledge_embeddings_tsv_update() RETURNS trigger AS $$
BEGIN
  NEW.content_tsv := to_tsvector('simple', COALESCE(NEW.content, ''));
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_knowledge_embeddings_tsv ON knowledge_embeddings;
CREATE TRIGGER trg_knowledge_embeddings_tsv
BEFORE INSERT OR UPDATE OF content ON knowledge_embeddings
FOR EACH ROW EXECUTE FUNCTION knowledge_embeddings_tsv_update();

-- 4. 一次性回填已有数据
UPDATE knowledge_embeddings
SET content_tsv = to_tsvector('simple', COALESCE(content, ''))
WHERE content_tsv IS NULL;

COMMENT ON COLUMN knowledge_embeddings.content_tsv IS 'USR-AI-02/03: tsvector 用于 BM25/全文检索';
