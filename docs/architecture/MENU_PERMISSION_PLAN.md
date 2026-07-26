# User-Web（商户端）单点登录 + 单表用户化 实施计划

> 计划目标：**统一单用户表 + 启用/禁用二元管控 + 三模块管理** —— 所有账号（含超管、客服）全部存储在 `system_users`；彻底清理历史 `team_users` / `team_roles` 双轨制。**user-web 是商户端（单租户、单角色），不引入菜单权限树，不做精细化授权**；超管侧提供 3 个独立管理模块：**人员管理 / 角色管理 / 授权管理**。
> 关联文档：
> - 五层架构：[GO_FIVE_LAYER_ARCHITECTURE.md](./GO_FIVE_LAYER_ARCHITECTURE.md)
> - 用户记忆硬约束：开源自部署、超管默认全权限、菜单不控制到按钮、密码入库

---

## 〇、需求边界（v3.1 收口）

| 维度 | 范围 |
| --- | --- |
| 控制粒度 | **不需要菜单级权限**（v2.0 方案取消）；仅"账号是否启用 / 是否允许登录"二元管控 |
| 角色值 | **简化**：`admin` / `customer_service` / `staff` 三档（v2.0 的 5 档收口为 3 档；其他业务角色按业务需求再增） |
| 超管权限 | 安装时创建的超管**默认拥有全部页面访问权**（前端不拦截 + 后端不拦截；仅靠"账号存在 + 启用"判断） |
| 客服/管理 密码 | 全部**保存到数据库**（bcrypt 加密），不再依赖配置文件 / 环境变量 |
| **用户表统一** | **所有账号仅存于 `system_users` 表**；不再保留 `team_users` / `team_roles` |
| **三管理模块** | 团队（超管可见）下设 3 个独立模块：① 人员管理 ② 角色管理 ③ 授权管理 |
| 不做的事 | ❌ 不做菜单树、❌ 不做权限点、❌ 不做 my-menus、❌ 不做 role-menus、❌ 不做 MenuPermission 视图、❌ 不做菜单级中间件 |
| 涉及端 | `user-web` (前端) + `user-server` (后端) |

### 三管理模块职责

| 模块 | 路径 | 职责 | 操作 |
| --- | --- | --- | --- |
| **人员管理** | `/system/users` | 账号 CRUD | 新建 / 编辑 / 删除 / 启停 / 改密 |
| **角色管理** | `/system/roles` | 3 档系统角色只读视图 | 查看角色定义 / 查看角色下成员列表 |
| **授权管理** | `/system/permissions` | 启停 + 改密 + 操作审计 | 启停 / 改密 / 查看操作日志（谁启停/改密了谁） |

> **三者关系**：人员管理是 CRUD 主入口；角色管理是只读视图（展示 system_users 分布）；授权管理是高频操作快捷面板（不需进入列表页直接对某账号操作）+ 审计追溯。

---

## 三、设计方案

### 3.0 核心原则：单表 + 二元管控 + 三模块管理

> **所有账号（超管、客服、员工）全部存在 `system_users` 一张表**。`role` 字段值域（v3.1 收口为 3 档）：
>
> | role 值 | 业务语义 | 备注 |
> | --- | --- | --- |
> | `admin` | 超管（默认全权限） | 安装初始化时创建；**不可被删除（系统至少保留 1 个）** |
> | `customer_service` | 客服 | 由 admin 创建/启用/禁用 |
> | `staff` | 普通员工 | 由 admin 创建/启用/禁用 |
>
> **二元管控**：
> 1. `enabled`（BOOL）：账号是否启用 → 禁用后无法登录
> 2. 角色值：决定前端入口可见性（仅"团队整个一级菜单"用 role 判断）
>
> **三管理模块**（超管专属，UI 上独立菜单项）：
>
> | 模块 | 后端入口 | 前端路由 | 数据源 |
> | --- | --- | --- | --- |
> | **人员管理** | `/api/system/users/*` | `/system/users` | `system_users` 表 |
> | **角色管理** | `/api/system/roles/*` | `/system/roles` | 3 档常量 + `system_users` 统计 |
> | **授权管理** | `/api/system/permissions/*` | `/system/permissions` | `system_users` + `operation_logs`（审计） |

### 3.1 数据模型变更

#### 3.1.0 `system_users` 表扩展（v1 → v3 升级）

