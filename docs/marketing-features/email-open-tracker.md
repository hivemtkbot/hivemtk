# 邮件打开率追踪 (Email Open Tracker)

> **所属模块**: email
> **功能 slug**: `email-open-tracker`
> **文档定位**: 通过追踪像素 + 第三方 SMTP 平台 webhook 采集打开事件，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **代码位置**: `user-server/internal/controller/email_open_tracker_controller.go` + `internal/service/EmailOpenTrackerService`
> **设计依据**: `docs/核心链路优化.md` §13.2 邮件打开率追踪

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 邮件打开率追踪 |
| 功能名称(英文) | Email Open Tracker |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P1 |
| 实际完成时间 | 2026-07 |
| 最后更新 | 2026-07-24 |

### 1.1 已完成内容

- [x] 1×1 透明 PNG 追踪像素（公开路由，无 JWT）
- [x] Postmark 风格 webhook 入站（公开路由）
- [x] SendCloud（塞邮式）webhook 入站（公开路由）
- [x] 任务打开率指标查询接口（鉴权）
- [x] 服务降级：service 未初始化时像素仍返回，避免影响邮件显示
- [x] Cache-Control 策略（`public, max-age=N, immutable`）

### 1.2 待完成内容

- [ ] 更多 SMTP 平台 webhook 适配（阿里云 / 腾讯云 / 网易）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

部分 SMTP 服务商（Postmark / SendCloud）会主动通过 webhook 推送邮件打开事件，比单纯的像素追踪更精准（能区分图片被禁用的情况）。系统需同时支持「像素主动追踪」与「webhook 被动接收」两条采集通道。

### 2.2 解决思路

- 像素通道：`GET /api/email/track/pixel/:token.png` 返回 1×1 PNG 并记录 open 事件
- Webhook 通道：
  - Postmark 推送到 `POST /api/email/track/webhook/postmark`
  - SendCloud 推送到 `POST /api/email/track/webhook/sendcloud`
- 后台通过 `GET /api/email/track/open-metrics?job_id=xxx&total_sent=N` 计算打开率

### 2.3 关键算法或模型

- **EmailOpenPixel**: 内置 1×1 PNG 常量，service 不可用时仍返回
- **RenderPixel**: 解析 token → 记录事件 → 返回像素 + content-type + max-age
- **PostmarkOpenEvent / SendCloudOpenEvent**: 各自的事件结构体
- **打开率**: `open_count / total_sent * 100%`，total_sent 由调用方传入

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(TrackingPixel) | token | path | 是 | 可带 `.png` 后缀 |
| 输入(PostmarkWebhook) | RecordType / etc | body | 是 | Postmark Open 事件 |
| 输入(SendCloudWebhook) | event / etc | body | 是 | SendCloud Open 事件 |
| 输入(GetOpenMetrics) | job_id | query | 是 | 任务 ID |
| 输入(GetOpenMetrics) | total_sent | query | 否 | 发送总量（默认 0） |
| 输出(像素) | - | image/png | - | 1×1 透明 PNG |
| 输出(webhook) | recorded | bool | - | 是否入库成功 |
| 输出(指标) | open_rate | float | - | 打开率 |

---

## 三、设计标准

### 3.1 API 契约

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/email/track/pixel/:token | 公开 | 追踪像素（1×1 PNG） |
| POST | /api/email/track/webhook/postmark | 公开 | Postmark webhook 入站 |
| POST | /api/email/track/webhook/sendcloud | 公开 | SendCloud webhook 入站 |
| GET | /api/email/track/open-metrics | JWT | 任务打开率指标 |

### 3.2 安全与合规

- 公开 webhook 路由需在生产部署时通过反向代理 + 来源 IP 白名单限制
- service 降级保护：`svc == nil` 时像素仍返回，避免影响邮件显示
- token 无效也返回像素（避免邮件客户端报错）
- 后台指标查询走 JWT 鉴权

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 像素响应 | < 30ms |
| webhook 入站 | < 200ms |
| 打开率聚合 | < 500ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/email_open_tracker_controller.go | 公开 + 鉴权路由 |
| Service | internal/service/EmailOpenTrackerService | 事件入库 + 打开率计算 |
| Model | internal/model/email_jobs / email_send | 关联任务 / 发送记录 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 邮件追踪（email-tracking） | 提供通用追踪能力，本模块专门处理打开率 |
| 邮件任务（email-jobs-management） | 任务 ID 来源 |
| 邮件发送执行（email-send-execution） | 发送时插入像素 URL |

