-- 027_user_blacklist.sql
-- 方向10：坐席实时聊天看板 - 用户黑名单表（user_id 维度 + TTL + 软删除）
--
-- 修复 bug：GET /api/customer-sessions/blacklist 返回 500
--   ERROR: relation "user_blacklist" does not exist (SQLSTATE 42P01)
--
-- 设计要点：
--   1. 以 user_id 维度拉黑（非单次会话），避免该访客后续再次接入
--   2. 软删除：active=false 保留历史，便于审计与解除
--   3. reason / source 记录拉黑原因与来源（坐席手动 manual / 风控自动 auto / risk）
--   4. expires_at 支持临时拉黑（NULL = 永久）
--   5. 复合索引 (user_id, platform, active) 加速黑名单校验查询
--
-- 关联模型：internal/model/user_blacklist.go (UserBlacklist)
-- 关联服务：internal/service/customer_session_blacklist.go
-- 关联路由：GET /api/customer-sessions/blacklist (ListActiveBlacklist)

CREATE TABLE IF NOT EXISTS user_blacklist (
    id            BIGSERIAL    PRIMARY KEY,
    user_id       VARCHAR(50)  NOT NULL,
    platform      VARCHAR(20)  NOT NULL DEFAULT 'web',
    reason        VARCHAR(500),
    source        VARCHAR(50)  NOT NULL DEFAULT 'manual',
    operator_id   BIGINT       NOT NULL DEFAULT 0,
    operator_name VARCHAR(100),
    session_id    VARCHAR(50),
    active        BOOLEAN      NOT NULL DEFAULT TRUE,
    expires_at    TIMESTAMP,
    created_at    TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMP    NOT NULL DEFAULT NOW()
);

-- 单字段索引：加速按 user_id / platform / active 的独立查询
CREATE INDEX IF NOT EXISTS idx_user_blacklist_user_id   ON user_blacklist (user_id);
CREATE INDEX IF NOT EXISTS idx_user_blacklist_platform  ON user_blacklist (platform);
CREATE INDEX IF NOT EXISTS idx_user_blacklist_active    ON user_blacklist (active);
CREATE INDEX IF NOT EXISTS idx_user_blacklist_operator  ON user_blacklist (operator_id);
CREATE INDEX IF NOT EXISTS idx_user_blacklist_session   ON user_blacklist (session_id);

-- 复合索引：加速 IsBlacklisted(user_id, platform) 校验（仅查 active 记录）
CREATE INDEX IF NOT EXISTS idx_user_blacklist_user_platform_active
    ON user_blacklist (user_id, platform, active);
