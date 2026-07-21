-- AI 私域销冠系统 - 数据库迁移脚本
-- 011_ai_sales_champion.sql
-- 适用于 PostgreSQL 15+ (项目唯一数据库)

-- ============================================================
-- 1. 个人微信账号表
-- ============================================================
CREATE TABLE IF NOT EXISTS personal_wechat_accounts (
    id BIGSERIAL PRIMARY KEY,
    wxid VARCHAR(100) NOT NULL,
    nickname VARCHAR(100),
    avatar VARCHAR(500),
    phone VARCHAR(20),
    status INTEGER DEFAULT 1,  -- 1-在线 0-离线 2-封禁
    login_state VARCHAR(20) DEFAULT 'offline', -- online/offline/expired
    device_info TEXT,
    last_active_at TIMESTAMP,
    friend_count INTEGER DEFAULT 0,
    group_count INTEGER DEFAULT 0,
    daily_msg_quota INTEGER DEFAULT 200,
    daily_msg_used INTEGER DEFAULT 0,
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(wxid)
);
CREATE INDEX idx_personal_wx_status ON personal_wechat_accounts(status);

-- ============================================================
-- 2. 消息中台表（多账号聚合）
-- ============================================================
CREATE TABLE IF NOT EXISTS message_hub (
    id BIGSERIAL PRIMARY KEY,
    msg_id VARCHAR(100) UNIQUE,
    platform VARCHAR(30) NOT NULL,  -- wecom / personal_wx / douyin / ...
    account_id VARCHAR(100) NOT NULL,
    direction VARCHAR(10) NOT NULL, -- inbound / outbound
    msg_type VARCHAR(20) NOT NULL,  -- text / image / file / link / card
    sender_id VARCHAR(100),
    sender_name VARCHAR(200),
    receiver_id VARCHAR(100),
    receiver_name VARCHAR(200),
    content TEXT,
    media_url VARCHAR(500),
    conversation_id VARCHAR(100),
    is_group BOOLEAN DEFAULT FALSE,
    group_id VARCHAR(100),
    is_ai_reply BOOLEAN DEFAULT FALSE,
    ai_agent VARCHAR(50),
    is_read BOOLEAN DEFAULT FALSE,
    read_at TIMESTAMP,
    sent_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_message_hub_account ON message_hub(account_id);
CREATE INDEX idx_message_hub_platform ON message_hub(platform);
CREATE INDEX idx_message_hub_sender ON message_hub(sender_id);
CREATE INDEX idx_message_hub_conversation ON message_hub(conversation_id);
CREATE INDEX idx_message_hub_sent_at ON message_hub(sent_at DESC);

-- ============================================================
-- 3. 销售意图识别记录
-- ============================================================
CREATE TABLE IF NOT EXISTS intent_records (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    customer_id VARCHAR(64),
    message_id BIGINT,
    raw_text TEXT NOT NULL,
    intent_type VARCHAR(50) NOT NULL,  -- price_inquiry / objection_price / purchase / after_sale / churn / social
    intent_subtype VARCHAR(50),
    confidence DECIMAL(5,2) NOT NULL,
    confidence_level VARCHAR(20),  -- high/medium/low
    entities JSONB,  -- 关键实体抽取
    sentiment VARCHAR(20),  -- positive/negative/neutral
    llm_model VARCHAR(50),
    cost_tokens INTEGER DEFAULT 0,
    latency_ms INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_intent_records_session ON intent_records(session_id);
CREATE INDEX idx_intent_records_customer ON intent_records(customer_id);
CREATE INDEX idx_intent_records_type ON intent_records(intent_type);
CREATE INDEX idx_intent_records_created ON intent_records(created_at DESC);

-- ============================================================
-- 4. 对话长期记忆
-- ============================================================
CREATE TABLE IF NOT EXISTS dialogue_memories (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    customer_id VARCHAR(64) NOT NULL,
    summary TEXT,
    key_facts JSONB,  -- 关键事实
    customer_name VARCHAR(100),
    customer_phone VARCHAR(20),
    customer_wechat VARCHAR(100),
    budget VARCHAR(200),
    demand TEXT,
    objections JSONB,  -- 历史异议点
    purchase_intent VARCHAR(20),  -- high/medium/low
    intent_trail JSONB,  -- 意图轨迹
    sop_history JSONB,  -- 经历 SOP
    last_action VARCHAR(100),
    next_action_suggestion VARCHAR(200),
    last_active_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    message_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(session_id)
);
CREATE INDEX idx_dialogue_memories_customer ON dialogue_memories(customer_id);
CREATE INDEX idx_dialogue_memories_intent ON dialogue_memories(purchase_intent);

-- ============================================================
-- 5. SOP 智能体定义
-- ============================================================
CREATE TABLE IF NOT EXISTS sop_agents (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    scenario VARCHAR(50) NOT NULL,  -- new_customer / dormant / purchase / objection
    description VARCHAR(500),
    trigger_type VARCHAR(50),  -- manual / auto_intent / auto_event
    trigger_config JSONB,
    sop_graph JSONB NOT NULL,  -- 流程图定义
    version INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT TRUE,
    priority INTEGER DEFAULT 0,
    execution_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sop_agents_scenario ON sop_agents(scenario);
CREATE INDEX idx_sop_agents_active ON sop_agents(is_active);

-- ============================================================
-- 6. SOP 执行记录
-- ============================================================
CREATE TABLE IF NOT EXISTS sop_executions (
    id BIGSERIAL PRIMARY KEY,
    sop_id BIGINT NOT NULL,
    customer_id VARCHAR(64) NOT NULL,
    session_id VARCHAR(50),
    current_node VARCHAR(50),
    current_node_index INTEGER DEFAULT 0,
    status VARCHAR(20) DEFAULT 'running',  -- running / completed / failed / paused
    execution_data JSONB,
    started_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_sop_executions_sop ON sop_executions(sop_id);
CREATE INDEX idx_sop_executions_customer ON sop_executions(customer_id);
CREATE INDEX idx_sop_executions_status ON sop_executions(status);

-- ============================================================
-- 7. 销售意向打分
-- ============================================================
CREATE TABLE IF NOT EXISTS sales_intent_scores (
    id BIGSERIAL PRIMARY KEY,
    customer_id VARCHAR(64) NOT NULL,
    total_score DECIMAL(5,2) NOT NULL,  -- 0-100
    intent_level VARCHAR(20) NOT NULL,  -- S/A/B/C/D
    dimensions JSONB,  -- 各维度得分
    behavior_score DECIMAL(5,2),  -- 行为分
    content_score DECIMAL(5,2),   -- 内容分
    frequency_score DECIMAL(5,2), -- 频次分
    profile_score DECIMAL(5,2),   -- 画像分
    last_message_at TIMESTAMP,
    last_intent_type VARCHAR(50),
    last_score_change DECIMAL(5,2),
    recommended_action VARCHAR(200),
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(customer_id)
);
CREATE INDEX idx_sales_intent_total ON sales_intent_scores(total_score DESC);
CREATE INDEX idx_sales_intent_level ON sales_intent_scores(intent_level);

-- ============================================================
-- 8. 朋友圈发布表
-- ============================================================
CREATE TABLE IF NOT EXISTS moments_posts (
    id BIGSERIAL PRIMARY KEY,
    account_id VARCHAR(100) NOT NULL,
    account_type VARCHAR(20) NOT NULL,  -- wecom / personal_wx
    content TEXT NOT NULL,
    images JSONB,  -- 图片URL列表
    video_url VARCHAR(500),
    link_url VARCHAR(500),
    link_title VARCHAR(200),
    link_description VARCHAR(500),
    link_image VARCHAR(500),
    ai_generated BOOLEAN DEFAULT FALSE,
    ai_prompt TEXT,
    persona VARCHAR(100),  -- 销冠人设
    scheduled_at TIMESTAMP,
    published_at TIMESTAMP,
    status VARCHAR(20) DEFAULT 'draft',  -- draft / scheduled / published / failed
    publish_error TEXT,
    like_count INTEGER DEFAULT 0,
    comment_count INTEGER DEFAULT 0,
    view_count INTEGER DEFAULT 0,
    share_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_moments_account ON moments_posts(account_id);
CREATE INDEX idx_moments_status ON moments_posts(status);
CREATE INDEX idx_moments_scheduled ON moments_posts(scheduled_at);

-- ============================================================
-- 9. 销冠话术库
-- ============================================================
CREATE TABLE IF NOT EXISTS script_library (
    id BIGSERIAL PRIMARY KEY,
    category VARCHAR(50) NOT NULL,  -- greeting / product / price / objection / closing / after_sale
    subcategory VARCHAR(50),
    title VARCHAR(200) NOT NULL,
    content TEXT NOT NULL,
    scenario VARCHAR(100),  -- 适用场景
    tags JSONB,
    usage_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    conversion_rate DECIMAL(5,2) DEFAULT 0,
    is_featured BOOLEAN DEFAULT FALSE,
    created_by BIGINT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_script_library_category ON script_library(category);
CREATE INDEX idx_script_library_featured ON script_library(is_featured);

-- ============================================================
-- 10. 异议处理模板
-- ============================================================
CREATE TABLE IF NOT EXISTS objection_templates (
    id BIGSERIAL PRIMARY KEY,
    objection_type VARCHAR(50) NOT NULL,  -- price / need / trust / timing / competitor
    objection_keyword VARCHAR(200) NOT NULL,  -- 异议关键词
    objection_pattern VARCHAR(500),  -- 匹配模式(正则)
    reply_template TEXT NOT NULL,
    reply_strategy VARCHAR(200),  -- 处理策略说明
    example_reply TEXT,
    use_count INTEGER DEFAULT 0,
    success_count INTEGER DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    priority INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_objection_type ON objection_templates(objection_type);
CREATE INDEX idx_objection_active ON objection_templates(is_active);

-- ============================================================
-- 11. 转化漏斗统计
-- ============================================================
CREATE TABLE IF NOT EXISTS conversion_funnels (
    id BIGSERIAL PRIMARY KEY,
    stat_date VARCHAR(20) NOT NULL,  -- YYYY-MM-DD
    funnel_type VARCHAR(50) NOT NULL,  -- sales / sop / ai_agent
    stage VARCHAR(50) NOT NULL,  -- new / contacted / intent / opportunity / negotiation / won / lost
    stage_order INTEGER NOT NULL,
    count INTEGER DEFAULT 0,
    conversion_rate DECIMAL(5,2) DEFAULT 0,  -- 阶段间转化率
    drop_off_rate DECIMAL(5,2) DEFAULT 0,   -- 流失率
    avg_duration_seconds INTEGER DEFAULT 0, -- 平均停留时长
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_funnel_date ON conversion_funnels(stat_date);
CREATE INDEX idx_funnel_type ON conversion_funnels(funnel_type);

-- ============================================================
-- 12. 销冠能力画像
-- ============================================================
CREATE TABLE IF NOT EXISTS sales_personas (
    id BIGSERIAL PRIMARY KEY,
    sales_id VARCHAR(64) NOT NULL,
    sales_name VARCHAR(100),
    avatar VARCHAR(500),
    total_customers INTEGER DEFAULT 0,
    active_customers INTEGER DEFAULT 0,
    converted_customers INTEGER DEFAULT 0,
    conversion_rate DECIMAL(5,2) DEFAULT 0,
    avg_response_seconds INTEGER DEFAULT 0,
    avg_deal_amount DECIMAL(12,2) DEFAULT 0,
    total_revenue DECIMAL(14,2) DEFAULT 0,
    skill_tags JSONB,  -- 能力标签
    best_scenarios JSONB,  -- 最擅长场景
    work_days INTEGER DEFAULT 0,
    last_active_at TIMESTAMP,
    level VARCHAR(20),  -- top / senior / intermediate / junior
    level_score DECIMAL(5,2) DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(sales_id)
);
CREATE INDEX idx_sales_personas_level ON sales_personas(level);
CREATE INDEX idx_sales_personas_conversion ON sales_personas(conversion_rate DESC);

-- ============================================================
-- 13. AI 谈单日志（用于回溯/分析/反馈学习）
-- ============================================================
CREATE TABLE IF NOT EXISTS ai_sales_logs (
    id BIGSERIAL PRIMARY KEY,
    session_id VARCHAR(50) NOT NULL,
    customer_id VARCHAR(64),
    sop_id BIGINT,
    llm_model VARCHAR(50),
    scenario VARCHAR(50),  -- intent / reply / objection / funnel
    prompt_tokens INTEGER DEFAULT 0,
    completion_tokens INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    cost DECIMAL(10,4) DEFAULT 0,
    latency_ms INTEGER DEFAULT 0,
    success BOOLEAN DEFAULT TRUE,
    error_message TEXT,
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ai_sales_logs_session ON ai_sales_logs(session_id);
CREATE INDEX idx_ai_sales_logs_scenario ON ai_sales_logs(scenario);
CREATE INDEX idx_ai_sales_logs_created ON ai_sales_logs(created_at DESC);

-- ============================================================
-- 14. 触达 Pipeline 表（Reach Pipeline）
-- ============================================================
CREATE TABLE IF NOT EXISTS reach_pipelines (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500),
    channel VARCHAR(30) NOT NULL,                  -- wecom/sms/email/card/weixin/dingtalk
    steps JSONB NOT NULL,                          -- 9 步配置
    retry_policy JSONB,                            -- 重试策略
    rate_limit JSONB,                              -- 限流配置
    status VARCHAR(20) DEFAULT 'active',           -- active/paused/archived
    version INTEGER DEFAULT 1,
    total_runs BIGINT DEFAULT 0,
    total_success BIGINT DEFAULT 0,
    total_failure BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_reach_pipeline_channel ON reach_pipelines(channel);
CREATE INDEX idx_reach_pipeline_status ON reach_pipelines(status);

-- ============================================================
-- 15. 触达任务表（Reach Job）
-- ============================================================
CREATE TABLE IF NOT EXISTS reach_jobs (
    id BIGSERIAL PRIMARY KEY,
    pipeline_id BIGINT NOT NULL,
    channel VARCHAR(30),
    customer_id VARCHAR(64),
    account_id VARCHAR(100),
    payload JSONB NOT NULL,
    state VARCHAR(20) DEFAULT 'pending',           -- pending/running/success/failed/canceled
    current_step INTEGER DEFAULT 0,
    step_results JSONB,
    retry_count INTEGER DEFAULT 0,
    max_retry INTEGER DEFAULT 3,
    next_run_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    duration_ms INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_reach_job_pipeline ON reach_jobs(pipeline_id);
CREATE INDEX idx_reach_job_customer ON reach_jobs(customer_id);
CREATE INDEX idx_reach_job_account ON reach_jobs(account_id);
CREATE INDEX idx_reach_job_state ON reach_jobs(state);
CREATE INDEX idx_reach_job_next_run ON reach_jobs(next_run_at);

-- ============================================================
-- 16. 企微账号健康度表（WeCom Account Health）
-- ============================================================
CREATE TABLE IF NOT EXISTS wecom_account_health (
    id BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL,
    platform VARCHAR(30) DEFAULT 'wecom',
    health_score INTEGER DEFAULT 0,                -- 0-100
    risk_level VARCHAR(20) DEFAULT 'normal',      -- normal/warning/critical/banned
    login_state VARCHAR(20),
    quota_used INTEGER DEFAULT 0,
    quota_total INTEGER DEFAULT 0,
    quota_usage_rate DECIMAL(5,2) DEFAULT 0,
    success_rate DECIMAL(5,2) DEFAULT 100,
    error_count INTEGER DEFAULT 0,
    last_error VARCHAR(500),
    metrics JSONB,
    reported_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_wecom_health_account ON wecom_account_health(account_id);
CREATE INDEX idx_wecom_health_reported ON wecom_account_health(reported_at DESC);
CREATE INDEX idx_wecom_health_risk ON wecom_account_health(risk_level);

-- ============================================================
-- 17. 统一收件箱会话表（Inbox Conversation）
-- ============================================================
CREATE TABLE IF NOT EXISTS inbox_conversations (
    id BIGSERIAL PRIMARY KEY,
    conversation_id VARCHAR(100) NOT NULL,
    platform VARCHAR(30) NOT NULL,                 -- wecom/personal_wx/douyin
    account_id VARCHAR(100) NOT NULL,
    customer_id VARCHAR(64),
    customer_name VARCHAR(200),
    last_message TEXT,
    last_message_at TIMESTAMP,
    unread_count INTEGER DEFAULT 0,
    status VARCHAR(20) DEFAULT 'open',             -- open/closed/pending
    priority VARCHAR(20) DEFAULT 'normal',         -- low/normal/high/urgent
    assigned_to BIGINT,
    tags JSONB,
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(conversation_id)
);
CREATE INDEX idx_inbox_conv_platform ON inbox_conversations(platform);
CREATE INDEX idx_inbox_conv_customer ON inbox_conversations(customer_id);
CREATE INDEX idx_inbox_conv_status ON inbox_conversations(status);
CREATE INDEX idx_inbox_conv_assigned ON inbox_conversations(assigned_to);
CREATE INDEX idx_inbox_conv_last_msg ON inbox_conversations(last_message_at DESC);

-- ============================================================
-- 18. 统一收件箱分配记录表（Inbox Assignment）
-- ============================================================
CREATE TABLE IF NOT EXISTS inbox_assignments (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL,
    assignee_id BIGINT NOT NULL,
    assigner_id BIGINT,
    assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    released_at TIMESTAMP,
    reason VARCHAR(200),
    auto_assigned BOOLEAN DEFAULT FALSE,
    extra JSONB,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_inbox_assign_conversation ON inbox_assignments(conversation_id);
CREATE INDEX idx_inbox_assign_assignee ON inbox_assignments(assignee_id);
