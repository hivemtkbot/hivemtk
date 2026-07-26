# USER_SYSTEM.md · 用户体系规范

> **关联文档**：
> - [MENU_PERMISSION_PLAN.md](./MENU_PERMISSION_PLAN.md)（设计源）
> - [GO_FIVE_LAYER_ARCHITECTURE.md](./GO_FIVE_LAYER_ARCHITECTURE.md)（五层架构）
> - [ARCHITECTURE_DIAGRAM.md](./ARCHITECTURE_DIAGRAM.md)（架构图）

本文档规范「人员 / 角色 / 授权」三模块的全部接口、数据、部署与运维约束，是 **唯一权威**。后续任何关于 system_users 的修改都必须先更新本文。

---

## 一、目标与原则

- **单表化**：所有账号统一落在 `system_users` 一张表，**不再保留 `team_users` / `team_roles` 历史表**。
- **二元管控**：账号只有「启用 / 禁用」两个状态，角色固定 3 档，**无自定义角色、无细粒度权限表**。
- **三模块管理**：人员 / 角色 / 授权在 UI、API、中间件三个层面完全分离。
- **后端兜底**：所有需要 admin 的路由**必须**挂在 `RequireAdminMiddleware()` 之下，前端 `v-if` 只是体验优化，**不可作为安全屏障**。

---

## 二、数据模型

### 2.1 `system_users` 表结构（v3.1 当前形态）

| 字段 | 类型 | 必填 | 默认 | 说明 |
| --- | --- | --- | --- | --- |
| `id` | `BIGSERIAL` | ✓ | auto | 主键 |
| `username` | `VARCHAR(50)` UNIQUE | ✓ | - | 登录账号 |
| `password` | `VARCHAR(255)` | ✓ | - | bcrypt 密文 |
| `name` | `VARCHAR(50)` |  | NULL | 姓名 |
| `email` | `VARCHAR(100)` |  | NULL | 邮箱 |
| `phone` | `VARCHAR(20)` |  | NULL | 手机号 |
| `avatar` | `VARCHAR(255)` |  | NULL | 头像 URL |
| `role` | `VARCHAR(20)` CHECK | ✓ | - | 枚举：`admin` / `customer_service` / `staff` |
| `enabled` | `BOOLEAN` | ✓ | TRUE | 是否启用 |
| `status` | `SMALLINT` |  | 1 | 兼容 v1 字段（保留，不删） |
| `data_scope` | `VARCHAR(20)` |  | `self` | P1-4 行级权限：`all` / `department` / `team` / `self` |
| `department_id` | `BIGINT` |  | 0 | P1-4 所属部门 |
| `team_id` | `BIGINT` |  | 0 | P1-4 所属团队 |
| `last_login_at` | `TIMESTAMP` |  | NULL | 上次登录时间 |
| `last_login_ip` | `VARCHAR(45)` |  | NULL | 上次登录 IP |
| `created_at` | `TIMESTAMP` | ✓ | NOW() | 创建时间 |
| `updated_at` | `TIMESTAMP` | ✓ | NOW() | 更新时间 |

约束：
- `role` 字段 CHECK：`role IN ('admin','customer_service','staff')`
- `data_scope` 字段 CHECK（应用层）：`data_scope IN ('all','department','team','self')`
- 唯一索引：`uk_system_users_username(username)`

### 2.2 三档角色定义

| Code | 中文 | 英文 | 描述 |
| --- | --- | --- | --- |
| `admin` | 超级管理员 | Admin | 全部权限，唯一可管理账号的角色 |
| `customer_service` | 客服 | Customer Service | 客服工作台 + 触达运营 + 客户中心 |
| `staff` | 员工 | Staff | 业务运营基础角色（原 manager/viewer 合并） |

> 角色**只读**，不允许用户创建自定义角色。

### 2.3 数据迁移（`migrations/025_unify_system_users.sql`）

