-- 032_ai_tool_config.sql
-- AI工具配置与账号绑定表

-- ============================================================================
-- 1. AI工具配置表
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tool_configs (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL UNIQUE,           -- 工具名称（如 reach.sms.send）
    category VARCHAR(50) NOT NULL,                     -- 工具分类（reach/customer/knowledge/business/pm）
    is_enabled BOOLEAN DEFAULT true,                   -- 是否启用
    config JSONB DEFAULT '{}',                         -- 工具特定配置
    default_account_id VARCHAR(64),                    -- 默认账号ID
    default_card_id VARCHAR(64),                       -- 默认卡片ID（触达工具）
    display_order INT DEFAULT 0,                       -- 显示排序
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ai_tool_configs_category ON ai_tool_configs(category);
CREATE INDEX IF NOT EXISTS idx_ai_tool_configs_enabled ON ai_tool_configs(is_enabled);

-- ============================================================================
-- 2. AI工具-账号绑定表
-- ============================================================================
CREATE TABLE IF NOT EXISTS ai_tool_account_bindings (
    id BIGSERIAL PRIMARY KEY,
    tool_name VARCHAR(100) NOT NULL,                   -- 工具名称
    account_type VARCHAR(50) NOT NULL,                 -- 账号类型（sms/email/telegram/wecom/feishu/whatsapp/douyin/kuaishou/xhs/xianyu）
    account_id VARCHAR(64) NOT NULL,                   -- 账号ID（对应各渠道表的ID）
    is_primary BOOLEAN DEFAULT false,                  -- 是否主账号
    config JSONB DEFAULT '{}',                         -- 工具特定配置（覆盖账号默认配置）
    enabled BOOLEAN DEFAULT true,                      -- 是否启用
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(tool_name, account_type, account_id)
);

CREATE INDEX IF NOT EXISTS idx_tool_bindings_name ON ai_tool_account_bindings(tool_name);
CREATE INDEX IF NOT EXISTS idx_tool_bindings_type ON ai_tool_account_bindings(account_type);

-- ============================================================================
-- 3. 初始化数据：插入40个AI工具配置
-- ============================================================================

-- 客户工具（8个）
INSERT INTO ai_tool_configs (tool_name, category, is_enabled, display_order) VALUES
('customer.search', 'customer', true, 1),
('customer.get', 'customer', true, 2),
('customer.create', 'customer', true, 3),
('customer.update', 'customer', true, 4),
('customer.merge', 'customer', true, 5),
('customer.add_tag', 'customer', true, 6),
('customer.remove_tag', 'customer', true, 7),
('customer.segment', 'customer', true, 8)
ON CONFLICT (tool_name) DO NOTHING;

-- 触达工具（20个）
INSERT INTO ai_tool_configs (tool_name, category, is_enabled, display_order) VALUES
('reach.sms.send', 'reach', true, 10),
('reach.email.send', 'reach', true, 11),
('reach.wecom.send', 'reach', true, 12),
('reach.weixin.send', 'reach', true, 13),
('reach.douyin.send', 'reach', true, 14),
('reach.kuaishou.send', 'reach', true, 15),
('reach.xhs.send', 'reach', true, 16),
('reach.dingtalk.send', 'reach', true, 17),
('reach.telegram.send', 'reach', true, 18),
('reach.whatsapp.send', 'reach', true, 19),
('reach.feishu.send', 'reach', true, 20),
('reach.web.send', 'reach', true, 21),
('reach.card.send', 'reach', true, 22),
('reach.batch', 'reach', true, 23),
('reach.schedule', 'reach', true, 24),
('reach.recall', 'reach', true, 25),
('reach.health', 'reach', true, 26),
('reach.history', 'reach', true, 27),
('reach.template.apply', 'reach', true, 28),
('reach.account.list', 'reach', true, 29)
ON CONFLICT (tool_name) DO NOTHING;

-- 私信工具（3个）
INSERT INTO ai_tool_configs (tool_name, category, is_enabled, display_order) VALUES
('pm.session.open', 'private_message', true, 30),
('pm.session.read', 'private_message', true, 31),
('pm.message.send', 'private_message', true, 32)
ON CONFLICT (tool_name) DO NOTHING;

-- 知识工具（4个）
INSERT INTO ai_tool_configs (tool_name, category, is_enabled, display_order) VALUES
('rag.search', 'knowledge', true, 40),
('knowledge.feedback', 'knowledge', true, 41),
('knowledge.add_doc', 'knowledge', true, 42),
('knowledge.list_kb', 'knowledge', true, 43)
ON CONFLICT (tool_name) DO NOTHING;

-- 业务工具（5个）
INSERT INTO ai_tool_configs (tool_name, category, is_enabled, display_order) VALUES
('follow_task.create', 'business', true, 50),
('follow_task.update', 'business', true, 51),
('order.lookup', 'business', true, 52),
('aftersale.create', 'business', true, 53),
('aftersale.query', 'business', true, 54)
ON CONFLICT (tool_name) DO NOTHING;