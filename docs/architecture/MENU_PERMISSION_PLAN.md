# User-Web（商户端）单点登录 + 单表用户化 实施计划

> 计划版本：**v3.1** ｜ 计划日期：2026-07-23 ｜ 状态更新：2026-07-24
> 计划目标：**统一单用户表 + 启用/禁用二元管控 + 三模块管理** —— 所有账号（含超管、客服）全部存储在 `system_users`；彻底清理历史 `team_users` / `team_roles` 双轨制。**user-web 是商户端（单租户、单角色），不引入菜单权限树，不做精细化授权**；超管侧提供 3 个独立管理模块：**人员管理 / 角色管理 / 授权管理**。
> 关联文档：
> - 五层架构：[GO_FIVE_LAYER_ARCHITECTURE.md](./GO_FIVE_LAYER_ARCHITECTURE.md)
> - 用户记忆硬约束：开源自部署、超管默认全权限、菜单不控制到按钮、密码入库
>
> **2026-07-24 状态更新**：
> - `must_change_password` 强制首登改密机制已彻底移除（commit 65079e5），下文涉及该机制的步骤/解决方案视为已失效。
> - `team-user-management.md` 文档已删除（双轨制清理完成）。

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

### v3.1 与 v2.0 / v3.0 的关键差异

| 维度 | v2.0（已废弃） | v3.0 | **v3.1（当前）** |
| --- | --- | --- | --- |
| 菜单权限 | 持久化菜单树 + 角色→菜单映射 | 不做 | 不做 |
| 角色值 | 5 档 | 3 档 | **3 档** |
| my-menus 接口 | 需要 | 删除 | 删除 |
| system_menus 表 | 需要 | 删除 | 删除 |
| 团队子模块 | 1 个（用户管理） | 1 个（用户管理） | **3 个**（人员/角色/授权） |
| 角色管理 | 0 | 0 | **新建**（只读 3 档角色 + 成员数） |
| 授权管理 | 0 | 0 | **新建**（启停/改密/操作审计） |
| 后端中间件 | menu_guard | JWTAuthMiddleware | **JWTAuthMiddleware + RequireAdminMiddleware** |

### v3.1 三管理模块职责

| 模块 | 路径 | 职责 | 操作 |
| --- | --- | --- | --- |
| **人员管理** | `/system/users` | 账号 CRUD | 新建 / 编辑 / 删除 / 启停 / 改密 |
| **角色管理** | `/system/roles` | 3 档系统角色只读视图 | 查看角色定义 / 查看角色下成员列表 |
| **授权管理** | `/system/permissions` | 启停 + 改密 + 操作审计 | 启停 / 改密 / 查看操作日志（谁启停/改密了谁） |

> **三者关系**：人员管理是 CRUD 主入口；角色管理是只读视图（展示 system_users 分布）；授权管理是高频操作快捷面板（不需进入列表页直接对某账号操作）+ 审计追溯。

---

## 一、现状审计

### 1.1 现状（v1.0 双轨制 — 全部需要清理）

| 实体 | 表 | 角色字段 | 现状 | 改造动作 |
| --- | --- | --- | --- | --- |
| **SystemUser** | `system_users` | `admin` / `user` | 已存在，bcrypt 存密码 | **扩展为唯一用户表**，role 扩充为 3 档 |
| **TeamUser** | `team_users` | `admin/manager/viewer` | **待清理** | **整表 DROP**（数据迁移走 system_users） |
| **TeamRole** | `team_roles` | permission code JSON | **待清理** | **整表 DROP**；permission code 体系废弃 |
| **OperationLog** | `operation_logs` | 引用 team_user.id | **部分清理** | user_id 改为引用 `system_users.id`（uint 兼容） |

**历史污染面盘点**（后端 17 文件 + 前端 11 文件 + 1 个 migration）：

| 层 | 污染文件 | 清理动作 |
| --- | --- | --- |
| model | [team_user.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/model/team_user.go) | **删除**（合并到 system_user.go） |
| repository | [team_user.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/repository/team_user.go) | **删除**（合并到 system_user.go） |
| service | [team_user.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/service/team_user.go) | **删除**（合并到 system_user.go） |
| controller | [team_user.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/controller/team_user.go) | **删除**（合并到 system_user.go） |
| service | [auth.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/service/auth.go) 双表查询 | **重写**：单表查询，role 直接来自 system_users |
| router | [business_routes.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/business_routes.go) setupTeamRoutes | **删除整个函数** |
| router | [event_bus.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/router/event_bus.go) | 清理 TeamUser 相关订阅 |
| middleware | [audit.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/middleware/audit.go) | 移除 team_users 引用 |
| event | [types.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/event/types.go) / [subscribers.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/event/subscribers.go) | 移除 TeamUser 事件类型 |
| migration | [001_team_user_management.sql](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/migrations/001_team_user_management.sql) | **删除** |
| migration | [initial_schema.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/migration/migrations/initial_schema.go) | 移除 team_users / team_roles DDL |
| migration | [a_domain_p1_migration.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/migration/migrations/a_domain_p1_migration.go) | 移除 P1-4 行级权限相关字段 |
| migration | [unmultitenant_migration.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/migration/migrations/unmultitenant_migration.go) | 移除 team_users 迁移逻辑 |
| controller | [row_level_security_controller.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/controller/row_level_security_controller.go) | 路由 user_id 类型从 team_users.id 改为 system_users.id |
| test | *_test.go（4 个） | 全部删除或重写 |
| 前端 view | [teamUser/List.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/views/teamUser/List.vue) | **删除**（被 systemUser 取代） |
| 前端 view | [teamUser/Role.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/views/teamUser/Role.vue) | **删除** |
| 前端 api | [api/teamUser.js](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/api/teamUser.js) | **删除**（合并到 api/systemUser.js） |
| 前端 router | [router/modules/teamUser.js](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/router/modules/teamUser.js) | **删除** |
| 前端 i18n | en.json / zh.json / ja.json / ar.json | 移除 teamUser.* / teamRole.* 文案 |
| 前端 layout | [Layout.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/layout/Layout.vue) | 菜单保留硬编码（**不引入后端菜单树**）；仅"超管可见设置入口"用 role 判断 |
| 前端 store | [stores/user.js](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/stores/user.js) | 保留 role 字段；增加 `isAdmin` 计算属性 |

