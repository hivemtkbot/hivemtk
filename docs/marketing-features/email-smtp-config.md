# SMTP 配置管理 (Email SMTP Config)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `email-smtp-config`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | SMTP 配置管理 |
| 功能名称（英文） | Email SMTP Config |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（email_smtp_configs）
- [x] 后端 Service 与 Controller
- [x] 前端页面（列表/创建/编辑/测试）
- [x] 多 SMTP 服务商支持（QQ/163/Gmail/企业邮箱/SendGrid/Mailgun）
- [x] 连接测试 + 发送测试邮件
- [x] 凭据加密存储
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

邮件营销需要 SMTP 中转，支持商户自配 SMTP 服务商，灵活控制发件人、发送频率、IP 信誉。

### 2.2 解决思路

- 商户可添加多个 SMTP 配置（每个独立服务）
- 凭据加密存储（AES-256）
- 发送时负载均衡或优先级选择
- 测试连接 + 发送测试邮件验证

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 配置名 |
| 输入 | host | string | 是 | SMTP 主机 |
| 输入 | port | int | 是 | 端口 |
| 输入 | username | string | 是 | 用户名 |
| 输入 | password | string | 是 | 密码（明文保存即加密） |
| 输入 | from_email | string | 是 | 发件邮箱 |
| 输入 | from_name | string | 否 | 发件人显示名 |
| 输入 | tls | bool | 默认 true | 启用 TLS |
| 输出 | config_id | int64 | 是 | 配置ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/email-smtp | 配置列表 |
| POST | /api/email-smtp | 创建配置 |
| GET | /api/email-smtp/:id | 配置详情 |
| PUT | /api/email-smtp/:id | 更新配置 |
| DELETE | /api/email-smtp/:id | 删除配置 |
| POST | /api/email-smtp/:id/test | 测试连接 |
| POST | /api/email-smtp/:id/send-test | 发送测试邮件 |

### 3.3 安全与合规

- 密码 AES-256 加密
- 仅显示掩码（****）
- 删除前需解绑任务
- 凭据变更审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 测试连接 | < 5s | ~2s |
| 发送测试邮件 | < 10s | ~5s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/email_smtp.go` | 接口 |
| Service | `internal/service/email_smtp_service.go` | 业务 |
| Repository | `internal/repository/email_smtp_repo.go` | 数据 |
| Model | `internal/model/email_smtp.go` | 模型 |
| Infra | `internal/utils/secrets.go` + `gomail` | 加密+发送 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| utils/secrets | 凭据加解密 |
| email-send | 发送时使用 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| email-jobs | 任务选择 SMTP |
| email-send | 实际发送 |

### 4.4 数据流向

```text
[商户] → 填写 SMTP 配置
   → [email_smtp_service.Create]
   → 加密密码 → 写库
   → 测试连接（可选）
   → 返回 config_id
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建 SMTP 配置
2. 填写主机/端口/账号/密码
3. 测试连接
4. 发送测试邮件
5. 启用配置

### 5.2 系统处理流程

1. 鉴权
2. 参数校验
3. 密码加密
4. 写库
5. 测试连接（异步）
6. 返回结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 主机不可达 | 500001 | 提示检查网络 |
| 认证失败 | 401001 | 提示检查账号密码 |
| 端口被封 | 500002 | 提示换端口 |

---

## 六、数据库设计

### 6.1 核心表 email_smtp_configs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(64) | 非空 | 配置名 |
| host | varchar(128) | 非空 | 主机 |
| port | int | 非空 | 端口 |
| username | varchar(128) | 非空 | 用户名 |
| password_enc | varchar(512) | 非空 | 加密密码 |
| from_email | varchar(128) | 非空 | 发件邮箱 |
| from_name | varchar(64) | | 发件人 |
| tls | tinyint | 默认 1 | 启用 TLS |
| daily_limit | int | 默认 500 | 日发送上限 |
| status | tinyint | 非空 | 0=禁用 1=启用 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建配置 | 完整参数 | config_id | ✅ |
| TC-002 | 测试连接 | 真实 SMTP | 200 / 连接成功 | ✅ |
| TC-003 | 错误密码 | 错密码 | 401001 | ✅ |
| TC-004 | 加密验证 | 读库 | 密文存储 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SMTP_DEFAULT_TLS | SMTP_DEFAULT_TLS | true |
| SMTP_DAILY_LIMIT | SMTP_DAILY_LIMIT | 500 |

---

## 九、参考资料

- [email-list-management.md](email-list-management.md)
- [email-send-execution.md](email-send-execution.md)
- SENSITIVE_DATA_ENCRYPTION.md

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
