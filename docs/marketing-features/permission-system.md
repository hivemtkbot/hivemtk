# 权限三件套系统 (Permission System)

> **所属模块**: system（系统管理域）
> **功能 slug**: `permission-system`
> **文档定位**: 角色管理 + 授权管理 + 菜单权限 三件套总览，遵循 [MENU_PERMISSION_PLAN.md](../architecture/MENU_PERMISSION_PLAN.md) v3.1。
> **代码位置**:
> - `user-server/internal/controller/role.go` + `internal/service/RoleService`
> - `user-server/internal/controller/permission.go` + `internal/service/AuthorizationService`
> - `user-server/internal/middleware/permission.go` + `internal/middleware/data_scope.go`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 权限三件套系统（角色 / 授权 / 菜单） |
| 功能名称(英文) | Permission System (Role / Authorization / Menu) |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] **角色管理三件套**：
  - [x] `GET /api/system/roles` 列出 3 档系统角色 + 成员数
  - [x] `GET /api/system/roles/:code` 单个角色详情
  - [x] `GET /api/system/roles/:code/members` 角色下成员列表（分页）
- [x] **授权管理三件套**：
  - [x] `PUT /api/system/permissions/:id/enabled` 启用 / 禁用账号
  - [x] `PUT /api/system/permissions/:id/password` 重置密码
  - [x] `GET /api/system/permissions/audit-logs` 操作审计日志（分页）
- [x] **菜单权限**：
  - [x] 3 档角色 → 菜单可见性映射（admin / customer_service / staff）
  - [x] 路由层 `RequireAdminMiddleware` 保护高敏接口
  - [x] 细粒度权限检查 `service.PermissionService.CheckPermission`
- [x] **行级权限（数据范围）**：详见 [row-level-security.md](row-level-security.md)
- [x] **admin 唯一性保护**：拒绝删除最后一个 admin
- [x] **审计日志全链路**：所有授权动作通过 `LogCustom` 写 `operation_logs`

### 1.2 待完成内容

- [ ] 自定义角色（v3.1 之后的独立规划，当前仅 3 档只读角色）
- [ ] 菜单配置可视化编辑

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

私域独立部署场景下，权限模型需在「简洁可控」与「业务可用」之间取平衡。HiveMtk v3.1 收口为 3 档系统角色 + 行级数据范围 + 细粒度操作权限的"三件套"模型，避免 v2.0 自定义角色方案的复杂度。

### 2.3 关键算法或模型

#### 2.3.1 3 档系统角色定义

| Code | 名称 | TagType | 描述 |
|---|---|---|---|
| `admin` | 超管 | danger | 拥有全部权限，可管理账号 / 角色 / 授权，至少保留 1 个启用账号 |
| `customer_service` | 客服 | warning | 负责客户沟通、订单处理、智能体协同 |
| `staff` | 员工 | info | 负责内容编辑、数据分析、运营等日常工作 |

> 注意：变更 role code 时需同步检查 `model.IsValidSystemUserRole` 与 `service/system_user.go` 的 oneof 约束。

#### 2.3.2 数据范围枚举（DataScope）

| 值 | 含义 | 说明 |
|---|---|---|
| `all` | 全部数据 | 仅 admin |
| `department` | 本部门数据 | 按 department_id 过滤 |
| `team` | 本团队数据 | 按 team_id 过滤 |
| `self` | 仅自己创建的数据 | 默认值 |

详见 [row-level-security.md](row-level-security.md)。

#### 2.3.3 角色默认数据范围

`model.DefaultDataScopeForRole(role)` 返回角色对应的默认数据范围（admin → all，其它 → self）。

### 2.4 输入输出定义

#### 2.4.1 角色管理

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(GetRole) | code | path | 是 | 角色 code |
| 输入(ListMembers) | code | path | 是 | 角色 code |
| 输入(ListMembers) | page / size | query | 否 | 分页参数（默认 1/20，最大 100） |
| 输出 | code / name / tag_type / description / is_system / member_count | object | - | 角色详情 |

