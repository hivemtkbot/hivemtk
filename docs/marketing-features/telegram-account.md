# Telegram 账号管理 (Telegram Account)

> **所属模块**: community-management
> **功能 slug**: `telegram`
> **文档定位**: Telegram Bot 账号管理，支持 AI 销售自动化与群组管理。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | Telegram 账号管理 |
| 功能名称(英文) | Telegram Account Management |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | community-management |
| 优先级 | P1 |

### 1.1 已完成内容
- [x] Telegram Bot 账号管理（bot_token 加密存储）
- [x] Webhook 注册与配置
- [x] `setupTelegramRoutes` 路由注册
- [x] `internal/controller/telegram_account_controller.go` 后端控制器
- [x] allowed_chats 白名单管理
- [x] AI 销售自动化承接 Telegram 消息
- [x] 前端 `user-web/src/views/telegram/account.vue` 账号管理

### 1.2 待完成内容
- [ ] Telegram 群组批量管理增强

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
Telegram 是海外私域销售的重要触达渠道，支持 Bot 自动化承接客户咨询、推送营销内容。系统需集中管理多个 Telegram Bot 凭证，配置 Webhook 接收消息，并通过白名单控制承接范围。

### 2.3 关键算法或模型
- 凭证加密：AES-256-GCM 加密 bot_token
- Webhook 签名校验：基于 bot_token 的 secret_token 校验
- 消息路由：按 chat_id 匹配白名单与绑定智能体

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | bot_token | string | 是 | Telegram Bot Token |
| 输入 | name | string | 是 | 账号名称 |
| 输入 | webhook_url | string | 否 | Webhook 回调地址 |
| 输入 | allowed_chats | array | 否 | 允许承接的 chat 列表 |
| 输出 | bot_id | int64 | 是 | Bot ID |
| 输出 | bot_token | string | 是 | 加密后的 token |
| 输出 | webhook_url | string | 是 | Webhook 地址 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- Webhook 响应 < 200ms（先 200 后异步处理）
- 消息路由延迟 < 500ms
- Bot 在线率 ≥ 99.5%

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/telegram/account/list | 账号列表 | JWT |
| POST | /api/telegram/account | 新建账号 | JWT |
| PUT | /api/telegram/account/:id | 更新账号 | JWT |
| DELETE | /api/telegram/account/:id | 删除账号 | JWT |
| POST | /api/telegram/account/:id/register-webhook | 注册 Webhook | JWT |
| POST | /api/telegram/account/:id/test | 发送测试消息 | JWT |
| POST | /api/telegram/webhook/:bot_id | Webhook 回调入口 | 签名校验 |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| telegram_accounts | Telegram 账号主表 |
| telegram_chat_whitelist | 会话白名单表 |
| telegram_message_logs | 消息日志表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| bot_id | bigint | Bot ID |
| bot_token | varchar(255) | 加密后的 Bot Token |
| webhook_url | varchar(512) | Webhook 回调地址 |
| allowed_chats | jsonb | 允许承接的 chat 列表 |
| name | varchar(64) | 账号名称 |

---

## 六、业务流程
### 6.1 主流程
1. 运营人员注册 Telegram Bot，填入 bot_token
2. 系统加密存储 token，调用 Telegram API 注册 Webhook
3. 客户在 Telegram 发送消息，Telegram 推送至 Webhook
4. 系统校验签名与白名单，路由到绑定智能体
5. 智能体生成回复，调用 Telegram API 发送消息
6. 消息写入日志表

### 6.2 异常处理
- Webhook 签名校验失败：返回 401
- chat 不在白名单：忽略消息
- Telegram API 限流：指数退避重试
- token 失效：账号标记无效，告警

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| Telegram 账号管理 | /telegram/account | telegram/account.vue |

### 7.2 关键交互
- 账号列表表格（名称、bot_id、Webhook 状态、白名单数）
- 新增/编辑账号表单（bot_token 密文输入）
- Webhook 注册按钮
- 白名单管理（chat_id 列表编辑）
- 发送测试消息弹窗

---

## 八、测试策略
### 8.1 单元测试
- bot_token 加密/解密单测
- Webhook 签名校验单测
- 白名单匹配单测

### 8.2 集成测试
- Webhook 回调端到端测试
- 消息路由到智能体测试
- Telegram API 调用稳定性测试
