-- 客服子功能增强迁移
-- 版本: 1.2.0
-- 适用于: PostgreSQL 15+
-- 说明: 补齐快捷回复的「适用渠道」与会话标签的「标识/分组/说明」，
--       使网页端坐席工作台(快捷回复/会话标签)前后端字段完全闭环。

-- 1) 快捷回复增加「适用渠道」列 (空=通用)
ALTER TABLE quick_replies ADD COLUMN IF NOT EXISTS channel VARCHAR(20);
COMMENT ON COLUMN quick_replies.channel IS '适用渠道: 空=通用 whatsapp/wecom/feishu/telegram/email/sms';

-- 2) 会话标签增加「标识/分组/说明」
ALTER TABLE session_tags ADD COLUMN IF NOT EXISTS code VARCHAR(50);
ALTER TABLE session_tags ADD COLUMN IF NOT EXISTS "group" VARCHAR(50);
ALTER TABLE session_tags ADD COLUMN IF NOT EXISTS description VARCHAR(200);
COMMENT ON COLUMN session_tags.code IS '英文/拼音标识，如 vip';
COMMENT ON COLUMN session_tags."group" IS '分组: 客户类型/意向度';
COMMENT ON COLUMN session_tags.description IS '说明';

-- 唯一约束: code 允许为空(历史数据), 多个 NULL 在 PostgreSQL 中不违反 UNIQUE
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_session_tags_code'
    ) THEN
        ALTER TABLE session_tags ADD CONSTRAINT uq_session_tags_code UNIQUE (code);
    END IF;
END $$;

-- 3) 为历史默认标签回填 code (按 name 映射, 幂等)
UPDATE session_tags SET code = 'consult'    WHERE name = '咨询'  AND code IS NULL;
UPDATE session_tags SET code = 'complaint'  WHERE name = '投诉'  AND code IS NULL;
UPDATE session_tags SET code = 'aftersale'  WHERE name = '售后'  AND code IS NULL;
UPDATE session_tags SET code = 'urge'       WHERE name = '催单'  AND code IS NULL;
UPDATE session_tags SET code = 'resolved'   WHERE name = '已解决' AND code IS NULL;