```sql
-- v1 -> v3 升级
ALTER TABLE system_users
  DROP CONSTRAINT IF EXISTS system_users_role_check;
ALTER TABLE system_users
  ADD CONSTRAINT system_users_role_check
  CHECK (role IN ('admin','customer_service','staff'));

ALTER TABLE system_users
  ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;

UPDATE system_users SET enabled = TRUE WHERE enabled IS NULL;

-- team_users -> system_users（bcrypt 密文直接复用）
INSERT INTO system_users (username, password, name, email, phone, avatar, role, status, last_login_at, last_login_ip, created_at, updated_at)
SELECT
  username, password, name, email, phone, avatar,
  CASE role WHEN 'admin' THEN 'admin' WHEN 'manager' THEN 'staff' WHEN 'viewer' THEN 'staff' ELSE 'staff' END,
  status, last_login_at, last_login_ip, created_at, updated_at
FROM team_users
ON CONFLICT (username) DO NOTHING;

DROP TABLE IF EXISTS team_user_permissions CASCADE;
DROP TABLE IF EXISTS team_roles CASCADE;
DROP TABLE IF EXISTS team_users CASCADE;
```

---

## 三、API 接口规范

### 3.1 模块 ① 人员管理（`/api/system/users/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/system/users` | JWT admin | system_users 列表（分页 `page` / `size`） |
| GET | `/api/system/users/:id` | JWT admin | 单个账号详情 |
| POST | `/api/system/users` | JWT admin | 创建新账号（admin 选 role：customer_service / staff） |
| PUT | `/api/system/users/:id` | JWT admin | 更新账号（username/email/role） |
| DELETE | `/api/system/users/:id` | JWT admin | 删除账号（**禁止删除最后一个 admin**） |

入参样例（创建）：
```json
{
  "username": "alice",
  "password": "Strong#Pass1",
  "email": "alice@example.com",
  "name": "Alice",
  "role": "customer_service"
}
```

### 3.2 模块 ② 角色管理（`/api/system/roles/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| GET | `/api/system/roles` | JWT admin | 列出 3 档系统角色（含成员数） |
| GET | `/api/system/roles/:code` | JWT admin | 单个角色详情 |
| GET | `/api/system/roles/:code/members` | JWT admin | 该角色下所有成员列表（分页） |

响应样例（角色列表）：
```json
{
  "code": 0,
  "data": [
    {"code": "admin", "name": "超级管理员", "member_count": 2},
    {"code": "customer_service", "name": "客服", "member_count": 5},
    {"code": "staff", "name": "员工", "member_count": 8}
  ]
}
```

### 3.3 模块 ③ 授权管理（`/api/system/permissions/*`）

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| PUT | `/api/system/permissions/:id/enabled` | JWT admin | 启用/禁用账号（高频操作） |
| PUT | `/api/system/permissions/:id/password` | JWT admin | 重置密码（强密码校验） |
| GET | `/api/system/permissions/audit-logs` | JWT admin | 操作审计日志（启停/改密/创建/删除） |
| GET | `/api/system/permissions/audit-logs/export` | JWT admin | 导出审计日志（CSV） |

入参样例（启停）：
```json
{ "enabled": false }
```

### 3.4 通用接口

| Method | URL | 鉴权 | 说明 |
| --- | --- | --- | --- |
| POST | `/api/system/init-admin` | 公开 | **初始化超管**，必传 `username/password/email` |
| POST | `/api/auth/login` | 公开 | 单表登录，校验 `enabled=true` |
| GET | `/api/auth/current-user` | JWT | 返回当前用户信息（含 role / enabled） |
| POST | `/api/auth/change-password` | JWT | 自己改密 |