```sql
-- 修改 role 字段 CHECK 约束
ALTER TABLE system_users
  DROP CONSTRAINT IF EXISTS system_users_role_check;
ALTER TABLE system_users
  ADD CONSTRAINT system_users_role_check
  CHECK (role IN ('admin','customer_service','staff'));

-- 新增"账号是否启用"字段（语义清晰；替代 v1 的 status 1/0）
ALTER TABLE system_users
  ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- 初始化：所有现存账号默认启用
UPDATE system_users SET enabled = TRUE WHERE enabled IS NULL;

-- 兼容：保留 v1 的 status 字段（不删，避免破坏既有审计/日志引用）
-- 后续版本再清理
```

#### 3.1.1 删除历史遗留表（迁移完成后）

```sql
-- 1) team_users 数据迁移到 system_users（bcrypt 密文直接复用，零数据丢失）
INSERT INTO system_users (username, password, name, email, phone, avatar, role, status, last_login_at, last_login_ip, created_at, updated_at)
SELECT
  username, password, name, email, phone, avatar,
  CASE role WHEN 'admin' THEN 'admin' WHEN 'manager' THEN 'staff' WHEN 'viewer' THEN 'staff' ELSE 'staff' END,
  status, last_login_at, last_login_ip, created_at, updated_at
FROM team_users
ON CONFLICT (username) DO NOTHING;

-- 2) DROP 历史表（迁移前必须先备份）
DROP TABLE IF EXISTS team_user_permissions CASCADE;
DROP TABLE IF EXISTS team_roles CASCADE;
DROP TABLE IF EXISTS team_users CASCADE;
```

> **关键点**：bcrypt 密码原文不动，跨表迁移后**客服/老用户可直接登录**。
> 注意：v3.0 把原 team_users 的 manager/viewer 合并为 staff（user-web 是商户端，不需要精细角色）。

#### 3.1.2 修改 `install.lock`（文件）
```json
{
  "install_id": "ins-...",
  "install_time": "...",
  "admin_user_id": 1,
  "admin_username": "admin",
  "initialized": true,
  "version": "..."
}
```

### 3.2 接口设计（v3.1 三模块版）

#### 模块 ① 人员管理（`/api/system/users/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/system/users` | JWT admin | system_users 列表（**替代原 /api/team/users**） |
| GET | `/api/system/users/:id` | JWT admin | 单个账号详情 |
| POST | `/api/system/users` | JWT admin | 创建新账号（admin 选 role：customer_service / staff） |
| PUT | `/api/system/users/:id` | JWT admin | 更新账号（username/email/role） |
| DELETE | `/api/system/users/:id` | JWT admin | 删除账号（**禁止删除最后一个 admin**） |

#### 模块 ② 角色管理（`/api/system/roles/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/system/roles` | JWT admin | 列出 3 档系统角色（**只读**） |
| GET | `/api/system/roles/:code` | JWT admin | 单个角色详情（包含成员数） |
| GET | `/api/system/roles/:code/members` | JWT admin | 该角色下所有成员列表（分页） |

#### 模块 ③ 授权管理（`/api/system/permissions/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| PUT | `/api/system/permissions/:id/enabled` | JWT admin | 启用/禁用账号（高频操作） |
| PUT | `/api/system/permissions/:id/password` | JWT admin | 重置密码（强密码校验） |
| GET | `/api/system/permissions/audit-logs` | JWT admin | 操作审计日志（启停/改密/创建/删除） |
| GET | `/api/system/permissions/audit-logs/export` | JWT admin | 导出审计日志（CSV） |

#### 通用接口

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/system/init-admin`（**重写**） | 公开 | 入参强校验 `username/password/email`；不再读 config |
| POST | `/api/auth/login` | 公开 | **单表登录**，检查 `enabled=true` |
| GET | `/api/auth/current-user` | JWT | 返回当前用户信息（含 role、enabled） |
| POST | `/api/auth/change-password` | JWT | 自己改密 |

### 3.3 登录响应变更

```jsonc
{
  "token": "eyJhbGciOi...",
  "user": {
    "id": 1,
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin",
    "enabled": true
  },
  "expires": 1753267200
}
```

### 3.4 前端关键改造

| 位置 | 改造点 |
| --- | --- |
| `Layout.vue` | **保留硬编码菜单**；**整个"团队"一级菜单** `v-if="isAdmin"`（子模块不再单独判）；下设 3 个子菜单项：人员管理 / 角色管理 / 授权管理 |
| `stores/user.js` | 保留 `role` 字段；增加 `isAdmin` 计算属性（`role === 'admin'`） |
| `router/index.js` | 在 `beforeEach` 中若 `to.meta.requiresAdmin && !isAdmin` → 重定向 `/`；3 个子模块路由都加 `requiresAdmin: true` |
| 新建 `views/system/UserList.vue` | **人员管理**：列表 + 新建/编辑/删除 + 启停 + 改密 |
| 新建 `views/system/RoleList.vue` | **角色管理**：3 档角色卡片（admin/customer_service/staff）；每张卡片显示角色名/描述/成员数；点击"查看成员"跳到子页面列出该角色下所有账号 |
| 新建 `views/system/PermissionPanel.vue` | **授权管理**：快捷操作面板（输入 username 快速启停/改密）+ 操作审计日志表格 |
| `views/setup/InitSetup.vue` | 发送 `{username, password, email}` 而非依赖后端默认 |
| **删除** | `teamUser/List.vue` / `teamUser/Role.vue` / `api/teamUser.js` / `router/modules/teamUser.js` |
| **清理** | i18n 4 个语言文件中 teamUser.* / teamRole.* 文案 |

### 3.5 初始化流程重写（v3.1）

```
[前端 /setup]
   ↓
