-- 多平台自动回复打通数据库迁移脚本
-- 版本: 1.1.0  (2026-07-17 改为 PostgreSQL 15+ 语法)
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

-- 统一消息表
CREATE TABLE IF NOT EXISTS unified_messages (
    id BIGSERIAL PRIMARY KEY,
    message_id VARCHAR(50) UNIQUE NOT NULL,
    platform VARCHAR(20) NOT NULL,
    account_id VARCHAR(50),
    account_name VARCHAR(100),
    chat_id VARCHAR(50),
    chat_type VARCHAR(20) DEFAULT 'private',
    sender_id VARCHAR(50),
    sender_name VARCHAR(100),
    sender_avatar VARCHAR(500),
    content TEXT,
    content_type VARCHAR(20) DEFAULT 'text',
    media_url VARCHAR(500),
    reply_to_id VARCHAR(50),
    status VARCHAR(20) DEFAULT 'pending',
    raw_data TEXT,
    received_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_unified_messages_platform_account ON unified_messages (platform, account_id);
CREATE INDEX IF NOT EXISTS idx_unified_messages_chat ON unified_messages (chat_id);
CREATE INDEX IF NOT EXISTS idx_unified_messages_sender ON unified_messages (sender_id);
CREATE INDEX IF NOT EXISTS idx_unified_messages_status ON unified_messages (status);
CREATE INDEX IF NOT EXISTS idx_unified_messages_created_at ON unified_messages (created_at);
COMMENT ON COLUMN unified_messages.message_id IS '消息唯一ID';
COMMENT ON COLUMN unified_messages.platform IS '平台: douyin, kuaishou, xiaohongshu, xianyu';
COMMENT ON COLUMN unified_messages.account_id IS '账号ID';
COMMENT ON COLUMN unified_messages.account_name IS '账号名称';
COMMENT ON COLUMN unified_messages.chat_id IS '会话ID';
COMMENT ON COLUMN unified_messages.chat_type IS '会话类型: private, group';
COMMENT ON COLUMN unified_messages.sender_id IS '发送者ID';
COMMENT ON COLUMN unified_messages.sender_name IS '发送者名称';
COMMENT ON COLUMN unified_messages.sender_avatar IS '发送者头像';
COMMENT ON COLUMN unified_messages.content IS '消息内容';
COMMENT ON COLUMN unified_messages.content_type IS '内容类型: text, image, video, audio, file';
COMMENT ON COLUMN unified_messages.media_url IS '媒体URL';
COMMENT ON COLUMN unified_messages.reply_to_id IS '回复的消息ID';
COMMENT ON COLUMN unified_messages.status IS '状态: pending, processing, replied, failed, ignored';
COMMENT ON COLUMN unified_messages.raw_data IS '原始平台数据(JSON)';
COMMENT ON COLUMN unified_messages.received_at IS '接收时间';
COMMENT ON TABLE unified_messages IS '统一消息表';

-- 统一回复表
CREATE TABLE IF NOT EXISTS unified_replies (
    id BIGSERIAL PRIMARY KEY,
    reply_id VARCHAR(50) UNIQUE,
    message_id VARCHAR(50) NOT NULL,
    platform VARCHAR(20) NOT NULL,
    account_id VARCHAR(50),
    chat_id VARCHAR(50),
    content TEXT,
    content_type VARCHAR(20) DEFAULT 'text',
    media_url VARCHAR(500),
    reply_type VARCHAR(20),
    confidence DECIMAL(5,2),
    rule_id BIGINT,
    knowledge_id BIGINT,
    agent_id BIGINT,
    status VARCHAR(20) DEFAULT 'pending',
    error_message TEXT,
    platform_msg_id VARCHAR(50),
    sent_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_unified_replies_message ON unified_replies (message_id);
CREATE INDEX IF NOT EXISTS idx_unified_replies_platform ON unified_replies (platform);
CREATE INDEX IF NOT EXISTS idx_unified_replies_reply_type ON unified_replies (reply_type);
CREATE INDEX IF NOT EXISTS idx_unified_replies_created_at ON unified_replies (created_at);
COMMENT ON COLUMN unified_replies.reply_id IS '回复唯一ID';
COMMENT ON COLUMN unified_replies.message_id IS '关联的消息ID';
COMMENT ON COLUMN unified_replies.platform IS '平台';
COMMENT ON COLUMN unified_replies.account_id IS '账号ID';
COMMENT ON COLUMN unified_replies.chat_id IS '会话ID';
COMMENT ON COLUMN unified_replies.content IS '回复内容';
COMMENT ON COLUMN unified_replies.content_type IS '内容类型';
COMMENT ON COLUMN unified_replies.media_url IS '媒体URL';
COMMENT ON COLUMN unified_replies.reply_type IS '回复类型: rule, rag, llm, human';
COMMENT ON COLUMN unified_replies.confidence IS '置信度';
COMMENT ON COLUMN unified_replies.rule_id IS '匹配的规则ID';
COMMENT ON COLUMN unified_replies.knowledge_id IS '命中的知识库ID';
COMMENT ON COLUMN unified_replies.agent_id IS '人工客服ID';
COMMENT ON COLUMN unified_replies.status IS '状态: pending, sent, failed, discarded';
COMMENT ON COLUMN unified_replies.error_message IS '错误信息';
COMMENT ON COLUMN unified_replies.platform_msg_id IS '平台返回的消息ID';
COMMENT ON COLUMN unified_replies.sent_at IS '发送时间';
COMMENT ON TABLE unified_replies IS '统一回复表';

-- 平台账号配置表
CREATE TABLE IF NOT EXISTS platform_accounts (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(20) NOT NULL,
    account_id VARCHAR(50),
    account_name VARCHAR(100),
    account_avatar VARCHAR(500),
    config TEXT,
    cookie TEXT,
    token TEXT,
    status INT DEFAULT 1,
    last_sync_at TIMESTAMP,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_platform_accounts_platform ON platform_accounts (platform);
CREATE INDEX IF NOT EXISTS idx_platform_accounts_status ON platform_accounts (status);
COMMENT ON COLUMN platform_accounts.platform IS '平台';
COMMENT ON COLUMN platform_accounts.account_id IS '平台账号ID';
COMMENT ON COLUMN platform_accounts.account_name IS '账号名称';
COMMENT ON COLUMN platform_accounts.account_avatar IS '账号头像';
COMMENT ON COLUMN platform_accounts.config IS '平台特定配置(JSON)';
COMMENT ON COLUMN platform_accounts.cookie IS '加密存储的Cookie';
COMMENT ON COLUMN platform_accounts.token IS '加密存储的Token';
COMMENT ON COLUMN platform_accounts.status IS '状态: 1-正常, 0-禁用';
COMMENT ON COLUMN platform_accounts.last_sync_at IS '最后同步时间';
COMMENT ON COLUMN platform_accounts.expires_at IS '凭证过期时间';
COMMENT ON TABLE platform_accounts IS '平台账号配置表';

-- updated_at 自动更新触发器
CREATE OR REPLACE FUNCTION platform_accounts_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_platform_accounts_updated_at ON platform_accounts;
CREATE TRIGGER trg_platform_accounts_updated_at
    BEFORE UPDATE ON platform_accounts
    FOR EACH ROW EXECUTE FUNCTION platform_accounts_set_updated_at();
