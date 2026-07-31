# 邮件退订 (Email Unsubscribe)

> **所属模块**: email
> **功能 slug**: `email-unsubscribe`
> **文档定位**: 邮件退订确认页 / 提交 / 名单管理 / 重新订阅，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **代码位置**: `user-server/internal/controller/email_unsubscribe.go` + `internal/service/EmailUnsubscribeService`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 邮件退订 |
| 功能名称(英文) | Email Unsubscribe |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 退订确认页（HTML，公开路由）
- [x] 退订确认提交（公开路由，token 验证）
- [x] 退订名单分页查询（鉴权）
- [x] 退订名单 CSV 导出（鉴权，UTF-8 BOM）
- [x] 重新订阅接口（鉴权，合规要求）
- [x] 退订 token 签发与校验（含 job_id 关联）
- [x] 退订原因 / IP / UA / 来源链接记录
- [x] 公开路由与鉴权路由分组注册

### 1.2 待完成内容

- [ ] 退订原因统计图表

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

依据《互联网电子邮件服务管理办法》与 CAN-SPAM Act 等合规要求，每封营销邮件必须含可点击的退订链接。用户点击后落地页必须可确认退订；退订后不得再次发送营销邮件；用户可主动申请重新订阅。

### 2.3 关键算法或模型

- **Token**: 单向签名 token，包含 email + job_id + 过期时间
- **状态机**: `subscribed` → `unsubscribed` → `resubscribed`
- **HTML 模板**: `unsubscribeConfirmHTML` / `unsubscribedAlreadyHTML`，内嵌基础样式
- **CSV 导出**: 写入 UTF-8 BOM（`0xEF 0xBB 0xBF`）保证 Excel 中文识别
- **审计字段**: reason / source_link / ip / ua / job_id 全链路记录

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入(UnsubscribePage) | token | query | 是 | 退订 token |
| 输入(UnsubscribeConfirm) | token | body | 是 | 退订 token |
| 输入(UnsubscribeConfirm) | reason | body | 否 | 退订原因 |
| 输入(ListUnsubscribes) | page / limit | query | 否 | 分页参数 |
| 输入(ListUnsubscribes) | keyword | query | 否 | 邮箱关键字 |
| 输入(Resubscribe) | email | body | 是 | 邮箱（email 校验） |
| 输出(确认页) | - | text/html | - | HTML 页面 |
| 输出(确认提交) | email / status | object | - | unsubscribed |
| 输出(导出) | - | text/csv | - | CSV 下载 |
| 输出(重新订阅) | email / status | object | - | resubscribed |

---

## 三、设计标准

### 3.1 API 契约

| Method | URL | 鉴权 | 说明 |
|---|---|---|---|
| GET | /api/email/unsubscribe | 公开 | 退订确认页（HTML） |
| POST | /api/email/unsubscribe/confirm | 公开 | 退订确认提交 |
| GET | /api/email/unsubscribe/list | JWT | 退订名单分页查询 |
| GET | /api/email/unsubscribe/export | JWT | 导出退订名单（CSV） |
| POST | /api/email/unsubscribe/resubscribe | JWT | 重新订阅 |

### 3.2 安全与合规

- 公开路由：用户从邮件点击进入，无 JWT
- token 失效返回 400（避免误退订）
- 退订后再次访问确认页提示"已退订"
- 重新订阅走鉴权接口（后台操作或客服协助）
- CSV 导出含完整审计字段（合规备查）
- 重要账户通知（非营销）不在退订拦截范围

### 3.3 性能指标

| 指标 | 目标值 |
|---|---|
| 确认页响应 | < 100ms |
| 退订提交 | < 200ms |
| 名单查询（10w 行） | < 500ms |
| CSV 导出（10w 行） | < 3s |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/email_unsubscribe.go | 公开 + 鉴权路由 |
| Service | internal/service/EmailUnsubscribeService | token 校验 + 退订 / 重订阅 |
| Repository | internal/repository（email_unsubscribes） | 退订名单持久化 |
| Model | internal/model/email_jobs / email_send | 关联任务 / 收件人 |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 邮件任务（email-jobs-management） | 退订 token 含 job_id 关联 |
| 邮件发送执行（email-send-execution） | 发送时插入退订链接 |
| 邮件列表（email-list-management） | 退订邮箱在发送时过滤 |

