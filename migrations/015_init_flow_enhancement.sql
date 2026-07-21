-- ============================================================
-- 015_init_flow_enhancement.sql
-- 系统初始化流程增强：must_change_password + 索引
-- 对应 MERCHANT_INITIALIZATION_FLOW.md
-- ============================================================

-- 1. system_users.must_change_password 字段（首次登录强制改密）
ALTER TABLE system_users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

-- 2. 索引（优化首次登录判定）
CREATE INDEX IF NOT EXISTS idx_system_users_must_change ON system_users(must_change_password) WHERE must_change_password = TRUE;
CREATE INDEX IF NOT EXISTS idx_system_users_status ON system_users(status);

-- 3. 注释
COMMENT ON COLUMN system_users.must_change_password IS '是否必须修改密码（首次登录强制改密，初始化流程使用）';
