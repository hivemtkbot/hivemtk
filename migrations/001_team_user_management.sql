-- 团队用户管理模块数据库迁移脚本
-- 版本: 1.1.0  (2026-07-17 改为 PostgreSQL 15+ 语法)
-- 适用于: PostgreSQL 15+ (项目唯一数据库)

-- 团队用户表
CREATE TABLE IF NOT EXISTS team_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    password VARCHAR(255) NOT NULL,
    name VARCHAR(50),
    email VARCHAR(100),
    phone VARCHAR(20),
    avatar VARCHAR(255),
    role VARCHAR(20) DEFAULT 'viewer',
    status INT DEFAULT 1,
    last_login_at TIMESTAMP,
    last_login_ip VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_team_users_status ON team_users (status);
COMMENT ON COLUMN team_users.username IS '用户名';
COMMENT ON COLUMN team_users.password IS '密码';
COMMENT ON COLUMN team_users.name IS '姓名';
COMMENT ON COLUMN team_users.email IS '邮箱';
COMMENT ON COLUMN team_users.phone IS '手机号';
COMMENT ON COLUMN team_users.avatar IS '头像';
COMMENT ON COLUMN team_users.role IS '角色: admin, manager, viewer';
COMMENT ON COLUMN team_users.status IS '状态: 0-禁用, 1-启用';
COMMENT ON COLUMN team_users.last_login_at IS '最后登录时间';
COMMENT ON COLUMN team_users.last_login_ip IS '最后登录IP';
COMMENT ON TABLE team_users IS '团队用户表';

-- updated_at 自动更新触发器
CREATE OR REPLACE FUNCTION team_users_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trg_team_users_updated_at ON team_users;
CREATE TRIGGER trg_team_users_updated_at
    BEFORE UPDATE ON team_users
    FOR EACH ROW EXECUTE FUNCTION team_users_set_updated_at();

-- 团队角色表
CREATE TABLE IF NOT EXISTS team_roles (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(20) NOT NULL,
    name VARCHAR(50) NOT NULL,
    permissions TEXT,
    is_system BOOLEAN DEFAULT FALSE,
    status INT DEFAULT 1,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_team_roles_code ON team_roles (code);
COMMENT ON COLUMN team_roles.code IS '角色编码';
COMMENT ON COLUMN team_roles.name IS '角色名称';
COMMENT ON COLUMN team_roles.permissions IS '权限列表(JSON)';
COMMENT ON COLUMN team_roles.is_system IS '是否系统角色';
COMMENT ON COLUMN team_roles.status IS '状态: 0-禁用, 1-启用';
COMMENT ON TABLE team_roles IS '团队角色表';

DROP TRIGGER IF EXISTS trg_team_roles_updated_at ON team_roles;
CREATE TRIGGER trg_team_roles_updated_at
    BEFORE UPDATE ON team_roles
    FOR EACH ROW EXECUTE FUNCTION team_users_set_updated_at();

-- 操作日志表
CREATE TABLE IF NOT EXISTS operation_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    username VARCHAR(50),
    action VARCHAR(20) NOT NULL,
    module VARCHAR(50) NOT NULL,
    resource VARCHAR(50),
    resource_id VARCHAR(50),
    detail TEXT,
    old_value TEXT,
    new_value TEXT,
    ip VARCHAR(50),
    user_agent VARCHAR(255),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_user ON operation_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_operation_logs_action ON operation_logs (action);
CREATE INDEX IF NOT EXISTS idx_operation_logs_module ON operation_logs (module);
COMMENT ON COLUMN operation_logs.user_id IS '操作用户ID';
COMMENT ON COLUMN operation_logs.username IS '操作用户名';
COMMENT ON COLUMN operation_logs.action IS '操作类型: create, update, delete, login, logout等';
COMMENT ON COLUMN operation_logs.module IS '模块: user, role, card, shortlink等';
COMMENT ON COLUMN operation_logs.resource IS '资源类型';
COMMENT ON COLUMN operation_logs.resource_id IS '资源ID';
COMMENT ON COLUMN operation_logs.detail IS '操作详情(JSON)';
COMMENT ON COLUMN operation_logs.old_value IS '旧值(JSON)';
COMMENT ON COLUMN operation_logs.new_value IS '新值(JSON)';
COMMENT ON COLUMN operation_logs.ip IS '操作IP';
COMMENT ON COLUMN operation_logs.user_agent IS '用户代理';
COMMENT ON TABLE operation_logs IS '操作日志表';

-- 插入系统默认角色 (使用 ON CONFLICT 处理幂等)
INSERT INTO team_roles (code, name, permissions, is_system, status) VALUES
    ('admin', '管理员', '["*"]', TRUE, 1),
    ('manager', '运营经理', '["cards.*","shortlinks.*","clues.*","autoreply.*"]', TRUE, 1),
    ('viewer', '查看者', '["cards.view","shortlinks.view","clues.view"]', TRUE, 1)
ON CONFLICT (code) DO UPDATE SET
    name = EXCLUDED.name,
    permissions = EXCLUDED.permissions;

-- 创建定时清理旧日志的扩展 (可选, 需要启用 pg_cron)
-- 实际项目使用 GORM 定时任务或外部调度
