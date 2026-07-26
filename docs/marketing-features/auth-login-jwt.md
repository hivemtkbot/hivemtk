# 登录认证与 JWT 鉴权 (Auth Login & JWT)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `auth-login-jwt`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 登录认证与 JWT 鉴权 |
| 功能名称（英文） | Auth Login & JWT Authentication |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | auth |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（users / system_users / team_users）
- [x] 后端 Service 与 Controller（auth.go / user.go / account.go）
- [x] 前端页面与组件（Login.vue / Register.vue）
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

面向多角色（商户主账号 / 团队成员 / 系统管理员）的统一登录入口，提供 JWT Token 颁发、刷新、校验、撤销能力，是整个系统安全防线的第一道关卡。

### 2.2 关键算法或模型

- **bcrypt 哈希**：Cost=10，自带 salt
- **JWT 签名**：HS256，Access Token 2h，Refresh Token 7d
- **黑名单策略**：登出时将 jti 写入 Redis，过期时间=Token 剩余有效期

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | username | string | 是 | 用户名/手机号 |
| 输入 | password | string | 是 | 密码（明文，仅传输） |
| 输出 | access_token | string | 是 | 访问令牌 |
| 输出 | refresh_token | string | 是 | 刷新令牌 |
| 输出 | expires_in | int | 是 | 过期秒数 |
| 输出 | user_info | object | 是 | 用户基础信息 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md) — 项目最高规则
- [BACKEND_CODING_STANDARDS.md](../standards/BACKEND_CODING_STANDARDS.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/auth/login | 账号密码登录 |
| POST | /api/auth/sms-login | 短信验证码登录 |
| POST | /api/auth/refresh | 刷新 Token |
| POST | /api/auth/logout | 登出（加入黑名单） |
| GET | /api/auth/me | 获取当前登录用户 |

成功响应：
```json
{ "code": 0, "data": { "access_token": "eyJhbGc...", "refresh_token": "...", "expires_in": 7200, "user_info": {}}, "msg": "ok" }
```

### 3.3 安全与合规

- JWT 鉴权 + 中间件统一拦截
- bcrypt 密码哈希
- 登录失败 5 次锁定 15 分钟
- 短信验证码 1 分钟内不可重发
- 全部登录行为写入审计日志

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 登录响应延迟 | < 200ms | ~120ms |
| Token 校验 QPS | > 5000 | ~6500 |
| 并发登录用户 | 1000+ | 1200 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/auth.go` | 接收请求、参数校验 |
| Service | `internal/service/auth_service.go` | 登录逻辑编排 |
| Repository | `internal/repository/user_repo.go` | 用户数据访问 |
| Model | `internal/model/user.go` / `system_user.go` | 数据模型 |
| Infra | `internal/middleware/jwt.go` / `internal/cache/redis.go` | JWT 与 Redis |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| redis | Token 黑名单存储 |
| gorm | 用户表访问 |
| bcrypt | 密码哈希校验 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 所有 Controller | 通过 JWT 中间件进行身份校验 |

### 4.4 数据流向

```text
[用户] → [Login.vue] → POST /api/auth/login
   → [auth.go Controller] → [auth_service]
   → [user_repo] → [PostgreSQL users 表]
   → bcrypt 比对 → 签发 JWT
   → Redis 写入 jti → 返回 access_token
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 用户打开登录页
2. 选择登录方式（账号密码 / 短信）
3. 输入凭证并提交
4. 登录成功跳转首页，失败提示具体错误
5. 后续请求自动携带 Bearer Token

### 5.2 系统处理流程

1. 接收请求并校验参数（手机号格式、密码非空）
2. 限流检查（IP + 账号维度）
3. 查询用户记录（带索引）
4. bcrypt 比对密码
5. 生成 Access + Refresh Token
6. 写入 Redis 黑名单初始值（空）
7. 记录审计日志
8. 返回标准化响应

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 用户不存在 | 401001 | 统一返回"账号或密码错误"，避免枚举 |
| 密码错误 | 401001 | 累计失败次数，达 5 次锁定 15 分钟 |
| Token 过期 | 401002 | 前端自动调用 refresh 接口 |
| Token 在黑名单 | 401003 | 跳转到登录页 |

---

## 六、数据库设计

### 6.1 核心表 users

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| username | varchar(64) | UNIQUE | 用户名 |
| phone | varchar(20) | UNIQUE | 手机号 |
| password_hash | varchar(255) | 非空 | bcrypt 哈希 |
| status | tinyint | 非空 | 0=正常 1=禁用 |
| created_at | timestamp | 非空 | 创建时间 |
| updated_at | timestamp | 非空 | 更新时间 |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_users_phone | phone | btree | 手机号登录 |
| idx_users_username | username | btree | 用户名登录 |

### 6.3 迁移脚本

位于 `migrations/001_team_user_management.sql`

---

## 七、测试说明

### 7.1 测试范围

- 单元测试：密码哈希、Token 签发、过期校验
- 集成测试：登录 → 获取用户 → 登出 全链路
- UI 自动化：登录页交互、错误提示、Token 持久化

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 正常登录 | 正确凭证 | 返回 access_token | ✅ |
| TC-002 | 密码错误 5 次 | 错误密码×5 | 第 6 次返回 429 | ✅ |
| TC-003 | Token 过期 | 等待 2h | 401002 | ✅ |
| TC-004 | Refresh 续期 | 有效 refresh_token | 新 access_token | ✅ |
| TC-005 | 黑名单拦截 | 登出后再次使用 | 401003 | ✅ |

### 7.3 测试脚本位置

- 后端：`user/user-server/internal/controller/auth_test.go`
- 前端：`tests/ui/user/login.spec.js`

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| JWT_SECRET | JWT_SECRET | random 32 字节 | 签名密钥 |
| JWT_ACCESS_TTL | JWT_ACCESS_TTL | 7200 | Access 过期秒 |
| JWT_REFRESH_TTL | JWT_REFRESH_TTL | 604800 | Refresh 过期秒 |
| LOGIN_LOCK_THRESHOLD | LOGIN_LOCK_THRESHOLD | 5 | 锁定阈值 |
| LOGIN_LOCK_DURATION | LOGIN_LOCK_DURATION | 900 | 锁定秒数 |

### 8.2 依赖服务

- PostgreSQL
- Redis 7.x

### 8.3 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 登录失败率 | > 30% | 飞书/邮件 |
| Token 签发 QPS | > 1000 | 飞书/邮件 |
| 单账号高频登录 | > 10 次/分钟 | 飞书 |

---

## 九、参考资料

- [API_CONTRACT.md](../standards/API_CONTRACT.md)
- [BACKEND_CODING_STANDARDS.md](../standards/BACKEND_CODING_STANDARDS.md)
