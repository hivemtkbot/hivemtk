-- =============================================================================
-- Migration 049: 自定义角色 v3.1（OPT-UX-07）
-- 2026-08-16
--
-- 目标：支持可视化创建自定义角色，覆盖
--   1. 菜单权限（router path 列表）
--   2. 按钮权限（action code 列表）
--   3. 数据范围（all/dept/self/custom）
--
-- 表设计：
--   role_definition   - 自定义角色定义
--   role_permission  - 角色-权限关联（菜单 + 按钮）
--   role_data_scope  - 角色-数据范围关联
--
-- 兼容性：
--   - v3.1 之前的 system_users.role 字段保留 admin/customer_service/staff 三个值
--   - 启用自定义角色后，user_role 表存 role_id（外键 role_definition）
--   - 现有 system_users.role 兼容：迁移时自动建一个 role_definition
-- =============================================================================

BEGIN;

-- 1) role_definition 自定义角色定义表
CREATE TABLE IF NOT EXISTS role_definition (
    id              BIGSERIAL PRIMARY KEY,
    role_code       VARCHAR(64) UNIQUE NOT NULL,       -- 业务编码，如 "marketing_manager"
    name            VARCHAR(100) NOT NULL,             -- 显示名
    description     VARCHAR(500) DEFAULT '',
    is_system       BOOLEAN NOT NULL DEFAULT FALSE,    -- 系统内置不可删
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,            -- 排序
    color           VARCHAR(20) DEFAULT '',            -- 卡片颜色
    icon            VARCHAR(50) DEFAULT '',            -- 卡片图标
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_role_definition_code ON role_definition (role_code);
CREATE INDEX IF NOT EXISTS idx_role_definition_enabled ON role_definition (enabled);
CREATE INDEX IF NOT EXISTS idx_role_definition_deleted ON role_definition (deleted_at);

-- 2) role_permission 角色-权限关联
CREATE TABLE IF NOT EXISTS role_permission (
    id              BIGSERIAL PRIMARY KEY,
    role_id         BIGINT NOT NULL REFERENCES role_definition(id) ON DELETE CASCADE,
    permission_type VARCHAR(20) NOT NULL,              -- 'menu' | 'button'
    permission_code VARCHAR(200) NOT NULL,             -- 菜单 path 或 button code
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(role_id, permission_type, permission_code)
);
CREATE INDEX IF NOT EXISTS idx_role_permission_role ON role_permission (role_id);
CREATE INDEX IF NOT EXISTS idx_role_permission_type ON role_permission (permission_type);
CREATE INDEX IF NOT EXISTS idx_role_permission_code ON role_permission (permission_code);

-- 3) role_data_scope 角色-数据范围
CREATE TABLE IF NOT EXISTS role_data_scope (
    id              BIGSERIAL PRIMARY KEY,
    role_id         BIGINT NOT NULL REFERENCES role_definition(id) ON DELETE CASCADE,
    scope_type      VARCHAR(20) NOT NULL,              -- 'all'|'dept'|'self'|'custom'
    custom_dept_ids JSONB DEFAULT '[]'::jsonb,         -- scope_type=custom 时使用
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(role_id)
);
CREATE INDEX IF NOT EXISTS idx_role_data_scope_role ON role_data_scope (role_id);

-- 4) user_role 用户-角色关联（支持多角色 + 自定义角色）
CREATE TABLE IF NOT EXISTS user_role (
    id              BIGSERIAL PRIMARY KEY,
    user_id         VARCHAR(64) NOT NULL,              -- 关联 system_users.id
    role_id         BIGINT NOT NULL REFERENCES role_definition(id) ON DELETE CASCADE,
    is_primary      BOOLEAN NOT NULL DEFAULT FALSE,    -- 主角色
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, role_id)
);
CREATE INDEX IF NOT EXISTS idx_user_role_user ON user_role (user_id);
CREATE INDEX IF NOT EXISTS idx_user_role_role ON user_role (role_id);

-- 5) 初始化 3 个系统角色（与 v3.1 之前的 model/role.go 对齐）
INSERT INTO role_definition (role_code, name, description, is_system, sort_order, color, icon)
VALUES
  ('admin',           '超管',     '拥有全部权限，可管理账号/角色/授权', TRUE,  1, '#f56c6c', 'Lock'),
  ('customer_service','客服',     '负责客户沟通、订单处理、智能体协同', TRUE, 2, '#e6a23c', 'Service'),
  ('staff',           '员工',     '负责内容编辑、数据分析、运营等日常工作', TRUE, 3, '#909399', 'User')
ON CONFLICT (role_code) DO NOTHING;

-- 6) 系统角色数据范围（admin=all, 客服=self, 员工=self）
INSERT INTO role_data_scope (role_id, scope_type)
SELECT id, CASE
    WHEN role_code = 'admin' THEN 'all'
    ELSE 'self'
END
FROM role_definition
WHERE is_system = TRUE
ON CONFLICT (role_id) DO NOTHING;

-- 7) 现有 system_users 数据迁移
--    把 user.role 字符串映射到 user_role 表
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'system_users') THEN
    INSERT INTO user_role (user_id, role_id, is_primary)
    SELECT su.id, rd.id, TRUE
    FROM system_users su
    JOIN role_definition rd
      ON LOWER(su.role) = LOWER(rd.role_code)
         OR (LOWER(su.role) = 'user' AND rd.role_code = 'staff')
    WHERE su.deleted_at IS NULL OR su.deleted_at IS NULL
    ON CONFLICT (user_id, role_id) DO NOTHING;
    RAISE NOTICE 'Migrated system_users.role to user_role';
  END IF;
END $$;

COMMIT;

-- =============================================================================
-- 验证：
--   SELECT * FROM role_definition ORDER BY sort_order;
--   SELECT u.username, r.name FROM system_users u
--   JOIN user_role ur ON u.id = ur.user_id
--   JOIN role_definition r ON ur.role_id = r.id
--   LIMIT 10;
-- =============================================================================
