# 行级权限 / 数据范围 (Row-Level Security)

> **所属模块**: system（系统管理域 / 认证安全）
> **功能 slug**: `row-level-security`
> **文档定位**: 数据范围（DataScope）行级过滤机制总览，遵循 [MENU_PERMISSION_PLAN.md](../architecture/MENU_PERMISSION_PLAN.md) v3.1。
> **代码位置**: `user-server/internal/middleware/data_scope.go` + `user-server/internal/model/system_user.go`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 行级权限 / 数据范围 |
| 功能名称(英文) | Row-Level Security / Data Scope |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system |
| 优先级 | P1（P1-4 行级权限） |

### 1.1 已完成内容

- [x] 4 档数据范围枚举（all / department / team / self）
- [x] `system_users.data_scope` 字段 + CHECK 约束
- [x] `DataScopeMiddleware` 中间件：从 JWT / DB 注入 data_scope
- [x] `ApplyDataScope` GORM 查询过滤函数
- [x] admin 角色强制 data_scope = all
- [x] 降级保护：查询失败 / db 未初始化时降级为 self（保守策略）
- [x] 角色默认数据范围 `DefaultDataScopeForRole`

### 1.2 待完成内容

- [ ] 部门 / 团队管理 UI（当前 data_scope 字段可配但部门组织架构待补）
- [ ] data_scope 后台配置页面（当前仅 API）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

私域部署下虽是单租户，但商户内部仍有多角色协作需求：客服只能看自己的客户，部门主管可看本部门数据，超管可看全部。行级权限通过 `data_scope` 字段在 SQL 查询层强制过滤，避免每个 service / controller 重复写 if-else。

### 2.3 关键算法或模型

#### 2.3.1 4 档数据范围枚举

| 值 | 含义 | SQL 行为 |
|---|---|---|
| `all` | 全部数据 | 不过滤 |
| `department` | 本部门数据 | `WHERE {departmentField} = {dept_id}` |
| `team` | 本团队数据 | `WHERE {teamField} = {team_id}` |
| `self` | 仅自己创建的数据 | `WHERE {ownerField} = {user_id}` |

#### 2.3.2 admin 强制 all

```go
if roleStr == model.SystemUserRoleAdmin {
    c.Set("data_scope", model.DataScopeAll)
    c.Next()
    return
}
```

admin 角色始终 data_scope=all，不受字段值影响。

#### 2.3.3 ApplyDataScope 过滤逻辑

```go
func ApplyDataScope(database *gorm.DB, ctx *gin.Context, ownerField, departmentField, teamField string) *gorm.DB {
    // admin / data_scope=all → 不过滤
    if roleStr == model.SystemUserRoleAdmin { return database }
    if dsStr == model.DataScopeAll { return database }

    switch dsStr {
    case model.DataScopeSelf:
        database = database.Where(ownerField+" = ?", userID)
    case model.DataScopeDepartment:
        if deptID > 0 && departmentField != "" {
            database = database.Where(departmentField+" = ?", deptID)
        } else if userID > 0 {
            // 没有部门字段或部门 ID → 降级为 self
            database = database.Where(ownerField+" = ?", userID)
        }
    case model.DataScopeTeam:
        // 类似 department
    default:
        // 默认 self
        database = database.Where(ownerField+" = ?", userID)
    }
    return database
}
```

#### 2.3.4 降级链

