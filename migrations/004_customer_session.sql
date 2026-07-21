-- 人机协作客服系统数据库迁移脚本
-- 版本: 1.1.0  (2026-07-17 改为 PostgreSQL 15+ 语法)
-- 适用于: PostgreSQL 15+ (项目唯一数据库)
-- 创建时间: 2026-03-12 (原始) / 2026-07-17 (PG 化重写)

-- 客服会话表
CREATE TABLE IF NOT EXISTS customer_sessions (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50) UNIQUE NOT NULL,
    platform VARCHAR(20),
    account_id VARCHAR(50),
    user_id VARCHAR(50),
    user_name VARCHAR(100),
    user_avatar VARCHAR(500),
    user_phone VARCHAR(20),
    user_email VARCHAR(100),
    status VARCHAR(20) DEFAULT 'pending',
    handler_type VARCHAR(20),
    agent_id BIGINT,
    agent_name VARCHAR(100),
    priority INT DEFAULT 0,
    last_message TEXT,
    last_message_at TIMESTAMP,
    last_message_by VARCHAR(20),
    message_count INT DEFAULT 0,
    ai_reply_count INT DEFAULT 0,
    human_reply_count INT DEFAULT 0,
    avg_response_time INT,
    rating INT,
    rating_comment TEXT,
    tags TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP,
    closed_at TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_session_id ON customer_sessions (session_id);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_user ON customer_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_agent ON customer_sessions (agent_id);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_status ON customer_sessions (status);
CREATE INDEX IF NOT EXISTS idx_customer_sessions_created_at ON customer_sessions (created_at);
COMMENT ON COLUMN customer_sessions.session_id IS '会话唯一ID';
COMMENT ON COLUMN customer_sessions.platform IS '平台: douyin, kuaishou, xiaohongshu, xianyu';
COMMENT ON COLUMN customer_sessions.account_id IS '账号ID';
COMMENT ON COLUMN customer_sessions.user_id IS '用户ID';
COMMENT ON COLUMN customer_sessions.user_name IS '用户名称';
COMMENT ON COLUMN customer_sessions.user_avatar IS '用户头像';
COMMENT ON COLUMN customer_sessions.user_phone IS '用户电话';
COMMENT ON COLUMN customer_sessions.user_email IS '用户邮箱';
COMMENT ON COLUMN customer_sessions.status IS '状态: pending, ai_handling, human_handling, waiting, resolved, closed';
COMMENT ON COLUMN customer_sessions.handler_type IS '处理者类型: ai, human';
COMMENT ON COLUMN customer_sessions.agent_id IS '客服ID';
COMMENT ON COLUMN customer_sessions.agent_name IS '客服名称';
COMMENT ON COLUMN customer_sessions.priority IS '优先级: 0-普通, 1-重要, 2-紧急';
COMMENT ON COLUMN customer_sessions.last_message IS '最后消息';
COMMENT ON COLUMN customer_sessions.last_message_at IS '最后消息时间';
COMMENT ON COLUMN customer_sessions.last_message_by IS '最后消息发送者: user, ai, agent';
COMMENT ON COLUMN customer_sessions.message_count IS '消息总数';
COMMENT ON COLUMN customer_sessions.ai_reply_count IS 'AI回复数';
COMMENT ON COLUMN customer_sessions.human_reply_count IS '人工回复数';
COMMENT ON COLUMN customer_sessions.avg_response_time IS '平均响应时间(秒)';
COMMENT ON COLUMN customer_sessions.rating IS '用户评分 1-5';
COMMENT ON COLUMN customer_sessions.rating_comment IS '评分评论';
COMMENT ON COLUMN customer_sessions.tags IS '标签(JSON数组)';
COMMENT ON COLUMN customer_sessions.resolved_at IS '解决时间';
COMMENT ON COLUMN customer_sessions.closed_at IS '关闭时间';
COMMENT ON TABLE customer_sessions IS '客服会话表';

