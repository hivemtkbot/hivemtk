# 邮件追踪 (Email Tracking)

> **所属模块**: email
> **功能 slug**: `email-tracking`
> **文档定位**: 邮件发送后的打开 / 点击追踪与指标聚合，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **代码位置**: `user-server/internal/controller/email_tracking.go` + `internal/service/EmailTrackingService`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 邮件追踪 |
| 功能名称(英文) | Email Tracking |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 追踪像素接口（1×1 透明 PNG，无 JWT 公开路由）
- [x] 点击重定向接口（302 跳转，无 JWT 公开路由）
- [x] 任务级指标查询（送达 / 打开 / 点击 / 失败）
- [x] 任务级事件列表查询（分页）
- [x] 区间聚合指标查询（支持 RFC3339 / `2006-01-02 15:04:05` / `2006-01-02` 时间格式）
- [x] 公开路由与鉴权路由分组注册

### 1.2 待完成内容

- [ ] 实时大屏推送（SSE / WebSocket）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

邮件任务发送后，营销人员需要量化触达效果：是否被打开、链接是否被点击、整体打开率与点击率。这些指标直接影响营销策略迭代与 A/B 测试结论。

### 2.3 关键算法或模型

- **Token**: 单向签名 token，包含 job_id / 收件人加密标识，防伪
- **Pixel 响应**: 邮件客户端对错误响应敏感，所有异常均返回 1×1 PNG
- **Click Redirect**: 优先用 token 内 target；缺失时取 query 参数 `url`
- **多时间格式兼容**: `parseFlexibleTime` 支持 RFC3339 / `2006-01-02 15:04:05` / `2006-01-02`

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(TrackingPixel) | token | path | 是 | 追踪 token（可带 `.png` 后缀） |
| 输入(ClickRedirect) | token | path | 是 | 追踪 token |
| 输入(ClickRedirect) | url | query | 否 | 备用跳转目标 |
| 输入(GetJobMetrics) | id | path | 是 | 任务 ID |
| 输入(ListJobEvents) | id | path | 是 | 任务 ID |
| 输入(ListJobEvents) | page / limit | query | 否 | 分页参数 |
| 输入(GetRangeMetrics) | start | query | 是 | 开始时间 |
| 输入(GetRangeMetrics) | end | query | 是 | 结束时间 |
| 输出(TrackingPixel) | - | image/png | - | 1×1 透明 PNG |
| 输出(ClickRedirect) | - | 302 Location | - | 跳转到目标 URL |
| 输出(指标) | - | object | - | 送达 / 打开 / 点击等聚合 |

---

## 三、设计标准

### 3.1 API 契约

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/email/track/open/:token | 公开 | 追踪像素（1×1 PNG） |
| GET | /api/email/track/click/:token | 公开 | 点击重定向（302） |
| GET | /api/email/track/jobs/:id/metrics | JWT | 任务级指标 |
| GET | /api/email/track/jobs/:id/events | JWT | 任务级事件列表（分页） |
| GET | /api/email/track/metrics?start=&end= | JWT | 区间聚合指标 |

### 3.2 安全与合规

- 公开路由不携带 JWT，邮件客户端可直接访问
- token 失效仍返回像素（避免邮件客户端报错）
- 点击重定向在 token 无效时返回 400（防止任意跳转 / 钓鱼）
- 后台指标查询走 JWT 鉴权
- 设置 `Cache-Control: no-store` 防缓存

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 像素响应 | < 30ms |
| 点击重定向 | < 100ms |
| 任务指标聚合 | < 500ms（10w 事件） |
| 区间指标聚合 | < 1s（7 天范围） |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/email_tracking.go | 公开 + 鉴权路由 |
| Service | internal/service/EmailTrackingService | 事件记录 + 指标聚合 |
| Repository | internal/repository（email events） | email_events 持久化 |
| Model | internal/model/email_jobs / email_send | 关联任务 / 发送记录 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 邮件任务（email-jobs-management） | 任务 ID 来源 |
| 邮件发送执行（email-send-execution） | 发送时插入追踪 token |
| 邮件打开率追踪（email-open-tracker） | 配合使用，postmark/sendcloud webhook 入口 |

### 4.3 数据流向

```text
[邮件客户端加载图片] → [GET /track/open/:token] → [记录 open 事件] → [返回 1x1 PNG]
[收件人点击链接]   → [GET /track/click/:token] → [记录 click 事件] → [302 到 target]
[后台查询]         → [GET /track/jobs/:id/metrics] → [聚合查询] → [返回指标]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 营销人员在「邮件任务」创建任务并选择含追踪能力的模板
2. 系统发送邮件时把图片 / 链接替换为追踪 URL
3. 收件人打开邮件 → 自动加载像素 → 系统记录 open
4. 收件人点击链接 → 系统记录 click → 跳转到真实页面
5. 营销人员在「邮件追踪」页面查看任务指标 / 事件明细 / 区间趋势

### 5.2 系统处理流程

1. 接收公开请求，解析 token
2. 调用 service 记录 open / click 事件（含 IP / UA）
3. 像素请求：返回 1×1 PNG（异常也返回）
4. 点击请求：302 跳转 target；失败返回 400

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| token 为空（像素） | 200 | 仍返回像素 |
| token 无效（点击） | 400 | "追踪链接无效或已过期" |
| target 缺失（点击） | 400 | "缺少跳转目标 URL" |
| job_id 缺失（指标） | 400 | "缺少 job_id" |
| 时间格式错误 | 400 | "start/end 时间格式错误" |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `email_events` | 邮件事件（open / click / delivery / fail 等） |
| `email_jobs` | 任务主表（关联） |
| `email_send` | 单封发送记录（关联） |

事件按 job_id 聚合，token 内嵌 job_id 与收件人标识，避免明文暴露邮箱。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 像素正常返回 | 合法 token | 200 image/png | 待执行 |
| TC-002 | 像素 token 无效 | 错误 token | 仍返回像素 | 待执行 |
| TC-003 | 像素带 .png 后缀 | token=abc.png | 剥离后缀处理 | 待执行 |
| TC-004 | 点击跳转 | 合法 token + target | 302 到 target | 待执行 |
| TC-005 | 点击 token 无效 | 错误 token | 400 | 待执行 |
| TC-006 | 缺 target | token 有效但无 target 且无 url query | 400 | 待执行 |
| TC-007 | 任务指标 | job_id | 聚合指标 | 待执行 |
| TC-008 | 区间指标 | start/end | 聚合 | 待执行 |
| TC-009 | 时间格式兼容 | 多种格式 | 正常解析 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

无独立环境变量，追踪域名通过反向代理 / FRP 暴露 `/api/email/track/*` 即可。

### 8.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 像素响应失败率 | > 1% | 钉钉 |
| 点击重定向失败 | 持续 | 邮件 |

---

## 九、参考资料

- `user-server/internal/controller/email_tracking.go`
- [email-jobs-management.md](email-jobs-management.md)
- [email-send-execution.md](email-send-execution.md)
- [email-open-tracker.md](email-open-tracker.md)