1. JWT 已有 data_scope → 直接使用
2. JWT 缺失 → 从 DB 查询 system_users.data_scope
3. DB 查询失败 → 降级为 self
4. db 未初始化（测试场景）→ 降级为 self
5. data_scope 为空 → `DefaultDataScopeForRole(role)` 兜底

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | user_id | uint | 是 | 从 JWT 注入 |
| 输入 | role | string | 是 | 从 JWT 注入 |
| 输入 | data_scope | string | 否 | JWT 已有则用，否则查 DB |
| 输出 | data_scope | string | - | 写入 gin.Context |
| 输出 | department_id | uint | - | 写入 gin.Context（若有） |
| 输出 | team_id | uint | - | 写入 gin.Context（若有） |
| 输入(ApplyDataScope) | database | *gorm.DB | 是 | GORM 查询实例 |
| 输入(ApplyDataScope) | ownerField | string | 否 | 默认 "user_id" |
| 输入(ApplyDataScope) | departmentField | string | 否 | 空表示不支持部门维度 |
| 输入(ApplyDataScope) | teamField | string | 否 | 空表示不支持团队维度 |
| 输出(ApplyDataScope) | *gorm.DB | - | - | 附加 WHERE 条件的查询实例 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [MENU_PERMISSION_PLAN.md](../architecture/MENU_PERMISSION_PLAN.md) v3.1 §3.2
- 私域独立部署：无 merchant_id 字段

### 3.2 中间件链顺序

```text
JWTAuthMiddleware → DataScopeMiddleware → [RequireAdminMiddleware（仅 admin 接口）] → Controller
```

### 3.3 ApplyDataScope 调用约定

- Controller / Service 在查询数据前**必须**调用 `middleware.ApplyDataScope`
- 调用前需确保 ctx 已经过 `DataScopeMiddleware`
- `ownerField` / `departmentField` / `teamField` 按表实际字段传入
- 链式调用安全：返回的 `*gorm.DB` 可继续 `.Where()` / `.Find()`

### 3.4 安全与合规

- admin 角色强制 all（不可被字段值覆盖）
- 降级保守策略：异常时降级为 self，宁可漏看不可越权
- 字段缺失（如某表无 team_id）自动降级为 self
- 不依赖前端传 data_scope（仅 JWT / DB）

### 3.5 性能指标

| 指标 | 目标值 |
|---|---|
| 中间件开销 | < 5ms（DB 查询补充时 < 30ms） |
| ApplyDataScope 开销 | < 1ms |
| 索引覆盖 | owner_field / department_field / team_field 均需建索引 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| L2 中间件 | internal/middleware/data_scope.go | DataScopeMiddleware + ApplyDataScope |
| L3 业务 | 各 service / controller 调用 ApplyDataScope | 行级过滤入口 |
| L5 数据 | system_users 表 data_scope / department_id / team_id 字段 | 数据模型 |
| Migration | internal/migration/migrations/auth_security_migration.go | v2.10.0 P1-4 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 登录认证（auth-login-jwt） | JWT 注入 user_id / role |
| 权限三件套（permission-system） | role / data_scope 字段共用 |
| 用户管理（user-management） | system_users 表共用 |
| 客户管理（customer-360） | ApplyDataScope 主要消费方 |
| 线索管理（clue-management） | ApplyDataScope 主要消费方 |
| 订单管理 | ApplyDataScope 主要消费方 |

### 4.3 数据流向