### 4.3 数据流向

```text
[邮件客户端加载图片] → [GET /track/pixel/:token] → [记录 open 事件] → [返回 PNG]
[Postmark 推送]     → [POST /track/webhook/postmark] → [解析事件入库] → [返回 recorded=true]
[SendCloud 推送]    → [POST /track/webhook/sendcloud] → [解析事件入库] → [返回 recorded=true]
[后台查询打开率]    → [GET /track/open-metrics?job_id=X&total_sent=N] → [聚合查询] → [返回 open_rate]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 营销人员创建邮件任务，模板支持插入打开率追踪像素
2. 邮件发送时把图片 URL 替换为 `/api/email/track/pixel/:token`
3. 收件人打开邮件 → 自动加载像素 → 系统记录 open
4. 若使用 Postmark / SendCloud，SMTP 平台也会通过 webhook 推送打开事件
5. 营销人员在「邮件追踪 → 打开率」查看任务级打开率

### 5.2 系统处理流程

**像素通道**：
1. 接收 GET 请求
2. 解析 token（剥离 `.png` 后缀）
3. service 调用 `RenderPixel` → 记录事件 + 返回 PNG / content-type / max-age
4. 异常时仍返回内置像素常量

**Webhook 通道**：
1. 接收 POST 请求
2. `ShouldBindJSON` 到对应事件结构体（Postmark / SendCloud）
3. service 调用 `RecordPostmarkEvent` / `RecordSendCloudEvent` 入库
4. 返回 `recorded=true` 或 400/500

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| service 未初始化（像素） | 200 | 仍返回像素 |
| token 无效（像素） | 200 | 仍返回像素 |
| service 未初始化（webhook） | 503 | "邮件打开追踪服务未初始化" |
| 参数错误（webhook） | 400 | "参数错误：..." |
| 记录失败（webhook） | 500 | "记录事件失败：..." |
| 缺 job_id（指标） | 400 | "缺少 job_id 参数" |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `email_events` | 邮件事件（含 open 类型，记录来源：pixel / postmark / sendcloud） |
| `email_jobs` | 任务主表（关联） |

不同来源的 open 事件以 `source` 字段区分，避免重复计算。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 像素正常 | 合法 token | 200 PNG + Cache-Control | 待执行 |
| TC-002 | 像素 service 降级 | svc=nil | 仍返回像素 | 待执行 |
| TC-003 | 像素 token 无效 | 错误 token | 仍返回像素 | 待执行 |
| TC-004 | Postmark webhook | 合法事件 | recorded=true | 待执行 |
| TC-005 | SendCloud webhook | 合法事件 | recorded=true | 待执行 |
| TC-006 | webhook 参数错误 | 缺字段 | 400 | 待执行 |
| TC-007 | webhook service 降级 | svc=nil | 503 | 待执行 |
| TC-008 | 打开率查询 | job_id + total_sent | open_rate | 待执行 |
| TC-009 | 缺 job_id | query 空 | 400 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| Postmark webhook 白名单 | POSTMARK_WEBHOOK_IPS | - | 可选，反向代理层校验 |
| SendCloud webhook 白名单 | SENDCLOUD_WEBHOOK_IPS | - | 可选，反向代理层校验 |

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| webhook 入站失败率 | > 1% | 钉钉 |
| 打开率异常下跌 | 同比 > 30% | 钉钉 |

---

## 九、参考资料

- `user-server/internal/controller/email_open_tracker_controller.go`
- `docs/核心链路优化.md` §13.2
- [email-tracking.md](email-tracking.md)
- [email-jobs-management.md](email-jobs-management.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-24 | 独立功能文档生成（F-P1-108 补建） | |
