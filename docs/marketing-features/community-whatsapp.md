# WhatsApp 营销 (WhatsApp Marketing)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `community-whatsapp`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | WhatsApp 营销 |
| 功能名称（英文） | WhatsApp Marketing |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | community |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（whatsapp_accounts / drafts / tasks / templates / messages）
- [x] 后端 Service 与 Controller
- [x] 前端页面（账号/草稿/任务/模板/群发 5 页）
- [x] chromedp 浏览器自动化（WhatsApp Web）
- [x] 模板消息 + 草稿消息
- [x] 群发任务 + 发送限流
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

WhatsApp 作为全球最大即时通讯工具，是跨境营销主战场。通过 chromedp 模拟 WhatsApp Web 登录，实现自动消息发送。

### 2.2 解决思路

- 账号：扫码登录（持久化 Cookie + LocalStorage）
- 模板：预设常用话术（订单确认/物流通知/营销活动）
- 草稿：富文本编辑 + 变量
- 任务：号码列表 + 模板/草稿 + 群发
- 限流：每账号 50-200 条/小时（防封号）

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_id | int64 | 是 | 账号 |
| 输入 | target_phones | []string | 是 | 目标手机号 |
| 输入 | template_id | int64 | 否 | 模板 |
| 输入 | draft_id | int64 | 否 | 草稿 |
| 输入 | variables | jsonb | 否 | 变量 |
| 输出 | task_id | int64 | 是 | 任务ID |
| 输出 | total | int | 是 | 总数 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/whatsapp/accounts | 账号列表 |
| POST | /api/whatsapp/accounts | 添加账号 |
| POST | /api/whatsapp/accounts/:id/login | 登录 |
| GET | /api/whatsapp/accounts/:id/qrcode | 二维码 |
| GET | /api/whatsapp/drafts | 草稿 |
| POST | /api/whatsapp/drafts | 创建草稿 |
| GET | /api/whatsapp/templates | 模板 |
| POST | /api/whatsapp/templates | 创建模板 |
| POST | /api/whatsapp/tasks | 创建群发任务 |
| GET | /api/whatsapp/tasks | 任务列表 |
| GET | /api/whatsapp/messages | 消息记录 |

### 3.3 安全与合规

- Cookie 加密
- 频次限制（防封号）
- 内容审核
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 单条消息发送 | < 8s | ~5s |
| 群发速率 | 50/h | 60/h |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/whatsapp.go` | 接口 |
| Service | `internal/service/whatsapp_service.go` | 业务 |
| Repository | `internal/repository/whatsapp_repo.go` | 数据 |
| Model | `internal/model/whatsapp_*.go` | 模型 |
| Infra | `internal/browser/chromedp.go` + Redis | 浏览器+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auto-reply-universal | 复用引擎 |
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[商户] → 群发任务
   → [whatsapp_service.CreateTask]
   → 写 whatsapp_tasks
   → 队列消费
   → chromedp 登录态复用
   → 逐条发送（限流）
   → 写 whatsapp_messages
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加 WhatsApp 账号（扫码）
2. 创建模板/草稿
3. 创建群发任务
4. 监控进度
5. 查看消息记录

### 5.2 系统处理流程

1. 鉴权
2. 校验账号在线
3. 写任务
4. 异步队列消费
5. 限流等待
6. chromedp 发送
7. 写消息记录
8. 更新进度

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 账号掉线 | 401001 | 任务暂停 |
| 频次超限 | 429001 | 限流等待 |
| 号码无效 | 400101 | 跳过 |

---

## 六、数据库设计

### 6.1 核心表 whatsapp_accounts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| phone | varchar(32) | UNIQUE | 手机号 |
| name | varchar(64) | | 昵称 |
| cookie_enc | text | | 加密 Cookie |
| status | tinyint | 非空 | 0=离线 1=在线 2=禁用 |
| last_active_at | timestamp | | |

### 6.2 核心表 whatsapp_tasks

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| target_phones | jsonb | 非空 | 目标号码 |
| template_id | bigint | FK | 模板 |
| draft_id | bigint | FK | 草稿 |
| total_count | int | | 总数 |
| sent_count | int | 默认 0 | 已发送 |
| failed_count | int | 默认 0 | 失败 |
| status | varchar(16) | | pending/running/paused/completed/failed |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 添加账号 | 扫码 | 账号 ID | ✅ |
| TC-002 | 模板发送 | 模板 + 号码 | 收到消息 | ✅ |
| TC-003 | 群发任务 | 100 号码 | 100 条记录 | ✅ |
| TC-004 | 频次限制 | 1 小时内 100 条 | 前 50 正常 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| WA_HOURLY_LIMIT | WA_HOURLY_LIMIT | 50 |
| WA_INTERVAL_MIN | WA_INTERVAL_MIN | 60 |
| WA_INTERVAL_MAX | WA_INTERVAL_MAX | 120 |

---

## 九、参考资料

- BROWSER_ASSISTANT.md
- [auto-reply-universal.md](auto-reply-universal.md)
- [FUNCTION_DETAILS.md 第七章](../architecture/FUNCTION_DETAILS.md#七消息模块---whatsapp营销)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
