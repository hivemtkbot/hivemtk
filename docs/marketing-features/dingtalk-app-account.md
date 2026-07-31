# 钉钉企业内部应用账号 (DingTalk App Account)

> **所属模块**: community（社群管理域）
> **功能 slug**: `dingtalk-app-account`
> **文档定位**: 钉钉企业内部应用账号 CRUD 与连通性测试，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **代码位置**: `user-server/internal/controller/dingtalk_app_account_controller.go` + `internal/service/DingTalkAppService`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 钉钉企业内部应用账号 |
| 功能名称(英文) | DingTalk App Account |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | community |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 应用账号 CRUD（AppKey / AppSecret / AgentId / 回调 Token / AESKey）
- [x] 入站收消息开关（inbound_enabled）
- [x] AI 智能体绑定（ai_agent_id）
- [x] 凭据脱敏返回（AppSecret / AESKey 掩码）
- [x] 配置测试接口（校验必填项）
- [x] 最后一次错误记录（last_error_at / last_error_msg）

### 1.2 待完成内容

- [ ] 钉钉 access_token 自动刷新缓存
- [ ] 入站消息处理（接收钉钉回调后路由到智能体）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

钉钉企业内部应用是企业内部协同 / 智能客服的重要渠道。商户需在用户端后台维护应用账号（AppKey / AppSecret / AgentId），并支持把入站消息路由到 AI 智能体处理。

### 2.3 关键算法或模型

- **VO 转换**: `toDingTalkAppVO` 把 AppSecret / AESKey 转为掩码字段
- **掩码函数**: 复用 `maskFeishuSecret`（同包内通用脱敏工具）
- **状态字段**: `status` 默认 1（启用），支持禁用
- **错误记录**: `last_error_at` + `last_error_msg` 便于运维定位

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_name | string | 是 | 账号名称 |
| 输入 | app_key | string | 是 | 钉钉 AppKey |
| 输入 | app_secret | string | 是 | 钉钉 AppSecret（加密存储） |
| 输入 | agent_id | string | 否 | 应用 AgentId |
| 输入 | token | string | 是 | 回调 Token |
| 输入 | aes_key | string | 是 | 回调 AESKey（加密存储） |
| 输入 | inbound_enabled | bool | 否 | 是否启用入站收消息 |
| 输入 | ai_agent_id | string | 否 | 绑定的 AI 智能体 ID |
| 输入 | user_id | uint | 否 | 所属用户 |
| 输入 | status | int | 否 | 状态（默认 1 启用） |
| 输出 | id | uint | - | 账号 ID |
| 输出 | app_secret_masked | string | - | AppSecret 掩码 |
| 输出 | aes_key_masked | string | - | AESKey 掩码 |
| 输出 | last_error_at / last_error_msg | - | - | 最后错误信息 |

---

## 三、设计标准

### 3.1 API 契约

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/dingtalk-app/accounts | JWT | 列出所有账号 |
| GET | /api/dingtalk-app/accounts/:id | JWT | 查询单个账号 |
| POST | /api/dingtalk-app/accounts | JWT | 创建账号 |
| PUT | /api/dingtalk-app/accounts/:id | JWT | 更新账号 |
| DELETE | /api/dingtalk-app/accounts/:id | JWT | 删除账号 |
| POST | /api/dingtalk-app/accounts/:id/test | JWT | 测试账号配置 |

### 3.2 安全与合规

- AppSecret / AESKey 加密存储（AES-256-GCM）
- 返回时统一掩码（`app_secret_masked` / `aes_key_masked`）
- 测试接口只返回 `ok: true` + `inbound_enabled`，不暴露敏感信息
- 路由由 JWT 中间件保护

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 列表查询 | < 100ms |
| 创建 / 更新 | < 200ms |
| 测试接口 | < 100ms（静态校验） |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/dingtalk_app_account_controller.go | CRUD + test |
| Service | internal/service/DingTalkAppService | 业务校验 + 持久化 |
| Repository | internal/repository（dingtalk_app_account） | 表持久化 |
| Model | internal/model（dingtalk_app_account） | 数据模型 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 多 AI 智能体（ai-agent） | ai_agent_id 绑定 |
| 通用脱敏工具（maskFeishuSecret） | AppSecret / AESKey 掩码 |
| 飞书账号管理（feishu-account） | 共用掩码工具，模式类似 |

### 4.3 数据流向

```text
[创建账号] → [Controller 解析参数] → [Service 校验] → [Repository 入库] → [VO 脱敏返回]
[查询列表] → [Repository 查询] → [VO 转换脱敏] → [返回]
[测试] → [GetAccount] → [校验 AppKey/AppSecret/Token 必填] → [返回 ok]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 用户在钉钉开放平台创建企业内部应用，获取 AppKey / AppSecret / AgentId
2. 配置回调 Token / AESKey（用于钉钉回调验签）
3. 在用户端「社群管理 → 钉钉应用账号」点击"新增"
4. 填写参数 → 保存
5. 点击"测试"校验必填项
6. 可选：开启入站收消息 + 绑定 AI 智能体

### 5.2 系统处理流程

1. Controller 解析 JSON 请求体到 `dingTalkAppAccountRequest`
2. 转换为 model.DingTalkAppAccount（status 默认 1）
3. Service.CreateAccount 入库
4. 返回脱敏 VO（`toDingTalkAppVO`）

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| ID 解析失败 | 400 | "ID 错误" |
| 账号不存在 | 404 | "账号不存在" |
| 创建失败 | 500 | "创建失败" |
| 更新失败 | 500 | "更新失败" |
| 删除失败 | 500 | "删除失败" |
| 配置不完整 | 400 | "AppKey/AppSecret/Token 均为必填" |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `dingtalk_app_accounts` | 钉钉企业内部应用账号 |

字段：account_name / app_key / app_secret（加密） / agent_id / token / aes_key（加密） / inbound_enabled / ai_agent_id / user_id / status / last_error_at / last_error_msg

私域独立部署：无 merchant_id 字段。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建账号 | 完整参数 | 200 + 脱敏 VO | 待执行 |
| TC-002 | 查询列表 | - | 200 + 数组 | 待执行 |
| TC-003 | 查询单个 | id | 200 + VO | 待执行 |
| TC-004 | 不存在的 ID | id=999 | 404 | 待执行 |
| TC-005 | 更新账号 | id + 参数 | 200 + VO | 待执行 |
| TC-006 | 删除账号 | id | 200 | 待执行 |
| TC-007 | 测试 - 配置完整 | id | ok=true | 待执行 |
| TC-008 | 测试 - 缺 AppSecret | id | 400 | 待执行 |
| TC-009 | 脱敏返回 | - | app_secret_masked 不可见明文 | 待执行 |
| TC-010 | ID 解析失败 | id=abc | 400 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

无独立环境变量，AppSecret / AESKey 加密密钥复用全局加密密钥（同 feishu-account）。

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- `user-server/internal/controller/dingtalk_app_account_controller.go`
- [feishu-account.md](feishu-account.md)
- [telegram-account.md](telegram-account.md)
- [wecom-account.md](wecom-account.md)
- [ai-agent.md](ai-agent.md)