-- 会话消息表
CREATE TABLE IF NOT EXISTS session_messages (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    content TEXT,
    content_type VARCHAR(20) DEFAULT 'text',
    media_url VARCHAR(500),
    sender_type VARCHAR(20) NOT NULL,
    sender_id VARCHAR(50),
    sender_name VARCHAR(100),
    sender_avatar VARCHAR(500),
    ai_confidence DECIMAL(5,2),
    ai_source VARCHAR(20),
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_session_messages_session ON session_messages (session_id);
CREATE INDEX IF NOT EXISTS idx_session_messages_created_at ON session_messages (created_at);
COMMENT ON COLUMN session_messages.session_id IS '会话ID';
COMMENT ON COLUMN session_messages.content IS '消息内容';
COMMENT ON COLUMN session_messages.content_type IS '消息类型: text, image, video, audio, file';
COMMENT ON COLUMN session_messages.media_url IS '媒体URL';
COMMENT ON COLUMN session_messages.sender_type IS '发送者类型: user, ai, agent';
COMMENT ON COLUMN session_messages.sender_id IS '发送者ID';
COMMENT ON COLUMN session_messages.sender_name IS '发送者名称';
COMMENT ON COLUMN session_messages.sender_avatar IS '发送者头像';
COMMENT ON COLUMN session_messages.ai_confidence IS 'AI置信度';
COMMENT ON COLUMN session_messages.ai_source IS 'AI来源: rule, rag, llm';
COMMENT ON COLUMN session_messages.is_read IS '是否已读';
COMMENT ON COLUMN session_messages.read_at IS '已读时间';
COMMENT ON TABLE session_messages IS '会话消息表';

-- 客服状态表
CREATE TABLE IF NOT EXISTS agent_statuses (
    id BIGSERIAL PRIMARY KEY,
    agent_id BIGINT UNIQUE NOT NULL,
    agent_name VARCHAR(100),
    status VARCHAR(20) DEFAULT 'offline',
    max_sessions INT DEFAULT 5,
    active_sessions INT DEFAULT 0,
    today_sessions INT DEFAULT 0,
    today_messages INT DEFAULT 0,
    avg_response_time INT DEFAULT 0,
    online_at TIMESTAMP,
    offline_at TIMESTAMP,
    last_active_at TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_agent_statuses_agent ON agent_statuses (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_statuses_status ON agent_statuses (status);
COMMENT ON COLUMN agent_statuses.agent_name IS '客服名称';
COMMENT ON COLUMN agent_statuses.status IS '状态: online, busy, away, offline';
COMMENT ON COLUMN agent_statuses.max_sessions IS '最大会话数';
COMMENT ON COLUMN agent_statuses.active_sessions IS '活跃会话数';
COMMENT ON COLUMN agent_statuses.today_sessions IS '今日会话数';
COMMENT ON COLUMN agent_statuses.today_messages IS '今日消息数';
COMMENT ON COLUMN agent_statuses.avg_response_time IS '平均响应时间(秒)';
COMMENT ON COLUMN agent_statuses.online_at IS '上线时间';
COMMENT ON COLUMN agent_statuses.offline_at IS '下线时间';
COMMENT ON COLUMN agent_statuses.last_active_at IS '最后活跃时间';
COMMENT ON TABLE agent_statuses IS '客服状态表';

-- updated_at 自动更新触发器 (agent_statuses)
CREATE OR REPLACE FUNCTION agent_statuses_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_agent_statuses_updated_at ON agent_statuses;
CREATE TRIGGER trg_agent_statuses_updated_at
    BEFORE UPDATE ON agent_statuses
    FOR EACH ROW EXECUTE FUNCTION agent_statuses_set_updated_at();

-- AI建议表
CREATE TABLE IF NOT EXISTS ai_suggestions (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50),
    message_id BIGINT,
    suggestion TEXT,
    confidence DECIMAL(5,2),
    source VARCHAR(20),
    is_used BOOLEAN DEFAULT FALSE,
    used_by BIGINT,
    used_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_suggestions_session ON ai_suggestions (session_id);
COMMENT ON COLUMN ai_suggestions.session_id IS '会话ID';
COMMENT ON COLUMN ai_suggestions.message_id IS '消息ID';
COMMENT ON COLUMN ai_suggestions.suggestion IS '建议内容';
COMMENT ON COLUMN ai_suggestions.confidence IS '置信度';
COMMENT ON COLUMN ai_suggestions.source IS '来源: rule, rag, llm';
COMMENT ON COLUMN ai_suggestions.is_used IS '是否已使用';
COMMENT ON COLUMN ai_suggestions.used_by IS '使用的客服ID';
COMMENT ON COLUMN ai_suggestions.used_at IS '使用时间';
COMMENT ON TABLE ai_suggestions IS 'AI建议表';

-- 快捷回复表
CREATE TABLE IF NOT EXISTS quick_replies (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(50),
    title VARCHAR(100) NOT NULL,
    content TEXT NOT NULL,
    sort_order INT DEFAULT 0,
    is_public BOOLEAN DEFAULT TRUE,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_quick_replies_category ON quick_replies (category);
COMMENT ON COLUMN quick_replies.category IS '分类';
COMMENT ON COLUMN quick_replies.title IS '标题';
COMMENT ON COLUMN quick_replies.content IS '内容';
COMMENT ON COLUMN quick_replies.sort_order IS '排序';
COMMENT ON COLUMN quick_replies.is_public IS '是否公开';
COMMENT ON COLUMN quick_replies.created_by IS '创建者ID';
COMMENT ON TABLE quick_replies IS '快捷回复表';

-- updated_at 自动更新触发器 (quick_replies)
CREATE OR REPLACE FUNCTION quick_replies_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_quick_replies_updated_at ON quick_replies;
CREATE TRIGGER trg_quick_replies_updated_at
    BEFORE UPDATE ON quick_replies
    FOR EACH ROW EXECUTE FUNCTION quick_replies_set_updated_at();

-- 会话标签表
CREATE TABLE IF NOT EXISTS session_tags (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(50) UNIQUE NOT NULL,
    color VARCHAR(20),
    sort_order INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
COMMENT ON COLUMN session_tags.name IS '标签名称';
COMMENT ON COLUMN session_tags.color IS '标签颜色';
COMMENT ON COLUMN session_tags.sort_order IS '排序';
COMMENT ON TABLE session_tags IS '会话标签表';

-- 初始化默认会话标签 (使用 ON CONFLICT 处理幂等, 按 name 唯一)
INSERT INTO session_tags (name, color, sort_order) VALUES
    ('咨询', '#1890ff', 1),
    ('投诉', '#ff4d4f', 2),
    ('售后', '#52c41a', 3),
    ('催单', '#faad14', 4),
    ('已解决', '#13c2c2', 5)
ON CONFLICT (name) DO UPDATE SET
    color = EXCLUDED.color,
    sort_order = EXCLUDED.sort_order;

-- 初始化默认快捷回复 (使用 ON CONFLICT 处理幂等, 按 category+title 唯一需要先加唯一约束)
-- 业务侧运行时, 快捷回复 title 在 category 内应当唯一
CREATE UNIQUE INDEX IF NOT EXISTS uq_quick_replies_category_title ON quick_replies (category, title);
INSERT INTO quick_replies (category, title, content, sort_order, is_public) VALUES
    ('问候', '欢迎语', '您好，很高兴为您服务！请问有什么可以帮助您的吗？', 1, TRUE),
    ('问候', '感谢', '感谢您的咨询，祝您生活愉快！', 2, TRUE),
    ('常见问题', '发货时间', '您好，我们会在24小时内为您发货，请您耐心等待。', 3, TRUE),
    ('常见问题', '物流查询', '您好，您可以在订单详情页面查看物流信息，或者告诉我您的订单号，我帮您查询。', 4, TRUE),
    ('常见问题', '退换货', '您好，如需退换货，请在订单详情页面申请，我们会在3个工作日内处理。', 5, TRUE),
    ('转人工', '转接', '好的，我这就为您转接人工客服，请稍等。', 6, TRUE),
    ('转人工', '等待', '人工客服正忙，请您稍等，我们会尽快为您服务。', 7, TRUE)
ON CONFLICT (category, title) DO UPDATE SET
    content = EXCLUDED.content,
    sort_order = EXCLUDED.sort_order,
    is_public = EXCLUDED.is_public;
