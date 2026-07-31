-- 035_knowledge_base.sql
-- 知识库(KnowledgeBase) + 智能体知识库绑定(AgentKBBinding) 表结构
-- 设计依据: 2026-07-31 强 1对1 改造
--   - KnowledgeBase: 抽象的"知识库"，可挂载 FAQ / RAG / SOP 三类条目
--   - AgentKBBinding: 智能体 ↔ 知识库绑定 (一个智能体可绑多个知识库)
--   - channel_agent_bindings 加 UNIQUE 约束保证 (channel_type, account_id) 仅 1 条主绑定

-- ============================================================================
-- 1. 知识库主表
-- ============================================================================
CREATE TABLE IF NOT EXISTS knowledge_bases (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(128) NOT NULL,
    type VARCHAR(16) NOT NULL,                          -- faq / rag / sop
    owner_type VARCHAR(16) NOT NULL DEFAULT 'shared',   -- private / shared
    owner_agent_id BIGINT,                              -- owner_type=private 时必填
    description TEXT,
    is_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT chk_kb_type CHECK (type IN ('faq', 'rag', 'sop')),
    CONSTRAINT chk_kb_owner_type CHECK (owner_type IN ('private', 'shared'))
);

CREATE INDEX IF NOT EXISTS idx_kb_type ON knowledge_bases(type);
CREATE INDEX IF NOT EXISTS idx_kb_owner_type ON knowledge_bases(owner_type);
CREATE INDEX IF NOT EXISTS idx_kb_owner_agent ON knowledge_bases(owner_agent_id);
CREATE INDEX IF NOT EXISTS idx_kb_enabled ON knowledge_bases(is_enabled);

-- ============================================================================
-- 2. 智能体 ↔ 知识库 绑定表
-- ============================================================================
CREATE TABLE IF NOT EXISTS agent_kb_bindings (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT NOT NULL,
    knowledge_base_id BIGINT NOT NULL,
    priority INT DEFAULT 0,
    is_enabled BOOLEAN DEFAULT true,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    CONSTRAINT uq_agent_kb UNIQUE (agent_id, knowledge_base_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_kb_agent ON agent_kb_bindings(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_kb_kb ON agent_kb_bindings(knowledge_base_id);

-- ============================================================================
-- 3. channel_agent_bindings 强 1对1: 加 UNIQUE 约束
-- 保证同一 (channel_type, account_id) 仅 1 条 is_primary=true 记录
-- ============================================================================
-- 索引清理: 旧逻辑只按"is_primary=true + enabled=true"过滤, 重复数据可能存在
-- 先把已有数据中非最新 is_primary 标记的清掉, 仅保留最新 1 条
UPDATE channel_agent_bindings c
SET is_primary = false
WHERE c.is_primary = true
  AND c.id NOT IN (
    SELECT id FROM channel_agent_bindings c2
    WHERE c2.channel_type = c.channel_type
      AND c2.account_id = c.account_id
      AND c2.is_primary = true
    ORDER BY c2.id DESC
    LIMIT 1
  );

-- 加 UNIQUE 约束: (channel_type, account_id) + is_primary=true
-- PostgreSQL 不支持部分 UNIQUE INDEX 直接作为约束, 用唯一索引表达
CREATE UNIQUE INDEX IF NOT EXISTS uq_channel_account_primary
    ON channel_agent_bindings (channel_type, account_id)
    WHERE is_primary = true;
