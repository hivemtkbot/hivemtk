-- AI内容创作模块数据库迁移脚本
-- 版本: 1.1.0
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

-- AI生成记录表
CREATE TABLE IF NOT EXISTS ai_generation_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    type VARCHAR(20) NOT NULL,
    input TEXT,
    output TEXT,
    template_id BIGINT,
    model VARCHAR(50),
    tokens_used INT DEFAULT 0,
    is_saved BOOLEAN DEFAULT FALSE,
    is_favorite BOOLEAN DEFAULT FALSE,
    rating INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_ai_records_type ON ai_generation_records (type);
CREATE INDEX IF NOT EXISTS idx_ai_records_created_at ON ai_generation_records (created_at);
CREATE INDEX IF NOT EXISTS idx_ai_records_is_favorite ON ai_generation_records (is_favorite);
COMMENT ON COLUMN ai_generation_records.user_id IS '用户ID';
COMMENT ON COLUMN ai_generation_records.type IS '生成类型: copywriting, title, reply等';
COMMENT ON COLUMN ai_generation_records.input IS '输入内容';
COMMENT ON COLUMN ai_generation_records.output IS '生成输出';
COMMENT ON COLUMN ai_generation_records.template_id IS '使用的模板ID';
COMMENT ON COLUMN ai_generation_records.model IS '使用的模型';
COMMENT ON COLUMN ai_generation_records.tokens_used IS '消耗的token数';
COMMENT ON COLUMN ai_generation_records.is_saved IS '是否保存';
COMMENT ON COLUMN ai_generation_records.is_favorite IS '是否收藏';
COMMENT ON COLUMN ai_generation_records.rating IS '评分(1-5)';
COMMENT ON TABLE ai_generation_records IS 'AI生成记录表';

-- 提示词模板表
CREATE TABLE IF NOT EXISTS prompt_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(20) NOT NULL,
    template TEXT NOT NULL,
    variables TEXT,
    description VARCHAR(255),
    example TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    status INT DEFAULT 1,
    use_count INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_type ON prompt_templates (type);
CREATE INDEX IF NOT EXISTS idx_prompt_templates_is_system ON prompt_templates (is_system);
COMMENT ON COLUMN prompt_templates.name IS '模板名称';
COMMENT ON COLUMN prompt_templates.type IS '模板类型';
COMMENT ON COLUMN prompt_templates.template IS '模板内容';
COMMENT ON COLUMN prompt_templates.variables IS '变量定义(JSON)';
COMMENT ON COLUMN prompt_templates.description IS '描述';
COMMENT ON COLUMN prompt_templates.example IS '示例输出';
COMMENT ON COLUMN prompt_templates.is_system IS '是否系统模板';
COMMENT ON COLUMN prompt_templates.status IS '状态: 0-禁用, 1-启用';
COMMENT ON COLUMN prompt_templates.use_count IS '使用次数';
COMMENT ON TABLE prompt_templates IS '提示词模板表';

-- updated_at 自动更新触发器
CREATE OR REPLACE FUNCTION prompt_templates_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_prompt_templates_updated_at ON prompt_templates;
CREATE TRIGGER trg_prompt_templates_updated_at
    BEFORE UPDATE ON prompt_templates
    FOR EACH ROW EXECUTE FUNCTION prompt_templates_set_updated_at();