> **总计需清理/合并**：后端 17 文件 + 前端 11 文件 + 1 个 SQL migration = **29 个文件**。

### 1.2 现有超管密码来源（**痛点**）

[admin.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/config/admin.go) 三处来源：
1. 硬编码默认值 `admin / Admin@123456`（`defaultAdminConfig`）
2. `./.env/admin.json` 配置文件
3. 环境变量 `ADMIN_USERNAME` / `PLATFORM_ADMIN_PASSWORD`

[auth.go](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-server/internal/controller/auth.go) `CreateDefaultAdmin` 仍从 `config.GetAdminConfig().DefaultAdmin` 读取并入库。

> **结论**：超管密码"硬编码默认值 + .env 覆盖 + 环境变量覆盖"三层，**违反用户"密码必须入库"的要求**，必须清理。

### 1.3 user-web 现有菜单结构（**仅做参考，不持久化**）

| 一级菜单 | 二级菜单示例 | 现状 |
| --- | --- | --- |
| 工作台 | 首页 Dashboard | 硬编码于 [Layout.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/layout/Layout.vue) |
| 客户管理 | 客户列表 / 客户画像 / RFM 分层 | 同上 |
| 内容中心 | 素材 / 话术 / 模板 | 同上 |
| AI 智能体 | 智能体配置 / 知识库 / 工作台 | 同上 |
| 触达中心 | 短链 / 活码 / 邮件 / 短信 | 同上 |
| 数据分析 | 概览 / 报表 / 看板 | 同上 |
| **团队**（超管可见） | **人员管理** / **角色管理** / **授权管理** | **v3.1 拆为 3 个独立子模块** |
| 系统 | 安装初始化 / 个人资料 | 同上 |