登录响应：
```json
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

---

## 四、前端关键约束

| 位置 | 约束 |
| --- | --- |
| `Layout.vue` | 「团队」一级菜单 `roles: ['admin']`（或 `v-if="isAdmin"`），下设 3 子项指向 `/system/users` / `/system/roles` / `/system/permissions` |
| `stores/user.js` | 暴露 `isAdmin` computed（`role === 'admin'`） |
| `router/index.js` | `beforeEach` 校验 `to.meta.requiresAdmin && !userStore.isAdmin` → 提示并 `next('/')` |
| 路由 meta | `/system/*` 三个路由 `meta.requiresAdmin: true` + `meta.requiresAuth: true` |
| `InitSetup.vue` | 提交 `{username, password, email}` 三字段，**不再依赖后端默认** |
| **删除** | `teamUser/List.vue` / `teamUser/Role.vue` / `api/teamUser.js` / `router/modules/teamUser.js` |
| **清理** | i18n 4 语言文件中 `teamUser.*` / `teamRole.*` 节点（已删除） |

---

## 五、部署升级指南

### 5.1 升级前（强制）

1. **备份数据库**（含历史表）：
   ```bash
   pg_dump -t system_users -t team_users -t team_roles -t operation_logs \
     -f backup_$(date +%Y%m%d).sql $DB_NAME
   ```
2. **确认当前版本**：`/api/system/init-status` 返回 `INITIALIZED`。
3. **拉取最新代码**：`git pull` 至包含本规范（v3.1）的 commit。
4. **go build 通过基线**：
   ```bash
   cd user-server && go build -o /dev/null ./...
   cd user-web && npm run build
   ```

### 5.2 升级步骤

1. **执行迁移**（v3 关键）：
   ```bash
   psql $DB_NAME -f migrations/025_unify_system_users.sql
   ```
   该脚本会：
   - 加 `enabled` 字段（默认 TRUE）
   - 把 `team_users` 数据迁入 `system_users`（bcrypt 密文零损失）
   - DROP `team_users` / `team_roles` / `team_user_permissions`
2. **重启 user-server**（后台自动迁移失效时手动执行）：
   ```bash
   ./bin/api
   ```
   启动日志会输出「system_users schema OK」。
3. **首次登录**用初始化时设置的超管账号，访问「团队 / 人员管理」确认列表正常。
4. **验证三模块 API**：
   ```bash
   TOKEN=$(curl -s -X POST $BASE/api/auth/login -d '{"username":"admin","password":"..."}' | jq -r .data.token)
   curl -H "Authorization: Bearer $TOKEN" $BASE/api/system/users
   curl -H "Authorization: Bearer $TOKEN" $BASE/api/system/roles
   curl -H "Authorization: Bearer $TOKEN" $BASE/api/system/permissions/audit-logs
   ```

### 5.3 回滚（出现数据问题时）

1. 停服。
2. 从 `backup_YYYYMMDD.sql` 恢复（仅 `team_users` / `team_roles`）：
   ```bash
   psql $DB_NAME -c "DROP TABLE IF EXISTS system_users CASCADE;"
   pg_restore --table=team_users --table=team_roles backup_*.sql
   ```
3. 切回上一 commit（v2.x 分支）。
4. 启动 → 业务恢复。

> 注意：v3.1 之后已不支持 v2 的 `team_users`，回滚**只能**回到 v2 的最近 commit。

---

## 六、常见问题 FAQ

**Q1：超管忘记密码怎么办？**
A：进入「团队 / 授权管理」，选中该超管账号 → 「重置密码」。或在数据库直接：
```sql
-- 临时方案（必须先停服，bcrypt 替换）
UPDATE system_users SET password = '$2a$10$...' WHERE username = 'admin';
```

**Q2：能否创建自定义角色？**
A：**不能**。v3.1 明确不支持。所有业务需求请用「数据范围 (data_scope)」+ 「路由 requiresAuth」+ 「后端服务层 AssertCanOperateSystemUser」组合实现。

**Q3：如何禁用某账号？**
A：「团队 / 授权管理」输入 username → 切换「启用」开关（会写 operation_logs）。**禁止**走数据库直接改 `enabled`。

**Q4：删除最后一个 admin 会发生什么？**
A：后端 400 拒绝（`ErrLastAdmin`）。前端会显示「至少保留一个超管」。

**Q5：客服（customer_service）能看到「团队」菜单吗？**
A：不能。`/system/*` 三路由 meta 都是 `requiresAdmin: true`，非 admin 角色直接被 `beforeEach` 拦截并提示「需要超管权限」。

**Q6：team_users 物理删除后，旧数据去哪了？**
A：迁移时已通过 SQL `INSERT INTO system_users ... SELECT FROM team_users` 合并，**bcrypt 密文零修改**，旧用户可直接登录。

---

## 七、测试用例清单

### 7.1 人员管理（8 例）

- [ ] admin 创建新客服 `alice`（role=customer_service）→ 列表出现，密码可用
- [ ] admin 修改 `alice` 的 role 为 staff → 刷新后生效
- [ ] admin 删除 `alice` → 列表消失，登录返回 401
- [ ] admin 尝试删除自己 → 400「至少保留一个超管」
- [ ] 非 admin 调用 `/api/system/users` → 403
- [ ] 列表分页：`page=1&size=10` 正确返回
- [ ] 用户名重复创建 → 400「用户名已存在」
- [ ] 创建时缺 password → 400「密码必填」

### 7.2 角色管理（4 例）

- [ ] GET `/api/system/roles` → 3 档 + 各自 member_count
- [ ] GET `/api/system/roles/admin` → 详情正确
- [ ] GET `/api/system/roles/customer_service/members?page=1&size=20` → 分页
- [ ] 非 admin 访问 → 403

### 7.3 授权管理（6 例）

- [ ] 启用 `alice`（已禁用）→ 200 + audit_logs 新增
- [ ] 禁用 `alice` → 200，登录返回 401「账号已禁用」
- [ ] 重置密码为 `NewPass#2024` → 200，audit_logs 新增
- [ ] 弱密码 `123` → 400「密码必须含大小写+数字」
- [ ] GET `/audit-logs` → 仅本人触发的可见（按 user_id 过滤）
- [ ] 导出 CSV → 文件下载正常

### 7.4 初始化（3 例）

- [ ] 全新系统 → `/setup` 走 3 步完成初始化
- [ ] 已初始化 → 访问 `/setup` 跳 `/login`
- [ ] `init-admin` 缺 password 字段 → 400

### 7.5 前端 Layout（3 例）

- [ ] admin 登录 → 顶栏出现「系统设置」→ 侧栏出现「团队」分组
- [ ] 客服登录 → 顶栏「系统设置」消失，无「团队」菜单
- [ ] 客服直接访问 `/system/users` → 提示「需要超管权限」并跳 `/`

---

## 八、变更历史

| 版本 | 日期 | 变更 |
| --- | --- | --- |
| v1.0 | 历史 | 双轨制（team_users + system_users） |
| v2.0 | 历史 | 引入 team_users 主导，system_users 留作 admin |
| v3.0 | 2026-07-22 | 单表化、3 档角色统一 |
| **v3.1** | **2026-07-23** | **三模块彻底分离，删除 teamUser 全部残留，落地本规范** |

---

## 九、附录 · 文件清单

### 后端
- 模型：`user-server/internal/model/system_user.go`
- 仓储：`user-server/internal/repository/system_user.go`
- 服务：`user-server/internal/service/system_user.go`
- 角色服务：`user-server/internal/service/role.go`
- 权限检查：`user-server/internal/service/permission_check.go`
- 控制器：`user-server/internal/controller/system_user.go`、`role.go`、`permission.go`
- 路由：`user-server/internal/router/system_user_routes.go`、`role_routes.go`、`permission_routes.go`
- 中间件：`user-server/internal/middleware/require_admin.go`

### 前端
- 路由模块：`user-web/src/router/modules/systemUser.js`、`role.js`、`permission.js`
- 视图：`user-web/src/views/system/UserList.vue`、`RoleList.vue`、`PermissionPanel.vue`
- API：`user-web/src/api/systemUser.js`、`role.js`、`permission.js`
- 状态：`user-web/src/stores/user.js`（`isAdmin` computed）
- 路由守卫：`user-web/src/router/index.js`（`beforeEach` 校验 `requiresAdmin`）
- 布局：`user-web/src/layout/Layout.vue`（「团队」分组 + 3 子项）
- 初始化：`user-web/src/views/setup/InitSetup.vue`

### 数据库
- 迁移脚本：`migrations/025_unify_system_users.sql`
