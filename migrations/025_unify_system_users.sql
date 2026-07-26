-- ============================================================
-- Migration 025: 统一 system_users 表 + 启用/禁用管控
-- 关联文档：docs/architecture/MENU_PERMISSION_PLAN.md
-- ============================================================

-- 1) team_users 与 system_users 的 username 冲突预检测
DO $$
DECLARE
    conflict_count INT;
BEGIN
    SELECT COUNT(*) INTO conflict_count
    FROM team_users t
    WHERE t.username IN (SELECT s.username FROM system_users s)
      AND t.deleted_at IS NULL;
    IF conflict_count > 0 THEN
        RAISE NOTICE '检测到 % 条 username 冲突，迁移将优先保留 system_users', conflict_count;
    END IF;
END $$;

-- 2) DROP 现有 role CHECK 约束（如存在）
ALTER TABLE system_users DROP CONSTRAINT IF EXISTS system_users_role_check;

-- 3) 数据迁移：team_users → system_users
--    条件：deleted_at IS NULL AND status = 1
--    冲突处理：ON CONFLICT (username) DO NOTHING
INSERT INTO system_users (
    username, password, name, email, phone, avatar, role, status,
    last_login_at, last_login_ip, created_at, updated_at
)
SELECT
    t.username,
    t.password,                                   -- bcrypt 密文直接复用
    t.name,
    t.email,
    t.phone,
    t.avatar,
    CASE t.role
        WHEN 'admin' THEN 'admin'
        WHEN 'manager' THEN 'staff'
        WHEN 'viewer' THEN 'staff'
        ELSE 'staff'
    END,
    t.status,
    t.last_login_at,
    t.last_login_ip,
    t.created_at,
    t.updated_at
FROM team_users t
WHERE t.deleted_at IS NULL
  AND t.status = 1
ON CONFLICT (username) DO NOTHING;

-- 4) 新增 enabled 列（默认 TRUE）
ALTER TABLE system_users
    ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- 5) 初始化 enabled 字段（兼容旧 status）
UPDATE system_users SET enabled = (status = 1) WHERE enabled IS NULL OR enabled = TRUE;

-- 6) 重新加 role CHECK 约束（3 档：admin/customer_service/staff）
ALTER TABLE system_users
    ADD CONSTRAINT system_users_role_check
    CHECK (role IN ('admin','customer_service','staff'));

-- 7) 加索引（提升 role/enabled 查询性能）
CREATE INDEX IF NOT EXISTS idx_system_users_role ON system_users(role);
CREATE INDEX IF NOT EXISTS idx_system_users_enabled ON system_users(enabled);

-- 8) operation_logs.user_id 外键重映射（旧 team_users.id → 新 system_users.id）
UPDATE operation_logs ol
SET user_id = su.id
FROM team_users tu, system_users su
WHERE ol.user_id = tu.id
  AND tu.username = su.username
  AND tu.deleted_at IS NULL;

-- 9) DROP 历史表（team_user_permissions 在前，因为它有外键依赖 team_users）
DROP TABLE IF EXISTS team_user_permissions CASCADE;
DROP TABLE IF EXISTS team_roles CASCADE;
DROP TABLE IF EXISTS team_users CASCADE;

-- 10) 删除旧 migration 引用（不改 .sql 文件，只标记）
--     mvp/001_team_user_management.sql 由后续 sub-agent 在阶段 8 删除