#### 2.4.2 授权管理

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(SetEnabled) | id | path | 是 | 用户 ID |
| 输入(SetEnabled) | enabled | body | 是 | true/false |
| 输入(ResetPassword) | id | path | 是 | 用户 ID |
| 输入(ResetPassword) | password | body | 是 | 新密码 |
| 输入(ListAuditLogs) | user_id / action / page / page_size | query | 否 | 过滤与分页 |

---

## 三、设计标准

### 3.1 API 契约

#### 角色管理（`/api/system/roles/*`，由 router/role_routes.go 注册）

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/system/roles | RequireAdmin | 列出 3 档角色 + 成员数 |
| GET | /api/system/roles/:code | RequireAdmin | 单个角色详情 |
| GET | /api/system/roles/:code/members | RequireAdmin | 角色下成员列表（分页） |

#### 授权管理（`/api/system/permissions/*`，由 router/permission_routes.go 注册）

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| PUT | /api/system/permissions/:id/enabled | RequireAdmin | 启用 / 禁用账号 |
| PUT | /api/system/permissions/:id/password | RequireAdmin | 重置密码 |
| GET | /api/system/permissions/audit-logs | RequireAdmin | 操作审计日志（分页） |

### 3.2 安全与合规

- 全部端点受 `RequireAdminMiddleware` 保护（路由层）
- Service 通过 actorID（操作者 ID）重新查询用户获取 role 做权限校验，不信任 controller 传入的 role
- 业务校验失败返回 `fmt.Errorf("语义: %w", ErrInvalidInput)` → controller 转 400
- 系统级错误（DB 异常）直接返回原始 err → controller 转 500
- admin 唯一性保护：`repository.DeleteSafe` 拒绝删除最后一个 admin
- 所有"启停 / 改密 / 创建 / 删除"动作通过 `middleware.LogCustom` 写 `operation_logs`

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 角色列表查询 | < 50ms（3 条记录） |
| 角色成员分页查询 | < 200ms |
| 启用 / 禁用账号 | < 100ms |
| 重置密码 | < 200ms（含 BCrypt） |
| 审计日志查询（10w 行） | < 500ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/role.go | 角色管理（阶段 5） |
| Controller | internal/controller/permission.go | 授权管理（阶段 6） |
| Controller | internal/controller/system_user.go | 人员管理（阶段 4） |
| Middleware | internal/middleware/permission.go | 细粒度权限检查 |
| Middleware | internal/middleware/data_scope.go | 行级数据范围 |
| Middleware | internal/middleware/auth.go | JWT 鉴权 |
| Service | internal/service/RoleService | 角色查询 |
| Service | internal/service/AuthorizationService | 启停 / 改密 / 审计 |
| Service | internal/service/permission_check.go | 权限检查工具 |
| Repository | internal/repository/system_user.go | 系统用户仓储 |
| Model | internal/model/role.go | 角色字典 |
| Model | internal/model/system_user.go | 用户模型 + DataScope 枚举 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 登录认证（auth-login-jwt） | JWT 注入 user_id / role / data_scope |
| 操作日志（operation-log） | 授权动作审计 |
| 行级权限（row-level-security） | data_scope 字段消费 |
| 用户管理（user-management） | system_users 表共用 |

### 4.3 数据流向

```text
[管理员登录] → [JWT 注入 role/data_scope]
                 │
                 ├─→ [请求 /api/system/roles]   → [RoleService.ListRoles]      → [返回 3 档角色]
                 ├─→ [请求 /api/system/permissions/:id/enabled]
                 │       → [AuthorizationService.SetEnabled]
                 │       → [Repository 更新 system_users.status]
                 │       → [LogCustom 写 operation_logs]
                 └─→ [请求 /api/system/permissions/audit-logs] → [查询 operation_logs]
```

---

## 五、流程说明

### 5.1 用户操作流程

#### 角色管理

1. 管理员进入「系统管理 → 角色管理」
2. 查看 3 档角色（admin / customer_service / staff）+ 各自成员数
3. 点击某角色查看成员列表（分页）

#### 授权管理

1. 管理员进入「系统管理 → 人员管理」
2. 对某账号选择"启用 / 禁用"
3. 或选择"重置密码" → 输入新密码
4. 所有动作落审计日志
5. 在「操作日志」可按 user_id / action 过滤查询审计

