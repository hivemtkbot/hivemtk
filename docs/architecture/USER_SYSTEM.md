# 用户体系规范 (USER SYSTEM)

> **源文件**: `user-server/internal/model/role.go` · `model/user.go` · `repository/system_user.go` · `middleware/permission.go`
> **适用范围**: `hivemtk` user-server 单租户后端
> **版本**: v3.1 — 三档角色收口

***

## 1. 角色模型（三档）

| Code               | 名称 | Tag     | 描述              | 业务约束                             |
| ------------------ | -- | ------- | --------------- | -------------------------------- |
| `admin`            | 超管 | danger  | 全权限，管理账号/角色/授权  | **至少保留 1 个启用账号**（`ErrLastAdmin`） |
| `customer_service` | 客服 | warning | 客户沟通、订单处理、智能体协同 | 可操作业务数据，不可改系统配置                  |
| `staff`            | 员工 | info    | 内容编辑、数据分析、运营日常  | 权限最窄                             |

```go
// model/role.go
const (
    SystemRoleCodeAdmin           = "admin"
    SystemRoleCodeCustomerService = "customer_service"
    SystemRoleCodeStaff           = "staff"
)
```

### 1.1 历史兼容：v1 `user` → v3.1 `staff`

```go
func NormalizeRole(code string) string {
    switch code {
    case admin, customer_service, staff:
        return code
    case "user":
        return "staff"        // v1 user 等价新 staff
    default:
        return ""             // 非法值，调用方返回 ErrInvalidInput
    }
}
```

### 1.2 校验分层

| 函数                            | 校验范围               | 用途           |
| ----------------------------- | ------------------ | ------------ |
| `model.IsValidSystemUserRole` | 仅 `admin` / `user` | 登录态校验（兼容 v1） |
| `model.IsValidRole`           | 完整三档               | 业务层校验        |

***

## 2. SystemUser（系统账号表）

```go
// model/user.go → model.SystemUser
type SystemUser struct {
    ID        uint   `primaryKey`
    Username  string `varchar(50); uniqueIndex; not null`
    Email     string `varchar(100); uniqueIndex`
    Password  string `varchar(255); not null`           // bcrypt HashPassword
    Role      string `varchar(20); default:"user"`       // 运行时 NormalizeRole
    Enabled   bool   `default:true`
    LastLogin *time.Time
    CreatedAt time.Time
    UpdatedAt time.Time
}
```

### 2.1 唯一账号保护

- 同一邮箱/用户名不能重复创建 → `ErrDuplicateEmail` / `ErrDuplicateUsername`

- 软删除：`enabled = false`（不物理删，保留审计链）

- **最后一个 admin 不可删**：`repository.ErrLastAdmin`

### 2.2 Token 角色透传

JWT 登录成功后，payload 写入 `role` 字段：

```go
// middleware/jwt.go
c.Set("role", "admin")        // 从 claims.Role 注入 gin.Context
c.Set("userID", claims.UserID)
```

***

## 3. 权限中间件三层防线

### 3.1 AdminAuthMiddleware（最常用）

```go
// middleware/jwt.go
admin := auth.Group("", middleware.AdminAuthMiddleware())
```

- 从 Context 读 `role`

- 仅 role == "admin" 放行

- 测试模式下自动短路（`IsTestMode && testing.Testing()`）

### 3.2 RequireAdminMiddleware

```go
// middleware/require_admin.go
admin := auth.Group("/system/users", middleware.RequireAdminMiddleware())
```

- 与 `AdminAuthMiddleware` 功能一致

- 响应字段统一 `{code, message}`（前端协议一致）

- 不修改 ctx，仅 Abort

### 3.3 PermissionMiddleware（细粒度）

```go
// middleware/permission.go
PermissionMiddleware("knowledge:edit")   // 单个权限
RequireAnyPermission("knowledge:read", "knowledge:edit")  // 任一权限
```

- 委托给 `PermChecker.CheckPermission(ctx, role, permission)` 接口实现

- 支持 role-based 降级（某些权限 admin 自动获得）

### 3.4 防线使用惯例

| 路由文件                    | 覆盖范围                                                                    | 防线                     |
| ----------------------- | ----------------------------------------------------------------------- | ---------------------- |
| `system_user_routes.go` | `/system/users` `/system/roles` `/system/permissions`                   | RequireAdminMiddleware |
| `system_routes.go`      | AI 工具启停 / 第三方 API Key 绑定                                                | AdminAuthMiddleware    |
| `service_routes.go`     | 快捷回复 / 会话标签 / 应用配置写                                                     | AdminAuthMiddleware    |
| `frontend_aliases.go`   | 15+ 处别名路由写操作（LLM 配置 / SMTP / SMS / 短链 / OBS / CSAT / DNC / 自动化规则 / 域名池） | AdminAuthMiddleware    |

***

## 4. 数据隔离（Tenant Scope）

虽然项目定位**单租户**，但 Repository 的 `scope/tenant.go` 仍做角色维度隔离：

```go
// repository/scope/tenant.go
// role == "admin" → 不附加 uid 条件，返回全量
// 其他角色 → 附加 OwnerAgentID / UserID 过滤
if !IsAdmin(role) {
    db = db.Where("owner_agent_id = ?", agentID)
}
```

***

## 5. 安全机制

| 机制           | 位置                                        | 说明                      |
| ------------ | ----------------------------------------- | ----------------------- |
| 密码哈希         | `pkg/utils/bcrypt.HashPassword`           | bcrypt，`$2a$10$`        |
| 暴力破解守卫       | `middleware/brute_force.go`               | 失败次数累计 → 锁定，Redis 存计数   |
| CSRF         | SameSite Cookie + Origin 校验               | 同源 POST 校验              |
| Rate Limiter | `middleware/ratelimit.go`                 | IP + user\_id 双维度滑动窗口   |
| MFA 备份码      | `service/system_user.go`                  | 10 组 backup codes，一次性消耗 |
| 审计日志         | `middleware/audit.go` → `operation_log` 表 | 所有写操作落库                 |

***

## 6. 变更规则

新增角色 / 修改角色权限时：

1. **先改** **`model/role.go`** **的** **`SystemRoleList`**（源头）
2. 同步 `model.IsValidSystemUserRole` 的校验分支
3. 同步 `service/system_user.go` 的 oneof 约束
4. 全量回归 `user-server/internal/middleware/*_test.go`
5. 前端 `platform-web/src/views/system/users/` 下拉刷新

***

## 7. 相关文件索引

| 文件                                                  | 职责                                    |
| --------------------------------------------------- | ------------------------------------- |
| `user-server/internal/model/role.go`                | 角色常量 + 列表 + 归一化                       |
| `user-server/internal/model/user.go`                | User / SystemUser 表定义                 |
| `user-server/internal/repository/system_user.go`    | CRUD + `ErrLastAdmin` 业务约束            |
| `user-server/internal/middleware/jwt.go`            | JWT 解析 + AdminAuthMiddleware          |
| `user-server/internal/middleware/require_admin.go`  | RequireAdminMiddleware                |
| `user-server/internal/middleware/permission.go`     | PermissionMiddleware + PermChecker 接口 |
| `user-server/internal/router/system_user_routes.go` | `/system/users` 路由                    |
| `user-server/internal/repository/scope/tenant.go`   | tenant scope 角色隔离                     |
| `docs/architecture/MENU_PERMISSION_PLAN.md`         | 菜单与权限矩阵                               |