1) 探测：GET /api/system/init-status
   ↓ state == NOT_INSTALLED
2) 用户填表：username / password / email
   ↓
3) POST /api/system/init-admin { username, password, email }
   - service 层：bcrypt 加密 → 写入 system_users (role='admin', enabled=true)
   - install.MarkAdminInitialized(user_id, username)
   ↓
4) 前端跳 /login，正常登录（POST /api/auth/login，单表查询 + enabled 检查）
```

清理动作：
- 删除 `config/admin.go` 中 `DefaultAdmin.Password` 默认值；`GetAdminConfig()` 移除 `default_admin` 段
- 删除 `controller/auth.go#CreateDefaultAdmin`（重写为 `initAdmin`）
- 删除 `auth_routes.go#create-default-admin` 路由
- 清理 `.env/admin.json` 示例文件
- **删除 `controller/team_user.go` 全部端点（含 Login）**
- **删除 `setupTeamRoutes` 路由组**
- **删除 `ManagerOrAdminMiddleware` 中所有引用**

---

## 十一、编码规范特别要求

### 11.1 强约束（来自项目记忆）

1. **五层架构严格遵守**：[GO_FIVE_LAYER_ARCHITECTURE.md](./GO_FIVE_LAYER_ARCHITECTURE.md)
2. **禁止命名**：`utils.go` / `common.go` / `*_v1.go` / `*_stub.go` / `*_2026-*.go` / `*_ext*`
3. **禁止目录**：`utils/` / `common/` 包（如果只是零散函数，外迁到 `pkg/utils/` 下具名包）
4. **禁止 import cycle**：service 包与 tooluse 包之间不能循环依赖
5. **代码规范优先** > 预调查 > 测试规范 > 代码自测 > UI 美化

### 11.2 五层架构 AI 自检清单（编码前必须阅读）

[GO_FIVE_LAYER_ARCHITECTURE.md §七](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/docs/architecture/GO_FIVE_LAYER_ARCHITECTURE.md) 列出了完整的 AI 自检清单。编码前**必须**通读。

### 11.3 推荐工具命令

```bash
# 编译检查
go build -o /dev/null ./...

# 架构合规检查
bash scripts/check-architecture.sh

# 命名违规检查
grep -rn "utils\.go\|common\.go\|_v1\.go\|_stub\.go" \
  --include="*.go" internal/

# 残留检查
grep -rn "team_user\|team_role\|TeamUser" \
  --include="*.go" --include="*.sql" --include="*.vue" --include="*.js" \
  user-server/ user-web/ migrations/
```

### 11.4 测试要求（来自项目记忆）

1. 三个测试阶段：
   - 第 1 轮：curl 测试所有 API
   - 第 2 轮：检查所有页面和 API 交互（参数/路由/响应/字段）
   - 第 3 轮：打开页面，模拟多角色 UI 点击
2. 测试脚本应能独立报告错误，用于回归测试
3. 每个新接口至少 5 个测试用例
4. 必须在 100% 修复后再交付，不允许任何跳过/异常处理

### 11.5 数据迁移要求（来自项目记忆）

- 不能以"脏数据"为理由跳过数据
- 需要反复验证
- 迁移前 pg_dump 备份，保留 30 天
- 双向回滚 SQL 提前生成

### 11.6 任务执行要求（来自项目记忆）

- 一个一个功能完善，**绝对不能批量操作**
- 自动进入下一步
- 不向用户询问确认（除非有歧义或阻塞）
- 用 sub-agent 并行执行避免 thinking 时间超限
- 代码完成后立即 commit（Git 纪律）
