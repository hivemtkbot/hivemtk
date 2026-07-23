# 团队用户与角色管理 (Team User Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `team-user-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 团队用户与角色管理 |
| 功能名称（英文） | Team User & Role Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | team |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（team_users / roles / permissions / operation_logs）
- [x] 后端 Service 与 Controller
- [x] 前端页面（TeamMembers / OperationLogs）
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试
- [x] RBAC 权限中间件

---

## 二、核心原理

### 2.1 业务背景

多成员协作场景下，需要为不同角色（管理员/操作员/查看者）分配不同功能权限，并记录所有关键操作以供审计追溯。

### 2.2 解决思路

- 团队成员属于某个商户，独立于主账号
- 角色 = 权限集合（RBAC 模型）
- 操作日志记录所有写操作（CRUD / 启停 / 登录）
- 权限校验在中间件层统一拦截

### 2.3 关键算法或模型

```
HasPermission(user_id, permission_code):
  → 获取用户角色 → 获取角色权限集 → 校验 code 是否包含
```

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | user_id | int64 | 是 | 团队成员ID |
| 输入 | role | string | 是 | 角色名 |
| 输出 | permissions | []string | 是 | 权限码列表 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/team/members | 团队成员列表 |
| POST | /api/team/members | 添加成员 |
| PUT | /api/team/members/:id/role | 修改成员角色 |
| DELETE | /api/team/members/:id | 移除成员 |
| GET | /api/team/roles | 角色列表 |
| POST | /api/team/roles | 创建角色 |
| GET | /api/team/operation-logs | 操作日志 |

### 3.3 安全与合规

- 管理员才能管理角色
- 成员只能看到自己有权限的功能菜单
- 所有写操作必须记录 operation_log
- 角色删除前需解绑所有成员

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 权限校验延迟 | < 5ms | ~2ms |
| 操作日志写入 | 异步批量 | 100 条/批 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/team.go` | 团队/角色/日志接口 |
| Service | `internal/service/team_service.go` | RBAC 业务逻辑 |
| Repository | `internal/repository/team_repo.go` | 团队数据访问 |
| Model | `internal/model/team_user.go` | 团队模型 |
| Infra | `internal/middleware/rbac.go` | 权限中间件 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| user-management | 关联用户基础信息 |
| auth | 登录态校验 |
| redis | 权限缓存 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 所有 Controller | 通过 RBAC 中间件拦截 |

### 4.4 数据流向

```text
请求 → JWT 中间件 → RBAC 中间件
   → 解析 user_id → Redis 读权限集 → 校验
   → 通过则放行,否则 403
   → Controller → Service → 操作日志异步落库
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 管理员进入「团队管理」
2. 查看成员列表
3. 邀请新成员（手机号+角色）
4. 修改成员角色
5. 移除成员
6. 查看操作日志（时间范围、成员、操作类型）

### 5.2 系统处理流程

1. 接收请求，鉴权
2. RBAC 校验（管理员权限）
3. 业务逻辑执行
4. 写操作日志（含 IP/UA/前后值）
5. 失效相关缓存
6. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 角色不存在 | 404001 | 提示"角色不存在" |
| 仍有成员绑定 | 409001 | 提示"请先解绑成员" |
| 权限不足 | 403001 | 统一无权限提示 |

---

## 六、数据库设计

### 6.1 核心表 team_users

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| user_id | bigint | FK | 关联用户 |
| role | varchar(32) | 非空 | 角色 |
| status | tinyint | 非空 | 0=正常 1=禁用 |
| created_at | timestamp | 非空 | 创建时间 |
| updated_at | timestamp | 非空 | 更新时间 |

### 6.2 核心表 operation_logs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| user_id | bigint | FK | 操作人 |
| action | varchar(64) | 非空 | 操作类型 |
| target_type | varchar(32) | | 目标类型 |
| target_id | varchar(64) | | 目标ID |
| before_value | jsonb | | 操作前值 |
| after_value | jsonb | | 操作后值 |
| ip | varchar(64) | | IP |
| ua | varchar(255) | | User-Agent |
| created_at | timestamp | 非空 | 操作时间 |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_oplog_user_time | user_id, created_at | btree | 日志查询 |

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 添加成员 | phone + role | 返回 member_id | ✅ |
| TC-002 | 修改角色 | member_id + new_role | 200 | ✅ |
| TC-003 | 移除成员 | member_id | 列表不再展示 | ✅ |
| TC-004 | 权限校验 | viewer 调用 admin API | 403001 | ✅ |
| TC-005 | 日志查询 | 时间范围+成员 | 分页日志 | ✅ |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| DEFAULT_ROLES | DEFAULT_ROLES | admin/operator/viewer |
| LOG_BATCH_SIZE | LOG_BATCH_SIZE | 100 |

---

## 九、参考资料

- [FUNCTION_DETAILS.md](../architecture/FUNCTION_DETAILS.md)
- [user-management.md](user-management.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
