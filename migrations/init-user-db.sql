-- ============================================================
-- 用户端 PostgreSQL 初始化脚本（独立数据库/独立容器）
-- ============================================================

-- 启用 pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ============================================================
-- 用户端数据库表结构 (user_db)
-- ============================================================

-- 知识库向量表
-- 2026-07-18 私域基线：默认 1024 维（本地 Xinference + bge-m3）
CREATE TABLE IF NOT EXISTS knowledge_embeddings (
    id SERIAL PRIMARY KEY,
    product_id VARCHAR(64) NOT NULL,
    chunk_id VARCHAR(64) NOT NULL,
    content TEXT,
    embedding vector(1024),
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_id, chunk_id)
);

-- 创建向量索引
CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_product_id ON knowledge_embeddings(product_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_embeddings_chunk_id ON knowledge_embeddings(chunk_id);

-- RAG 产品配置表
CREATE TABLE IF NOT EXISTS rag_products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    vector_table VARCHAR(128) UNIQUE,
    embedding_model VARCHAR(64) DEFAULT 'bge-m3',
    llm_model VARCHAR(64),
    temperature FLOAT DEFAULT 0.7,
    top_k INTEGER DEFAULT 5,
    system_prompt TEXT,
    status INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- RAG 产品配置索引
CREATE INDEX IF NOT EXISTS idx_rag_products_vector_table ON rag_products(vector_table);

-- 知识库文档表
CREATE TABLE IF NOT EXISTS knowledge_documents (
    id SERIAL PRIMARY KEY,
    product_id INTEGER REFERENCES rag_products(id),
    filename VARCHAR(256) NOT NULL,
    file_type VARCHAR(32),
    file_size INTEGER,
    content TEXT,
    status VARCHAR(32) DEFAULT 'pending',
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 知识库文档索引
CREATE INDEX IF NOT EXISTS idx_knowledge_documents_product ON knowledge_documents(product_id);

-- 脚本模板表
CREATE TABLE IF NOT EXISTS script_templates (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    category_id INTEGER,
    content TEXT,
    status INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 脚本分类表
CREATE TABLE IF NOT EXISTS script_categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 脚本推荐表
CREATE TABLE IF NOT EXISTS script_recommends (
    id SERIAL PRIMARY KEY,
    script_id INTEGER REFERENCES script_templates(id),
    sort_order INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 邮件 SMTP 配置表
CREATE TABLE IF NOT EXISTS email_smtps (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64),
    host VARCHAR(128),
    port INTEGER,
    username VARCHAR(128),
    password VARCHAR(256),
    from_address VARCHAR(128),
    from_name VARCHAR(64),
    status INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 邮件列表表
CREATE TABLE IF NOT EXISTS email_lists (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128),
    description TEXT,
    status INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 客户标签表
CREATE TABLE IF NOT EXISTS customer_tags (
    id SERIAL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    color VARCHAR(16),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- RAG 会话表
CREATE TABLE IF NOT EXISTS rag_sessions (
    id SERIAL PRIMARY KEY,
    product_id INTEGER REFERENCES rag_products(id),
    session_id VARCHAR(64) NOT NULL UNIQUE,
    status VARCHAR(32) DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- RAG 消息表
CREATE TABLE IF NOT EXISTS rag_messages (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL,
    role VARCHAR(32) NOT NULL,
    content TEXT,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- OBS 配置表
CREATE TABLE IF NOT EXISTS obs_configs (
    id SERIAL PRIMARY KEY,
    provider VARCHAR(32) NOT NULL,
    name VARCHAR(64),
    endpoint VARCHAR(256),
    bucket VARCHAR(128),
    access_key VARCHAR(128),
    secret_key VARCHAR(256),
    status INTEGER DEFAULT 1,
    is_default INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 短链接表
CREATE TABLE IF NOT EXISTS short_links (
    id SERIAL PRIMARY KEY,
    original_url TEXT NOT NULL,
    short_code VARCHAR(16) NOT NULL UNIQUE,
    click_count INTEGER DEFAULT 0,
    status INTEGER DEFAULT 1,
    expire_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 短链接访问记录表
CREATE TABLE IF NOT EXISTS short_link_accesses (
    id SERIAL PRIMARY KEY,
    short_link_id INTEGER REFERENCES short_links(id),
    ip_address VARCHAR(64),
    user_agent VARCHAR(256),
    referer VARCHAR(512),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- 抖音卡片活动表
CREATE TABLE IF NOT EXISTS douyin_card_activities (
    id SERIAL PRIMARY KEY,
    name VARCHAR(128),
    card_type VARCHAR(32),
    config JSONB,
    status INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
