-- ============================================================================
-- RAG 系统增强迁移 (RAG Enhancement Migration)
-- 对齐 RAG_AUTO_REPLY_UNIFIED_ARCHITECTURE.md 权威基线
-- 创建时间: 2026-07-15
-- 目的: 补齐 RAG 系统 UI 可视化导入、OpenAPI 集成、文档统计三大缺口
-- ============================================================================

-- ============================================================================
-- 1. rag_products 字段补全 (补齐缺失字段)
-- ============================================================================

-- pgvector 集合名
ALTER TABLE rag_products ADD COLUMN IF NOT EXISTS vector_table VARCHAR(128);
-- 为已存在的产品生成 vector_table (格式: rag_product_<id>)
UPDATE rag_products
   SET vector_table = 'rag_product_' || id
 WHERE vector_table IS NULL;
ALTER TABLE rag_products ALTER COLUMN vector_table SET NOT NULL;