> **v3.1 决策**：
> - 菜单**继续硬编码在 Layout.vue**（不做后端菜单树持久化）
> - 团队一级菜单下设 3 个独立子模块，仅超管可见（`v-if="isAdmin"`）
> - 客服 / 员工登录后**看不到团队**整个一级菜单
> - 3 个子模块独立路由（不嵌套）：
>   - `/system/users` → [UserList.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/views/system/UserList.vue)
>   - `/system/roles` → [RoleList.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/views/system/RoleList.vue)
>   - `/system/permissions` → [PermissionPanel.vue](file:///Users/xiaofang/Documents/www/go/hivemtk/hivemtk/user-web/src/views/system/PermissionPanel.vue)

### 1.4 现有初始化流程

| 阶段 | 入口 | 现状 |
| --- | --- | --- |
| 1. 状态探测 | `GET /api/system/init-status`（公开） | 读 `install.lock` |
| 2. 创建超管 | `POST /api/system/init-admin`（公开，AuthController.InitAdmin） | 入参传 password（已迁移自 config） |
| 3. 标记初始化 | `POST /api/system/init-complete` → `install.MarkAdminInitialized(username)` | 写 `install.lock.initialized=true` |

> **2026-07-24 变更**：原"3. 首登改密（`must_change_password` 强制）"步骤已移除（commit 65079e5）。开源版创建超管后直接可用，登录不再跳转 `/change-password`。详见 [MERCHANT_INITIALIZATION_FLOW.md](../operations/MERCHANT_INITIALIZATION_FLOW.md)。

---

## 二、头脑风暴检查（v3.0 收口）

### 2.1 必查的潜在问题

| # | 问题 | 风险等级 | 现状 | 处理方案 |
| --- | --- | --- | --- | --- |
| Q1 | **超管密码硬编码** | 🔴 高 | `defaultAdminConfig.Password = "Admin@123456"` 写死 | 创建超管 API 强制要求调用方传 `password`；删除默认值；保留"超管首次登录后必须改密"逻辑作为安全网 |
| Q2 | **SystemUser 与 TeamUser 双轨制** | 🔴 高 | 登录时先查 system_users → 失败再查 team_users；前端/后端 controller 共存 | **彻底合并**：TeamUser 表 DROP；controller/service/repo/model 文件全部删除；service/auth.go 改为单表查询 |
| Q3 | **system_users.role CHECK 约束** | 🟠 中 | 当前 CHECK 约束只允许 `admin`/`user` | 修改 CHECK 约束：`CHECK (role IN ('admin','customer_service','staff'))` |
| Q4 | **team_users 历史数据迁移** | 🟠 中 | 已有部署实例存在 team_users 数据 | `INSERT INTO system_users (...) SELECT ... FROM team_users ON CONFLICT DO NOTHING`；bcrypt 密文直接复用 |
| Q5 | **install.lock 与 system_users 的一致性** | 🟠 中 | `install.lock` 写 AdminUsername，`system_users` 写 Admin；可能出现二者不一致 | 改 `install.MarkAdminInitialized`：不仅写 username，还要写 `user_id`；半初始化自愈时从 DB 重建 lock |
| Q6 | **客服账号存储位置** | 🟠 中 | 当前在 `team_users` 表，与 system_users 完全分离 | **客服 = system_users 一行记录**，role 字段值 = `customer_service`；密码与其他账号统一在 system_users 表 bcrypt 入库 |
| Q7 | **operation_logs.user_id 语义** | 🟠 中 | 当前引用 team_user.id（uint）；新体系引用 system_user.id（uint） | 表结构相同（uint PK），不需要 ALTER COLUMN；外键语义在文档层明确写"user_id = system_users.id" |
| Q8 | **teamUser/Login 端点（独立登录入口）** | 🟡 低 | 路由 `/api/team/users/login` 独立走 TeamUserService | **删除整个端点**；所有账号统一走 `POST /api/auth/login` |
| Q9 | **前端路由 /team/users 路径** | 🟡 低 | 引入 2 个子路由 | 改为 `/system/users`（仅超管可见） |
| Q10 | **前端 i18n key 残留** | 🟡 低 | 4 个语言文件含 `teamUser.*` / `teamRole.*` 文案 | 移除；新增 `systemUser.*` 文案 |
| Q11 | **"是否启用"开关语义** | 🟡 低 | 当前 `status` 字段值 0/1 | 在 system_users 上加 `enabled BOOLEAN` 字段（语义清晰）；登录时检查 `enabled=true`，否则 403 |
| Q12 | **超管被另一个超管误删** | 🟠 中 | 删除账号无保护 | `DeleteSafe`：当且仅当 `count(admin) > 1` 才允许删除 admin 账号 |
| Q13 | **团队子模块拆分（v3.1 新增）** | 🟠 中 | 旧"团队 → 用户管理 + 角色管理"两子模块；TeamUser 清理后只留用户管理 | 拆为 3 个独立子模块：**人员管理**（CRUD 主入口）+ **角色管理**（只读 3 档角色 + 成员分布）+ **授权管理**（启停/改密/审计）。三者**职责正交**：人员管理管"谁存在"，角色管理管"角色是什么"，授权管理管"谁能做什么" |
| Q14 | **前端 Layout 菜单的 role 过滤** | 🟡 低 | `filterMenuByRole` 用 `userStore.role` 静态判断 | 改为"团队整个一级菜单 `v-if="isAdmin"`"，子模块不单独判；3 子模块独立路由（不嵌套） |
| Q15 | **角色管理 = 角色可增删改** | 🟡 低 | 误解风险：以为能做角色 CRUD | 角色是**系统级 3 档常量**，**只读展示**；如未来要支持自定义角色，独立规划 |
| Q16 | **授权管理 = 精细化菜单权限** | 🟡 低 | 误解风险：以为要做权限点分配 | 授权管理 = 启停 + 改密 + 操作审计；不做权限点分配（v2.0 方案已废弃） |

### 2.2 需在文档中显式拒绝的反例

- ❌ **不要**保留 TeamUser 任何影子代码（即便暂时不调用，也算污染）
- ❌ **不要**在 controller 直接判断 role → 必须经 service / middleware（违反五层架构）
- ❌ **不要**新建 `*_v2.go` / `*_stub.go` / 命名带版本后缀的 service（违反命名规范）
- ❌ **不要**保留 `admin.json` 配置文件 → 清理 `config.GetAdminConfig()` 中所有 admin 密码相关字段
- ❌ **不要**在 system_users 中保留与 team_users 同义的角色值（如 `team_admin`）
- ❌ **不要**引入后端菜单树 / my-menus / role-menus（v2.0 方案已废弃）
- ❌ **不要**做按钮级权限（用户明确不需要）
- ❌ **不要**做权限点 code 体系（v1 的 permission code 全部废弃）

### 2.3 范围/边界检查

| 边界 | 是否在范围内 | 决策 |
| --- | --- | --- |
| 控制到按钮（按钮级权限） | ❌ 明确不在 | 仅"账号是否启用"二元管控 |
| 菜单级权限 | ❌ v3.0 取消 | 不持久化菜单树；前端硬编码菜单 + role 判断入口可见性 |
| 数据行级权限（`data_scope`） | ❌ 简化掉 | user-web 是单租户，不需要 |
| 部门/团队隔离 | ❌ 独立部署场景 | 默认不启用；字段保留以备未来 |
| 平台端/贡献者端/商户端权限 | ❌ 不在 user-web 范围 | 独立规划 |
| 安装时的 License 授权 | ❌ 开源版已删除 | 沿用现状 |

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

> **取消 v2.0 的 `roles: []string` / `permissions_override` / `is_admin` 字段**；简化为 `role` + `enabled`。

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

## 四、实施阶段（按风险倒序，v3.1 含三模块）

### 阶段 0：预检 & 备份（必须）
- [ ] `git status` 确认无未提交变更
- [ ] 当前分支切到 `feature/system-user-unify`，从 main 拉最新
- [ ] **备份数据库**：`pg_dump system_users team_users team_roles operation_logs > backup.sql`
- [ ] `go build -o /dev/null ./...` 确认基线编译通过

### 阶段 1：单表化 & 数据迁移（最高风险，必须第一）
- [ ] 编写迁移脚本 `migrations/002_unify_system_users.sql`：
  - [ ] 扩展 `system_users.role` CHECK 约束为 3 档
  - [ ] 新增 `enabled` 列（默认 TRUE）
  - [ ] 数据迁移：`team_users → system_users`（`ON CONFLICT DO NOTHING`；manager/viewer 映射为 staff）
  - [ ] DROP `team_user_permissions` / `team_roles` / `team_users`
  - [ ] DROP `001_team_user_management.sql`（或 rename 为 _deprecated）
- [ ] 移除 `initial_schema.go` / `a_domain_p1_migration.go` / `unmultitenant_migration.go` 中 team_users 引用
- [ ] 验证：`psql` 查询 `system_users` 数量 ≥ 之前 `team_users + system_users` 合计
- [ ] 验证：原 team_users 账号用原密码能登录新 system_users

### 阶段 2：后端代码清理（彻底删除 TeamUser）
- [ ] **删除**：
  - [ ] `model/team_user.go` / `model/team_user_test.go`
  - [ ] `repository/team_user.go` / `repository/team_user_test.go`
  - [ ] `service/team_user.go` / `service/team_user_test.go`
  - [ ] `controller/team_user.go` / `controller/row_level_security_controller.go`（合并到 system_user）
  - [ ] `router/business_routes.go#setupTeamRoutes`
  - [ ] `router/event_bus.go` 中 TeamUser 订阅
  - [ ] `middleware/audit.go` 中 team_users 引用
  - [ ] `event/types.go` / `event/subscribers.go` 中 TeamUser 事件
  - [ ] `middleware/manager_or_admin.go`（或保留但移除对 TeamUser 的依赖；新增 `RequireAdminMiddleware` 替代）
- [ ] **合并**：
  - [ ] `model/system_user.go` 扩展 role 3 档 + `enabled` 字段
  - [ ] `repository/system_user.go` 新增 `GetByUsername` / `ListByRole` / `CountAdmins` / `DeleteSafe`（拒绝最后一个 admin）/ `SetEnabled`
  - [ ] `service/system_user.go` 新增 `CreateInitialAdmin` / `CreateByAdmin` / `UpdateByAdmin` / `DeleteByAdmin` / `ResetPasswordByAdmin` / `SetEnabledByAdmin`
  - [ ] `service/auth.go#Login` 改为单表查询 + enabled 检查
- [ ] 验证：`go build -o /dev/null ./...` 全绿

### 阶段 3：路由 & 中间件（三模块分路由）
- [ ] 新增 `middleware/require_admin.go`：JWT 校验 + `role==admin` 判断
- [ ] 新增 `router/system_user_routes.go`：注册 `/api/system/users/*`（人员管理）
- [ ] 新增 `router/role_routes.go`：注册 `/api/system/roles/*`（角色管理）
- [ ] 新增 `router/permission_routes.go`：注册 `/api/system/permissions/*`（授权管理）
- [ ] `init_guard.go` 白名单更新：`/api/system/init-admin` 加入
- [ ] `admin_routes.go#setupPublicRoutes` 注册 `/system/init-admin`
- [ ] 移除 `auth_routes.go#create-default-admin` 路由
- [ ] 移除 `business_routes.go#setupTeamRoutes`

### 阶段 4：模块 ① 人员管理（后端 + 前端）
- [ ] `controller/system_user.go` 新增 5 个端点（GetList / GetByID / Create / Update / Delete）
- [ ] `service/system_user.go` 补全 5 个方法（含参数校验、DeleteSafe、密码 bcrypt）
- [ ] 前端 `api/systemUser.js` 新建（5 个 API）
- [ ] 前端 `views/system/UserList.vue` 新建：列表 + 新建/编辑/删除 + 启停 + 改密
- [ ] 前端 `router/modules/systemUser.js` 新建：路由 meta 加 `requiresAdmin: true`

### 阶段 5：模块 ② 角色管理（后端 + 前端）
- [ ] `model/role.go` 新建：3 档角色常量（`admin` / `customer_service` / `staff`）+ 描述
- [ ] `service/role.go` 新建：`ListRoles` / `GetRole` / `CountMembers` / `ListMembersByRole`
- [ ] `controller/role.go` 新建：3 个端点（list / get / members）
- [ ] 前端 `api/role.js` 新建（3 个 API）
- [ ] 前端 `views/system/RoleList.vue` 新建：3 张角色卡片（admin/customer_service/staff）；每张显示角色名/描述/成员数；点击"查看成员"展开成员列表
- [ ] 前端 `router/modules/role.js` 新建：路由 meta 加 `requiresAdmin: true`

### 阶段 6：模块 ③ 授权管理（后端 + 前端）
- [ ] `service/permission.go` 新建：`SetEnabledByAdmin` / `ResetPasswordByAdmin` / `ListAuditLogs`（查 `operation_logs` 表）
- [ ] `controller/permission.go` 新建：3 个端点（put-enabled / put-password / list-audit）
- [ ] 前端 `api/permission.js` 新建（3 个 API）
- [ ] 前端 `views/system/PermissionPanel.vue` 新建：
  - 上半区：快捷操作面板（输入 username → 启停 / 改密按钮）
  - 下半区：操作审计日志表格（分页 + 导出）
- [ ] 前端 `router/modules/permission.js` 新建：路由 meta 加 `requiresAdmin: true`

### 阶段 7：路由菜单聚合
- [ ] `Layout.vue`：整个"团队"一级菜单 `v-if="isAdmin"`，下设 3 个子菜单项（人员管理 / 角色管理 / 授权管理）
- [ ] `router/index.js`：`beforeEach` 检查 `requiresAdmin`
- [ ] 验证：客服账号登录后**整个团队菜单不可见**；超管登录后可见 3 个子模块

### 阶段 8：清理 & 文档
- [ ] 删除 `config/admin.go` 中默认密码字段；保留 `GetAdminConfig` 返回空结构
- [ ] 删除 `.env/admin.json` 模板（若存在）
- [ ] 删除 `controller/auth.go#CreateDefaultAdmin`
- [ ] **最终 grep 验证零残留**：
  - [ ] `grep -r "team_user" --include="*.go" --include="*.sql" --include="*.vue" --include="*.js"` 应返回空
  - [ ] `grep -r "team_role" --include="*.go" --include="*.sql" --include="*.vue" --include="*.js"` 应返回空
  - [ ] `grep -r "TeamUserService\|teamUserCtrl\|TeamUserController" --include="*.go"` 应返回空
- [ ] 删除文档：`docs/marketing-features/team-user-management.md`
- [ ] 新增 `docs/architecture/USER_SYSTEM.md`（三模块 API 文档 + 部署指南）

### 阶段 9：测试（4 轮）

| 轮次 | 范围 | 工具 |
| --- | --- | --- |
| 1 | `curl` 跑通所有新接口：创建超管 / login / users CRUD / roles / permissions / audit-logs | curl + JSON |
| 2 | 三模块 UI 完整覆盖：超管登录 → 团队菜单可见 3 子项 → 各子项操作；客服登录 → 团队菜单完全不可见 → 触发 requiresAdmin 路由 → 重定向 | Playwright |
| 3 | 完整 E2E：初始化 → 创建超管 → 创建客服 → 客服登录 → 禁用客服 → 客服再次登录失败 → 在审计日志中可见操作记录 | Playwright + 多角色账号 |
| 4 | **回归测试**：原 team_users 账号用旧密码能登录新 system_users；原 admin 账号可登录；权限数据完整 | curl + DB 对比 |

### 阶段 10：交付
- [ ] 提交 PR：`feature/system-user-unify → main`
- [ ] 推送部署环境
- [ ] 提交摘要：列出 14 个关键文件 + 11 个接口 + 1 个迁移 + 29 个清理文件

---

## 五、风险与回滚

| 风险 | 应对 |
| --- | --- |
| team_users 数据迁移丢失 | 迁移前 `pg_dump` 备份；脚本 `ON CONFLICT DO NOTHING`；提供反向回滚 SQL |
| 现有 system_users 已有 admin 账号 | 迁移脚本幂等；半初始化自愈：从 DB 重建 install.lock |
| 误删最后一个 admin 账号 | service 层 `DeleteSafe`：当且仅当 `count(admin) > 1` 才允许删除 |
| 客服被禁用后无法恢复 | 超管可重新 PUT `/api/system/users/:id/enabled` 启用 |
| 老版本前端访问新后端 | 登录响应增加 `enabled` 字段；前端 store 兼容缺省值 |
| 删除 setupTeamRoutes 后某条路径仍调用 | 阶段 2 完成后跑全量 `go build` + `go vet` + 集成测试覆盖 |

回滚：
- 迁移脚本反向 SQL 预先生成（`rollback_002_unify_system_users.sql`）
- 数据库备份保留 30 天
- 新接口（system/users）上线时**保留双路由过渡**：旧 `/api/team/*` 与新 `/api/system/*` 并存 14 天，灰度切换

---

## 六、已确认的设计决策（按 v3.1 用户指令）

1. ✅ **统一到 `system_users` 一张表**（超管、客服、员工都存这里）
2. ✅ **历史 TeamUser 体系彻底清除**（DROP `team_users` / `team_roles` / `team_user_permissions` 表 + 删除全部相关 model/repo/service/controller 代码）
3. ✅ **超管密码强制入参传递**（不再读 config；不留任何默认值）
4. ✅ **不需要菜单级权限**（v2.0 方案作废；user-web 是商户端，单租户单角色）
5. ✅ **不需要精细化授权**（v2.0 的 5 档收口为 3 档：admin/customer_service/staff）
6. ✅ **不需要权限点 / 权限码 / role-menus / my-menus 接口 / MenuPermission 视图**
7. ✅ **超管默认全权限**（登录即用全部页面；仅"团队整个一级菜单"用 role 判断可见性）
8. ✅ **二元管控 = `enabled` 字段 + role 字段**
9. ✅ **三管理模块**（v3.1 新增）：**人员管理** / **角色管理** / **授权管理** — 3 个独立子模块，团队一级菜单下设子项
10. ✅ **角色管理只读**（3 档系统角色常量；不允许 CRUD；如未来要支持自定义角色独立规划）
11. ✅ **授权管理 = 启停/改密/审计**（不做权限点分配；不与 v2.0 方案冲突）

---

## 七、设计收口确认（v3.1）

按 v3.1 设计，最终交付物为：

#### 后端
- **3 个新 controller**：`controller/system_user.go`（5 端点）/ `controller/role.go`（3 端点）/ `controller/permission.go`（3 端点）
- **3 个新 service**：`service/system_user.go`（扩展）/ `service/role.go`（新）/ `service/permission.go`（新）
- **2 个新 model**：`model/system_user.go`（扩展）/ `model/role.go`（新）
- **1 个新 repository 扩展**：`repository/system_user.go`（新增 GetByUsername / ListByRole / CountAdmins / DeleteSafe / SetEnabled）
- **1 个新中间件**：`middleware/require_admin.go`
- **3 个新路由文件**：`router/system_user_routes.go` / `router/role_routes.go` / `router/permission_routes.go`
- **1 个迁移脚本**：`migrations/002_unify_system_users.sql`

#### 前端
- **3 个新视图**：`views/system/UserList.vue` / `views/system/RoleList.vue` / `views/system/PermissionPanel.vue`
- **3 个新 API**：`api/systemUser.js` / `api/role.js` / `api/permission.js`
- **3 个新路由模块**：`router/modules/systemUser.js` / `router/modules/role.js` / `router/modules/permission.js`
- **1 个 store 计算属性**：`isAdmin`
- **Layout.vue 改造**：整个"团队"一级菜单 `v-if="isAdmin"`，下设 3 个子菜单项

#### 清理
- **29 个清理文件**：17 后端 + 11 前端 + 1 SQL migration
- **0 个新表**（v2.0 的 system_menus / system_role_menus 已废弃）
- **0 个新权限接口**（v2.0 的 my-menus / role-menus 已废弃）

#### 接口统计
- **11 个新接口**（人员管理 5 + 角色管理 3 + 授权管理 3）
- **1 个新表字段**：`system_users.enabled`（BOOLEAN）
- **1 个 CHECK 约束更新**：`system_users.role IN ('admin','customer_service','staff')`

**总工作量与 v3.0 接近**（增加 2 个新模块 = 2 个 controller + 2 个 service + 2 个视图 = 约 +25% 工作量），但**仍然比 v2.0 少 30%**（不写菜单树、不写权限配置 UI、不写权限点体系）。

---

## 八、待用户最后确认（如有）

- ✅ 角色值收口为 `admin / customer_service / staff` 三档 —— **已确认**
- ✅ user-web 不做菜单级权限 —— **已确认**
- ✅ 仅"启用/禁用"二元管控 —— **已确认**
- ✅ 三管理模块：人员管理 / 角色管理 / 授权管理 —— **已确认**
- ✅ 角色管理只读，不做自定义角色 —— **已确认**
- ✅ 授权管理 = 启停/改密/审计，不做权限点分配 —— **已确认**

如果以上 6 点与你预期一致，将按阶段 0 开始执行。

---

## 九、二次头脑风暴查漏（v3.1 完整化）

### 9.1 数据迁移深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L1 | team_users 与 system_users `username` 冲突 | 迁移时 `ON CONFLICT DO NOTHING` 会**静默丢数据** | 迁移前先 `SELECT username FROM team_users WHERE username IN (SELECT username FROM system_users)` 输出冲突报告；冲突时优先保留 system_users（管理域），迁移后人工比对 |
| L2 | team_users 软删除行（`deleted_at IS NOT NULL`） | 软删除账号不应迁入新表 | 迁移 SQL 加 `WHERE deleted_at IS NULL` |
| L3 | team_users 状态值（`status` 0=禁用/1=启用） | status=0 的账号不能迁入新表为 enabled=true | 迁移 SQL 加 `WHERE status = 1` |
| L4 | team_users `password` 是 bcrypt 密文 | 直接复用可登录 | ✓ 沿用 plan |
| L5 | team_users 的 `role` 字段值不在新 CHECK 内 | 迁移时 CHECK 约束失败 | 先 DROP 约束 → 数据迁移 → ADD 约束（**严格按这个顺序**） |
| L6 | operation_logs.user_id 引用 team_users.id | DROP team_users 后**外键断裂** | 迁移脚本同时 `UPDATE operation_logs SET user_id = (SELECT id FROM system_users WHERE username = (SELECT username FROM team_users WHERE id = old.user_id)) WHERE user_id IN (SELECT id FROM team_users)` — 团队用户表已有 username 字段所以可恢复 |
| L7 | system_users 已有 `status` 字段 | 与新 `enabled` 字段语义重叠 | 迁移脚本中 `UPDATE system_users SET enabled = (status = 1)` 然后**保留 status 字段**（不删，审计用） |
| L8 | team_users.created_at / updated_at 时区 | 跨时区迁移可能时间偏移 | 沿用 `TIMESTAMPTZ`，Go 端用 `time.Now().UTC()` 写入 |

### 9.2 登录 & JWT 深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L9 | 旧 JWT token 中无 `enabled` 字段 | 已登录用户即使被禁用，旧 token 仍能访问 | 引入 `version` 字段：登录时 `version=hash(now)` 写入 JWT；每次请求中间件查 DB 的 version 是否匹配；不匹配返回 401 |
| L10 | 旧前端依赖 `user.role` 字段 | 改名会破坏既有页面 | **保留** `role` 字段在 login 响应中（兼容 v1）；只新增 `enabled` 字段 |
| L11 | 旧前端 `userStore.role` 在多处硬编码 | 删除会破坏现有 isAdmin 判断 | 新增 `isAdmin` computed = `role === 'admin'`；保留 `role` 字段 |
| L12 | 登录失败错误信息泄露账号存在性 | "用户不存在" vs "密码错误" 是用户枚举攻击 | 返回统一错误 `"用户名或密码错误"`（不区分） |
| L13 | 多次登录失败无锁定 | 暴力破解风险 | 引入 `login_attempts` / `locked_until` 字段（system_users）；连续 5 次失败锁定 15 分钟 |
| L14 | JWT 过期时间无 refresh | 用户频繁被踢出体验差 | 沿用现有 `RefreshToken` 机制（已有 `/api/auth/refresh-token`） |

### 9.3 角色管理深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L15 | 角色定义存放位置 | 硬编码在 service vs 单独 model 文件 | 新建 `model/role.go` 用 `var SystemRoles = []Role{...}`；service 从这里读 |
| L16 | 角色国际化 | 角色名 `admin` vs `管理员` vs `Administrator` | 在 `model/role.go` 中存 `Name string` + `I18nKey string`；前端用 i18n 翻译；后端只用 code 逻辑判断 |
| L17 | "查看该角色下成员"分页 | 大数据量时一次返回所有性能差 | 沿用现有 `?page=1&size=20` 参数 |
| L18 | 角色卡片的"成员数"实时性 | 用户增删后数不更新 | 前端拉取时同时调 `/api/system/roles` + `/api/system/users?role=admin` 拼接；或后端在 `/api/system/roles` 返回时连表 COUNT |
| L19 | 删除角色（虽然 v3.1 不开放） | 误删导致代码报错 | 删除 `DeleteRole` 接口；在 role 上加 `is_system BOOLEAN` 字段；`is_system=true` 时禁止改 |
| L20 | 角色值校验 | 创建账号时传 `role=invalid` 应拒绝 | service 层在 `CreateByAdmin` 中 `if !isValidRole(req.Role) { return error }` |

### 9.4 授权管理深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L21 | 授权管理 audit-logs 数据源 | `operation_logs` 表是否记录启停/改密？ | 验证：现有 operation_logs 写入逻辑是否覆盖 system_users 的写操作；如果不覆盖，需在 controller/system_user.go 的 SetEnabled/ResetPassword 中**手动写入** operation_logs |
| L22 | 启停自己的账号 | 超管禁用自己 → 系统无人可管理 | service 层：`if id == actor_id { return error("不能禁用自己") }` |
| L23 | 改密后未通知用户 | 客服改密后旧密码失效无感知 | 改密响应返回提示消息；前端登录时若密码已被客服重置，由 JWT 校验失败引导重新登录（开源版已移除 `must_change_password` 机制，不再强制首登改密） |
| L24 | audit-logs 分页 + 过滤 | 大量日志翻页慢 | 提供 `?actor_id=&target_id=&action=&date_from=&date_to=` 多维过滤 |
| L25 | audit-logs 导出格式 | CSV vs Excel | v1 先做 CSV；Excel 留作未来增强 |
| L26 | 启停后 token 立即失效 | 旧 token 仍可用 | 配合 L9 的 version 机制；启停时 `UPDATE system_users SET version = uuid_generate_v4() WHERE id = ?`；旧 token 失效 |

### 9.5 前端深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L27 | 路由 meta 类型 | `requiresAdmin: true` 需要 TypeScript 类型定义 | 沿用 JS 风格：在 `router/index.js` 中 `meta: { requiresAdmin: true }`；vue-router 自动透传 |
| L28 | Layout.vue 硬编码 `topMenus` 现有"团队"子项 | 删除后子菜单为空 | Layout.vue 中改 `teamMenu.children = [{name: '人员管理', path: '/system/users'}, ...]` 数组 |
| L29 | 三个新视图的 i18n key | 4 个语言文件都要加 | 一次性补 4 个：`systemUser.*` / `role.*` / `permission.*` |
| L30 | 新视图的 Element Plus 组件 | 不一致风格 | 沿用现有 `views/teamUser/List.vue` 风格（el-card / el-table / el-form） |
| L31 | api 文件的 axios 错误处理 | 与现有不同 | 沿用 `request.js` 拦截器 |

### 9.6 五层架构合规（核对清单）

| 项 | 要求 | v3.1 方案 |
| --- | --- | --- |
| Controller 不能直访 db/repository | ✓ | controller 仅调 service |
| Service 不能直访 db | ✓ | service 仅调 repository（repository 内调 gorm） |
| Model 不含业务方法 | ✓ | model/system_user.go 仅 struct + gorm tag；业务方法在 service |
| DTO 不反向引用 service | ✓ | dto/*.go 仅 struct，不 import service |
| 禁止 service ↔ tooluse import cycle | ✓ | 三个新 service 不依赖 tooluse |
| 禁止文件命名: utils.go / common.go / *_v1.go / *_stub.go | ✓ | 文件名: `system_user.go` / `role.go` / `permission.go` / `require_admin.go` / `audit_log.go` |
| ctx 透传 | ✓ | 所有 service 方法第一个参数 ctx context.Context |
| 错误处理用 fmt.Errorf("...: %w", err) | ✓ | 沿用项目惯例 |

### 9.7 部署 & 运维深化

| # | 漏点 | 风险 | 解决方案 |
| --- | --- | --- | --- |
| L32 | 多实例部署时的 version 字段并发 | 同时禁用同一账号 | 沿用 SQL `UPDATE ... WHERE version = old_version` 的乐观锁 |
| L33 | 安装后的 admin 账号强密码校验 | 安装时设弱密码 | init-admin 路由要求密码 ≥ 8 位 + 大小写 + 数字；与 password_policy 一致 |
| L34 | i18n 翻译中"启用/禁用"按钮文案 | 不同语言长度差异 | 用 `el-button` size 固定；或用 i18n `t('permission.enable')` |
| L35 | 数据库索引缺失 | role / enabled 查询慢 | 迁移脚本加：`CREATE INDEX IF NOT EXISTS idx_system_users_role ON system_users(role); CREATE INDEX IF NOT EXISTS idx_system_users_enabled ON system_users(enabled);` |
| L36 | 现有 operation_logs 是否记 admin 操作 | 审计追溯失败 | 验证：service/system_user.go 的所有写方法都通过 `auditService.Log(actor_id, "user.set_enabled", target_id, ...)` 记录 |

### 9.8 回归测试用例

| 测试用例 | 期望 | 工具 |
| --- | --- | --- |
| 旧 system_users 账号用旧密码登录 | ✓ 成功 | curl |
| 旧 team_users 账号用旧密码登录 | ✓ 成功（迁移后） | curl + 迁移脚本 |
| system_users admin 账号删除最后一个 admin | ✗ 拒绝 | curl |
| 客服账号访问 /api/system/users | ✗ 403 | curl + JWT |
| 客服账号登录后访问 /system/users | ✗ 重定向到 / | Playwright |
| 禁用客服后客服旧 JWT 访问任意接口 | ✗ 401 | curl |
| 启用客服后客服旧 JWT 访问 | ✗ 仍 401（version 变化） | curl |
| 启用客服后客服新登录 | ✓ 成功 | Playwright |
| 改密后旧密码登录 | ✗ 失败 | curl |
| 改密后新密码登录 | ✓ 成功 | curl |
| audit-logs 看到启停/改密记录 | ✓ 完整 | curl + DB |
| 删除 admin 后 system_users 表还有 ≥1 个 admin | ✓ 保证 | DB count |

---

## 十、最终交付物清单（v3.1 完整版）

### 10.1 后端新增（15 个文件）

```
model/
  ├─ system_user.go              (扩展：加 enabled 字段)
  └─ role.go                     (新：3 档系统角色常量)

repository/
  └─ system_user.go              (扩展：新增 5 个方法)

service/
  ├─ system_user.go              (扩展：5 方法)
  ├─ role.go                     (新)
  └─ permission.go               (新)

controller/
  ├─ system_user.go              (新：5 端点)
  ├─ role.go                     (新：3 端点)
  └─ permission.go               (新：3 端点)

middleware/
  └─ require_admin.go            (新：JWT + role==admin 校验)

router/
  ├─ system_user_routes.go       (新)
  ├─ role_routes.go              (新)
  └─ permission_routes.go        (新)

migration/
  └─ 002_unify_system_users.sql  (新)
```

### 10.2 后端删除/合并（17 个文件）

```
model/team_user.go               (删除)
repository/team_user.go          (删除)
service/team_user.go             (删除)
controller/team_user.go          (删除)
controller/row_level_security_controller.go  (删除，合并到 system_user)
router/business_routes.go#setupTeamRoutes   (删除函数)
middleware/manager_or_admin.go   (删除，替换为 require_admin)
migrations/001_team_user_management.sql     (删除文件)
migration/migrations/initial_schema.go       (清理 team_users/team_roles DDL)
migration/migrations/a_domain_p1_migration.go (清理 team_users 引用)
migration/migrations/unmultitenant_migration.go (清理 team_users 引用)
audit/middleware/audit.go        (清理 team_users 引用)
event/types.go / subscribers.go  (清理 TeamUser 事件)
+ 4 个 _test.go 文件             (删除或重写)
```

### 10.3 前端新增（12 个文件）

```
src/api/
  ├─ systemUser.js               (新：5 API)
  ├─ role.js                     (新：3 API)
  └─ permission.js               (新：3 API)

src/router/modules/
  ├─ systemUser.js               (新)
  ├─ role.js                     (新)
  └─ permission.js               (新)

src/views/system/
  ├─ UserList.vue                (新：人员管理)
  ├─ RoleList.vue                (新：角色管理)
  └─ PermissionPanel.vue         (新：授权管理)
```

### 10.4 前端删除（11 个文件）

```
src/api/teamUser.js
src/router/modules/teamUser.js
src/views/teamUser/List.vue
src/views/teamUser/Role.vue
+ i18n 4 个语言文件中 teamUser.* / teamRole.* 文案清理
```

### 10.5 修改文件（6 个）

```
src/layout/Layout.vue            (团队菜单改 3 子项)
src/stores/user.js               (加 isAdmin computed)
src/router/index.js              (加 beforeEach 校验)
src/views/setup/InitSetup.vue    (发送入参 password)
src/i18n/zh.json (en/ja/ar)      (新增 systemUser/role/permission 文案)
internal/router/auth_routes.go   (移除 create-default-admin)
internal/router/business_routes.go (移除 setupTeamRoutes)
internal/router/admin_routes.go  (注册 /system/init-admin)
config/admin.go                  (删除默认密码字段)
controller/auth.go               (改写 CreateDefaultAdmin → InitAdmin)
service/auth.go                  (单表查询 + enabled 检查)
```

### 10.6 接口统计

- **11 个新接口**（人员 5 + 角色 3 + 授权 3）
- **1 个新中间件**（`require_admin`）
- **1 个迁移脚本**（`002_unify_system_users.sql`）
- **1 个新表字段**（`system_users.enabled`）
- **1 个 CHECK 约束更新**（`role IN (3 档)`）
- **0 个新表**（v2.0 方案已废）
- **0 个新权限接口**（v2.0 方案已废）

---

## 十一、编码规范特别要求

> 用户明确要求"注意编码规范要求"。

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

---

## 十二、下一步行动（编码 → 测试 → 交付）

按上述规范，开始执行：

1. **阶段 0**：预检 & 备份（立即）
2. **阶段 1**：单表化 & 数据迁移
3. **阶段 2**：后端代码清理（删除 TeamUser 体系）
4. **阶段 3**：路由 & 中间件
5. **阶段 4-6**：三模块（人员/角色/授权）后端 + 前端
6. **阶段 7**：路由菜单聚合
7. **阶段 8**：清理 & 文档
8. **阶段 9**：测试（4 轮）
9. **阶段 10**：交付

预计 sub-agent 拆分：
- Sub-agent A：阶段 1+2（数据迁移 + 后端清理）— 最高风险
- Sub-agent B：阶段 3（路由 + 中间件）— 依赖 A
- Sub-agent C：阶段 4-6 人员管理（最小模块）— 依赖 B
- Sub-agent D：阶段 4-6 角色管理 + 授权管理 — 依赖 B
- Sub-agent E：阶段 7-8 前端 Layout + 清理 — 依赖 C/D
- Sub-agent F：阶段 9 测试 — 最后串行

每个 sub-agent 完成后立即 commit，确保 Git 纪律。
