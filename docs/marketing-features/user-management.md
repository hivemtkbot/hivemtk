# 用户管理 (User Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `user-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 用户管理 |
| 功能名称（英文） | User Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | user |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（users / system_users）
- [x] 后端 Service 与 Controller（user.go）
- [ ] 前端页面与组件（UserManagement.vue）— **未提供独立 UI**（孤儿文件已清理）
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

提供用户 CRUD 能力，支持创建、查询、修改、删除、状态切换。区分主账号（users）和子账号（system_users），每个子账号归属一个主账号 + 一个商户。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | username | string | 是 | 用户名 |
| 输入 | phone | string | 是 | 手机号 |
| 输入 | email | string | 否 | 邮箱 |
| 输入 | role | string | 否 | 角色 admin/operator/viewer |
| 输出 | id | int64 | 是 | 用户ID |
| 输出 | status | int | 是 | 0=正常 1=禁用 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/user/list | 用户列表（分页+搜索） |
| POST | /api/user/create | 创建用户 |
| PUT | /api/user/update | 更新用户 |
| DELETE | /api/user/delete | 删除用户（软删除） |
| PUT | /api/user/status | 启停用户 |

### 3.3 安全与合规

- 仅商户主账号可管理子账号
- 全部操作写入审计日志
- 密码字段绝不返回（脱敏）
- 删除前需二次确认

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/user.go` | 接收请求、参数校验 |
| Service | `internal/service/user_service.go` | CRUD 业务编排 |
| Repository | `internal/repository/user_repo.go` | 数据访问 |
| Model | `internal/model/user.go` / `system_user.go` | 数据模型 |
| Infra | `internal/middleware/auth.go` | 鉴权 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 登录态校验 |
| redis | 状态缓存 |
| gorm | 数据访问 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| team-user-management | 团队管理引用用户 |

### 4.4 数据流向

```text
[商户主] → UserManagement.vue → /api/user/list
   → [user.go Controller] → [user_service.List]
   → [user_repo.Find] → [PostgreSQL]
   → 脱敏处理 → 返回分页结果
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 商户主账号进入「用户管理」页面
2. 系统加载用户列表（分页 20/页）
3. 支持按用户名/手机号搜索
4. 点击「新建」弹出表单
5. 提交后立即生效
6. 点击「启停」切换状态

### 5.2 系统处理流程

1. 鉴权：必须是已登录商户主账号
2. 参数校验：手机号格式、用户名长度
3. 业务校验：手机号唯一性、商户配额
4. 写入数据库
5. 写审计日志
6. 失效相关 Redis 缓存
7. 返回标准化响应

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 手机号已存在 | 400101 | 提示"该手机号已注册" |
| 商户配额超限 | 403001 | 提示"用户数已达上限" |
| 无权操作其他商户 | 403002 | 提示"无权操作" |

---

## 六、数据库设计

### 6.1 核心表 system_users

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| username | varchar(64) | 非空 | 用户名 |
| phone | varchar(20) | 非空 | 手机号 |
| email | varchar(128) | | 邮箱 |
| password_hash | varchar(255) | 非空 | bcrypt 哈希 |
| role | varchar(32) | | 角色 |
| status | tinyint | 非空 | 0=正常 1=禁用 |
| created_at | timestamp | 非空 | 创建时间 |
| updated_at | timestamp | 非空 | 更新时间 |
| deleted_at | timestamp | | 软删除时间 |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_sysuser_phone | phone | btree | 手机号去重 |

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建用户 | 完整参数 | 返回新用户 ID | ✅ |
| TC-002 | 手机号重复 | 重复手机号 | 400101 | ✅ |
| TC-003 | 启停用户 | user_id | status 翻转 | ✅ |
| TC-004 | 软删除 | user_id | 列表不再展示 | ✅ |
| TC-005 | 搜索分页 | keyword + page | 分页结果 | ✅ |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_USERS_PER_MERCHANT | MAX_USERS_PER_MERCHANT | 50 |

### 8.3 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 用户创建失败率 | > 10% | 飞书 |
| 单商户用户数 | > 80% 配额 | 飞书 |

---

## 九、参考资料

- [auth-login-jwt.md](auth-login-jwt.md)
- 五层架构：[GO_FIVE_LAYER_ARCHITECTURE.md](../architecture/GO_FIVE_LAYER_ARCHITECTURE.md)
- 菜单与权限规划：[MENU_PERMISSION_PLAN.md](../architecture/MENU_PERMISSION_PLAN.md)