```text
[用户登录] → [JWT 包含 user_id + role + data_scope（可选）]
                │
                ▼
         [JWTAuthMiddleware]
                │
                ▼
         [DataScopeMiddleware]
                │
                ├─ role == admin → 强制 data_scope=all
                ├─ JWT 已有 data_scope → 直接使用
                └─ JWT 缺失 → 查 DB system_users.data_scope
                                │
                                ▼
                  [写回 gin.Context]
                                │
                                ▼
            [Controller 调用 ApplyDataScope]
                                │
                                ▼
            [GORM 查询追加 WHERE 条件]
                                │
                                ▼
            [返回过滤后的结果集]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 管理员在「系统管理 → 人员管理」创建用户
2. 选择角色（admin / customer_service / staff）
3. 配置 data_scope（all / department / team / self）
4. 配置 department_id / team_id（按需）
5. 该用户登录后，所有业务查询自动按 data_scope 过滤

### 5.2 系统处理流程

**中间件链**：
1. JWTAuthMiddleware 解析 token → 注入 user_id / role / data_scope（若 JWT 内有）
2. DataScopeMiddleware：
   - admin → 强制 all
   - JWT 已有合法 data_scope → 直接放行
   - 缺失 → 查 DB → 写回 gin.Context
3. 后续中间件 / Controller 通过 `ctx.Get("data_scope")` 读取

**查询过滤**：
1. Controller 拿到 GORM DB 实例
2. 调用 `database = middleware.ApplyDataScope(database, ctx, "owner_id", "dept_id", "team_id")`
3. ApplyDataScope 按 data_scope 追加 WHERE
4. 后续 `.Find(&records)` 返回过滤后的结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 未找到 user_id | 401 | "未找到用户信息" |
| user_id 类型错误 | 500 | "用户 ID 类型错误" |
| 用户不存在 | 401 | "用户不存在" |
| DB 异常 | - | 降级为 self（不阻断请求） |
| db 未初始化（测试） | - | 降级为 self |
| data_scope 为空 | - | `DefaultDataScopeForRole` 兜底 |
| 表无 department_field | - | 降级为 self |

---

## 六、数据库设计

### 6.1 核心表结构

#### `system_users`（关键字段）

| 字段 | 类型 | 说明 |
|---|---|---|
| id | uint | 主键 |
| role | varchar | admin / customer_service / staff |
| data_scope | varchar | all / department / team / self |
| department_id | uint | 部门 ID（可空） |
| team_id | uint | 团队 ID（可空） |

#### CHECK 约束

- `role IN ('admin', 'customer_service', 'staff')`
- `data_scope IN ('all', 'department', 'team', 'self')`

迁移文件：`internal/migration/migrations/auth_security_migration.go` v2.10.0（P1-1 MFA / P1-2 异常登录 / P1-3 密码策略 / P1-4 行级权限）。

### 6.2 业务表索引建议

每张需要行级过滤的业务表（customers / clues / orders / messages 等）应在 `owner_id` / `department_id` / `team_id` 字段上建索引。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | admin 不过滤 | role=admin | 不追加 WHERE | 待执行 |
| TC-002 | data_scope=all 不过滤 | ds=all | 不追加 WHERE | 待执行 |
| TC-003 | data_scope=self | ds=self + owner_id | WHERE owner_id = user_id | 待执行 |
| TC-004 | data_scope=department | ds=dept + dept_id | WHERE dept_id = dept_id | 待执行 |
| TC-005 | data_scope=team | ds=team + team_id | WHERE team_id = team_id | 待执行 |
| TC-006 | department 字段缺失 | ds=dept + 无 departmentField | 降级 self | 待执行 |
| TC-007 | team_id 为 0 | ds=team + team_id=0 | 降级 self | 待执行 |
| TC-008 | db 未初始化 | 测试场景 | 降级 self | 待执行 |
| TC-009 | 用户不存在 | userID=999 | 401 | 待执行 |
| TC-010 | data_scope 为空 | ds="" | DefaultDataScopeForRole 兜底 | 待执行 |
| TC-011 | JWT 已有 data_scope | ds=team | 直接使用，不查 DB | 待执行 |
| TC-012 | admin JWT 覆盖字段 | role=admin + data_scope=self | 强制 all | 待执行 |

### 7.2 集成测试要点

- 中间件链顺序：JWT → DataScope → Controller
- ApplyDataScope 链式调用安全（不影响后续 Where）
- 索引覆盖验证（EXPLAIN ANALYZE）

---

## 八、部署与运维

### 8.1 配置项

无独立环境变量，全部由数据库字段承载。

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- `user-server/internal/middleware/data_scope.go`
- `user-server/internal/model/system_user.go`（DataScope 枚举 + IsValidDataScope + DefaultDataScopeForRole）
- `user-server/internal/migration/migrations/auth_security_migration.go` v2.10.0 P1-4
- `docs/architecture/MENU_PERMISSION_PLAN.md` v3.1 §3.2
- [permission-system.md](permission-system.md)
- [auth-login-jwt.md](auth-login-jwt.md)
- [user-management.md](user-management.md)
- [security-audit.md](security-audit.md)
