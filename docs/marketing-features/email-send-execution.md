# 邮件发送执行 (Email Send Execution)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `email-send-execution`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 邮件发送执行 |
| 功能名称（英文） | Email Send Execution |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 异步发送队列
- [x] 失败重试（指数退避）
- [x] 打开/点击追踪
- [x] 退订链接
- [x] 发送限流
- [x] 单元测试 / 集成测试

---

## 二、核心原理

### 2.1 业务背景

邮件发送是核心执行能力。需保证稳定送达、失败重试、追踪反馈。

### 2.3 关键算法或模型

- 退避：1s → 5s → 30s → 5min → 标记失败
- 链接替换：正则匹配 `<a href>` → 替换为 `/track/click/:id`

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | job_id | int64 | 是 | 任务ID |
| 输入 | subscriber_id | int64 | 是 | 收件人 |
| 输出 | sent | bool | 是 | 是否成功 |
| 输出 | error | string | 否 | 错误信息 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/email-send/process | 触发一次处理（内部） |
| GET | /api/email-send/stats | 发送统计 |

### 3.3 安全与合规

- TLS 加密传输
- 退订链接合规
- 失败原因脱敏

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 发送速率 | 100/s | 80/s |
| 重试成功率 | > 70% | ~80% |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Service | `internal/service/email_send_service.go` | 发送逻辑 |
| Infra | `internal/email/sender.go` + Redis + SMTP | 客户端 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| email-jobs | 任务调度 |
| email-smtp | SMTP 连接 |
| short-link | 链接追踪 |
| email-list | 收件人 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[队列 Consumer]
   → 取出 job_id + subscriber_id
   → 读取草稿、列表、SMTP
   → 变量替换
   → 插入追踪像素
   → 替换链接为短链
   → SMTP 发送
   → 失败 → 重试队列
   → 成功 → 写 email_tracks
   → 更新任务进度
```

---

## 五、流程说明

### 5.2 系统处理流程

1. 队列消费（每批 100 条）
2. 限流等待
3. 拼装邮件内容
4. SMTP 发送
5. 结果处理
6. 进度更新
7. 失败重试

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| SMTP 超时 | 500001 | 指数退避重试 |
| 邮箱无效 | 400101 | 标记 + 跳过 |
| 收件人退订 | - | 跳过 |

---

## 六、数据库设计

记录在 [email_tracks](email-list-management.md) 表

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 正常发送 | 收件人 | 200 + sent | ✅ |
| TC-002 | SMTP 失败 | 错误 SMTP | 3 次后标记失败 | ✅ |
| TC-003 | 重试成功 | 暂时性失败 | 第二次成功 | ✅ |
| TC-004 | 退订跳过 | 退订用户 | 跳过 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SEND_RATE_LIMIT | SEND_RATE_LIMIT | 100 |
| RETRY_MAX_TIMES | RETRY_MAX_TIMES | 3 |

---

## 九、参考资料

- [email-jobs-management.md](email-jobs-management.md)
- [email-list-management.md](email-list-management.md)
