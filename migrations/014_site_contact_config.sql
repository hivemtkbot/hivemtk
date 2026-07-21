-- ============================================================
-- 014_site_contact_config.sql
-- 官网联系信息配置表（platform_db）
--
-- 用途：
--   - 平台管理员通过平台端管理联系信息（微信号、邮箱、电话、二维码 URL）
--   - 官网通过公开 API 读取并展示
--   - 同一份官网代码不同环境可以显示不同联系信息
--
-- 数据模型：单行配置（id 固定为 'default'），保证只存在一份
-- 写入：仅平台管理员
-- 读取：公开 API（无需鉴权）
-- ============================================================

CREATE TABLE IF NOT EXISTS site_contact_config (
    id VARCHAR(36) PRIMARY KEY DEFAULT 'default',
    -- 微信号（用户复制添加好友用）
    wechat_id VARCHAR(100) NOT NULL DEFAULT '',
    -- 微信二维码图片 URL（建议 CDN/对象存储路径，空则官网显示占位）
    wechat_qr_url TEXT NOT NULL DEFAULT '',
    -- 客服邮箱
    email VARCHAR(120) NOT NULL DEFAULT '',
    -- 客服电话
    phone VARCHAR(40) NOT NULL DEFAULT '',
    -- 商务联系企业微信号（区别于个人微信，可选）
    business_wechat_id VARCHAR(100) NOT NULL DEFAULT '',
    -- 客户案例 / 客户 Logo 列表（JSON 数组）
    customer_logos JSONB NOT NULL DEFAULT '[]'::jsonb,
    -- 服务时间描述
    service_hours VARCHAR(200) NOT NULL DEFAULT '工作日 09:00-18:00',
    -- 备注（供平台管理员查看，对用户不可见）
    note TEXT NOT NULL DEFAULT '',
    -- 维护人 / 团队
    owner VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_site_contact_id CHECK (id = 'default')
);

-- 插入默认行（首次启动时使用）
INSERT INTO site_contact_config (id, wechat_id, wechat_qr_url, email, phone, owner, note)
VALUES (
    'default',
    'your-wechat-id',
    '',
    'support@ai-sales-champion.local',
    '',
    '开通顾问',
    '默认联系信息，请通过平台端「系统设置 → 官网联系信息」修改'
) ON CONFLICT (id) DO NOTHING;

-- 触发器：自动更新 updated_at
CREATE OR REPLACE FUNCTION trg_site_contact_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_site_contact_updated ON site_contact_config;
CREATE TRIGGER trg_site_contact_updated
BEFORE UPDATE ON site_contact_config
FOR EACH ROW
EXECUTE FUNCTION trg_site_contact_updated_at();