-- 插入系统默认模板 (使用 ON CONFLICT 处理幂等, 按 type+name 唯一)
INSERT INTO prompt_templates (type, name, template, variables, description, is_system, status) VALUES
    ('copywriting', '营销文案生成', E'请为以下产品/服务生成一段吸引人的营销文案：\n\n产品/服务：{{product}}\n目标受众：{{audience}}\n核心卖点：{{selling_points}}\n\n要求：\n1. 突出产品优势\n2. 语言生动有感染力\n3. 字数控制在{{word_count}}字左右', '[{"name":"product","type":"string","required":true},{"name":"audience","type":"string","required":true},{"name":"selling_points","type":"string","required":true},{"name":"word_count","type":"number","default":200}]', '根据产品信息和目标受众生成营销文案', TRUE, 1),
    ('title', '标题生成', E'请为以下内容生成{{count}}个吸引人的标题：\n\n内容摘要：{{content}}\n风格：{{style}}\n\n要求：\n1. 标题要有吸引力\n2. 长度适中\n3. 符合{{style}}风格', '[{"name":"content","type":"string","required":true},{"name":"count","type":"number","default":5},{"name":"style","type":"string","default":"专业"}]', '根据内容生成多个标题选项', TRUE, 1),
    ('reply', '客服回复生成', E'作为客服，请针对以下客户问题生成专业、友好的回复：\n\n客户问题：{{question}}\n产品信息：{{product_info}}\n\n要求：\n1. 语气友好专业\n2. 解决客户问题\n3. 必要时提供解决方案', '[{"name":"question","type":"string","required":true},{"name":"product_info","type":"string","required":false}]', '生成专业的客服回复', TRUE, 1),
    ('rewrite', '内容改写', E'请将以下内容改写，要求：\n\n原文：{{content}}\n改写风格：{{style}}\n改写目的：{{purpose}}\n\n注意：\n1. 保持原意不变\n2. 语言更加{{style}}\n3. 适合{{purpose}}场景', '[{"name":"content","type":"string","required":true},{"name":"style","type":"string","default":"简洁明了"},{"name":"purpose","type":"string","default":"社交媒体发布"}]', '按指定风格改写内容', TRUE, 1),
    ('social_post', '社交媒体帖子', E'请为{{platform}}平台生成一条帖子：\n\n主题：{{topic}}\n目标：{{goal}}\n\n要求：\n1. 符合{{platform}}平台特点\n2. 吸引用户互动\n3. 包含合适的表情符号\n4. 字数控制在{{word_count}}字以内', '[{"name":"platform","type":"string","required":true},{"name":"topic","type":"string","required":true},{"name":"goal","type":"string","default":"增加品牌曝光"},{"name":"word_count","type":"number","default":200}]', '生成社交媒体平台帖子', TRUE, 1),
    ('ad_copy', '广告文案', E'请为以下产品生成广告文案：\n\n产品名称：{{product_name}}\n产品特点：{{features}}\n目标人群：{{target_audience}}\n投放平台：{{platform}}\n\n要求：\n1. 突出产品核心卖点\n2. 吸引目标用户点击\n3. 包含行动号召\n4. 字数{{word_count}}字左右', '[{"name":"product_name","type":"string","required":true},{"name":"features","type":"string","required":true},{"name":"target_audience","type":"string","required":true},{"name":"platform","type":"string","default":"微信"},{"name":"word_count","type":"number","default":100}]', '生成广告投放文案', TRUE, 1),
    ('script', '销售话术', E'请生成一段销售话术：\n\n产品/服务：{{product}}\n客户类型：{{customer_type}}\n销售场景：{{scenario}}\n\n要求：\n1. 开场白吸引注意\n2. 突出产品价值\n3. 处理常见异议\n4. 引导成交', '[{"name":"product","type":"string","required":true},{"name":"customer_type","type":"string","default":"潜在客户"},{"name":"scenario","type":"string","default":"电话销售"}]', '生成销售场景话术', TRUE, 1),
    ('email', '邮件撰写', E'请撰写一封邮件：\n\n邮件类型：{{email_type}}\n收件人：{{recipient}}\n主要内容：{{content}}\n语气：{{tone}}\n\n要求：\n1. 邮件主题明确\n2. 内容简洁专业\n3. 语气{{tone}}', '[{"name":"email_type","type":"string","required":true},{"name":"recipient","type":"string","required":true},{"name":"content","type":"string","required":true},{"name":"tone","type":"string","default":"正式专业"}]', '生成各类邮件内容', TRUE, 1)
ON CONFLICT (type, name) DO UPDATE SET
    template = EXCLUDED.template,
    variables = EXCLUDED.variables,
    description = EXCLUDED.description;