### 4.3 数据流向

```text
[收件人点击退订链接] → [GET /unsubscribe?token=xxx] → [校验 token]
                                                     ├─ 已退订 → [返回 unsubscribedAlreadyHTML]
                                                     └─ 未退订 → [返回 unsubscribeConfirmHTML]
[用户提交确认] → [POST /unsubscribe/confirm] → [校验 token] → [入库 + 审计字段] → [返回成功]
[后台查询]   → [GET /unsubscribe/list] → [分页查询] → [返回名单]
[后台导出]   → [GET /unsubscribe/export] → [CSV 写入 BOM + 表头 + 数据]
[重新订阅]   → [POST /unsubscribe/resubscribe] → [状态变更] → [返回 resubscribed]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 收件人收到营销邮件，点击底部"退订"链接
2. 浏览器打开 `/api/email/unsubscribe?token=xxx` 退订确认页
3. 用户可填写退订原因（可选），点击"确认退订"
4. 前端 fetch 调用 `POST /api/email/unsubscribe/confirm`
5. 成功后弹出提示并刷新页面（显示已退订）
6. 如需重新订阅，联系客服通过后台 `POST /unsubscribe/resubscribe` 操作

### 5.2 系统处理流程

**确认页**：
1. 解析 token 参数
2. service.VerifyUnsubscribeToken 校验
3. 检查是否已退订（IsUnsubscribed）
4. 返回对应 HTML 页面

**确认提交**：
1. BindJSON token + reason
2. service.VerifyUnsubscribeToken
3. service.UnsubscribeEmail（含 email / reason / source_link / job_id / ip / ua）
4. 返回统一响应

**名单查询**：
1. 解析 page / limit / keyword
2. service.ListUnsubscribes
3. SuccessWithPage 返回

**CSV 导出**：
1. service.ListAllUnsubscribes 全量查询
2. 设置 `Content-Type: text/csv; charset=utf-8` + `Content-Disposition: attachment`
3. 写入 UTF-8 BOM
4. csv.Writer 写表头 + 数据

**重新订阅**：
1. BindJSON email
2. service.ResubscribeEmail
3. 返回成功

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 缺 token（确认页） | 400 | "缺少 token 参数" |
| token 无效（确认页） | 400 | "退订链接无效或已过期" |
| token 无效（提交） | 400 | "退订链接无效或已过期" |
| 退订失败 | 500 | "退订失败" |
| 重订阅邮箱格式错误 | 400 | "参数错误" |
| 重订阅失败 | 500 | "重新订阅失败" |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `email_unsubscribes` | 退订名单（email / reason / source_link / ip / ua / job_id / unsubscribed_at） |

字段含义：
- `email`: 退订邮箱
- `reason`: 退订原因（可选）
- `source_link`: 触发退订的链接路径
- `ip` / `ua`: 客户端信息
- `job_id`: 关联邮件任务
- `unsubscribed_at`: 退订时间

发送邮件前会查询此表过滤已退订邮箱。

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 确认页（未退订） | 合法 token | unsubscribeConfirmHTML | 待执行 |
| TC-002 | 确认页（已退订） | 合法 token 但已退订 | unsubscribedAlreadyHTML | 待执行 |
| TC-003 | 确认页缺 token | 空 | 400 | 待执行 |
| TC-004 | 确认页 token 无效 | 错误 token | 400 | 待执行 |
| TC-005 | 确认提交 | token + reason | 200 + unsubscribed | 待执行 |
| TC-006 | 名单查询 | page/limit/keyword | 分页列表 | 待执行 |
| TC-007 | CSV 导出 | 全量 | CSV 下载（含 BOM） | 待执行 |
| TC-008 | 重新订阅 | email | 200 + resubscribed | 待执行 |
| TC-009 | 重订阅邮箱格式错误 | 非邮箱 | 400 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 退订 token 密钥 | UNSUBSCRIBE_TOKEN_SECRET | - | 与 JWT_SECRET 共用或独立 |
| 退订 token 有效期 | UNSUBSCRIBE_TOKEN_TTL | 30d | 长有效期避免链接失效 |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- `user-server/internal/controller/email_unsubscribe.go`
- 《互联网电子邮件服务管理办法》
- CAN-SPAM Act
- [email-jobs-management.md](email-jobs-management.md)
- [email-send-execution.md](email-send-execution.md)