### 5.2 系统处理流程

**SetEnabled**：
1. Controller 解析 id + enabled
2. `service.AuthorizationService.SetEnabled(actorID, id, enabled)`
3. Service 通过 actorID 查询操作者 role 做权限校验
4. Repository.SetEnabled 更新 system_users.status
5. 写 operation_logs
6. 返回"账号启用 / 禁用成功"

**ResetPassword**：
1. Controller 解析 id + password
2. 校验 password 非空
3. `service.AuthorizationService.ResetPassword(actorID, id, password)`
4. Service 校验权限 → 检查密码历史（forbid_reuse）→ BCrypt 哈希
5. Repository 更新 system_users.password + 写 password_history
6. 写 operation_logs
7. 返回"密码重置成功"

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| role code 为空 | 400 | "角色 code 不能为空" |
| 非法 role code | 400 | service 返回 ErrInvalidInput |
| 启停自己 | 400 | service 业务校验拒绝 |
| 删除最后一个 admin | 400 | `repository.ErrLastAdmin` |
| 改密重复历史密码 | 400 | "密码与历史重复" |
| 系统级错误 | 500 | 原始错误 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `system_users` | 系统用户（含 role / data_scope / department_id / team_id / status / password） |
| `password_history` | 密码历史（forbid_reuse 策略） |
| `operation_logs` | 操作审计日志 |

**关键约束**：
- `system_users.role` CHECK 约束：仅允许 admin / customer_service / staff
- `system_users.data_scope` CHECK 约束：仅允许 all / department / team / self
- `system_users.status`：1 启用 / 0 禁用
- 至少保留 1 个 admin（Repository.DeleteSafe 兜底）

迁移文件：`internal/migration/migrations/auth_security_migration.go` v2.10.0。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 角色列表 | - | 3 档角色 + 成员数 | 待执行 |
| TC-002 | 角色详情 | code=admin | 详情 + member_count | 待执行 |
| TC-003 | 角色成员列表 | code=customer_service | 分页列表 | 待执行 |
| TC-004 | 启用账号 | id + enabled=true | 200 + "账号启用成功" | 待执行 |
| TC-005 | 禁用账号 | id + enabled=false | 200 + "账号禁用成功" | 待执行 |
| TC-006 | 禁用最后一个 admin | id | 400 ErrLastAdmin | 待执行 |
| TC-007 | 重置密码 | id + password | 200 + 审计日志 | 待执行 |
| TC-008 | 重置为历史密码 | id + 重复密码 | 400 | 待执行 |
| TC-009 | 审计日志查询 | user_id / action | 分页列表 | 待执行 |
| TC-010 | 非管理员访问 | 普通用户 token | 403 | 待执行 |
| TC-011 | 分页参数兜底 | page=0/size=999 | 默认 1/20 | 待执行 |

### 7.2 集成测试

- `tests/permission_system_api_test.sh`：覆盖角色管理 + 授权管理端到端流程

---

## 八、部署与运维

### 8.1 配置项

无独立环境变量，全部由数据库 + 中间件承载。

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| admin 账号数 | < 1 | 钉钉（紧急） |
| 启停 / 改密动作频率 | > 10 次 / 分钟 | 钉钉（防爆破） |
| 失败的权限检查 | > 100 次 / 5 分钟 | 钉钉（防越权探测） |

---

## 九、参考资料

- `user-server/internal/controller/role.go`
- `user-server/internal/controller/permission.go`
- `user-server/internal/controller/system_user.go`
- `user-server/internal/middleware/permission.go`
- `user-server/internal/middleware/data_scope.go`
- `user-server/internal/service/permission_check.go`
- `user-server/internal/model/role.go`
- `user-server/internal/model/system_user.go`
- `docs/architecture/MENU_PERMISSION_PLAN.md` v3.1
- [row-level-security.md](row-level-security.md)
- [operation-log.md](operation-log.md)
- [user-management.md](user-management.md)
- [auth-login-jwt.md](auth-login-jwt.md)
- [security-audit.md](security-audit.md)
