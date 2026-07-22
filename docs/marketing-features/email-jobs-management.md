# 邮件任务管理 (Email Jobs Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `email-jobs-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 邮件任务管理 |
| 功能名称（英文） | Email Jobs Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（email_jobs）
- [x] 后端 Service 与 Controller
- [x] 任务创建/删除
- [x] 调度（立即/定时）
- [x] 进度跟踪
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

邮件任务 = 草稿 + 列表 + SMTP + 调度时间。统一管理任务的创建、暂停、恢复、删除。

### 2.2 解决思路

- 任务依赖：草稿、列表、SMTP 必须都已存在且启用
- 调度：立即执行 / 定时执行（Cron 表达式）
- 进度：已发送数 / 总数 / 失败数
- 取消：未开始的可以取消，进行中的需要确认

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | draft_id | int64 | 是 | 草稿ID |
| 输入 | list_id | int64 | 是 | 收件人列表 |
| 输入 | smtp_id | int64 | 是 | SMTP 配置 |
| 输入 | schedule_type | string | 是 | immediate/cron |
| 输入 | cron_expr | string | 否 | Cron 表达式 |
| 输出 | job_id | int64 | 是 | 任务ID |
| 输出 | total_count | int | 是 | 收件人总数 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/email-jobs | 创建任务 |
| GET | /api/email-jobs | 任务列表 |
| GET | /api/email-jobs/:id | 任务详情 |
| DELETE | /api/email-jobs/:id | 删除任务 |
| GET | /api/email-jobs/:id/progress | 任务进度 |
| POST | /api/email-jobs/:id/pause | 暂停 |
| POST | /api/email-jobs/:id/resume | 恢复 |
| POST | /api/email-jobs/:id/cancel | 取消 |

### 3.3 安全与合规

- 任务创建需鉴权
- 删除前需二次确认
- 配额限制

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 任务创建 | < 500ms | ~200ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/email_jobs.go` | 接口 |
| Service | `internal/service/email_jobs_service.go` | 业务 |
| Repository | `internal/repository/email_jobs_repo.go` | 数据 |
| Model | `internal/model/email_jobs.go` | 模型 |
| Infra | `internal/cron/` + Redis | 调度+队列 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| email-draft | 引用草稿 |
| email-list | 引用列表 |
| email-smtp | 引用 SMTP |
| email-send | 实际发送 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程节点 |

### 4.4 数据流向

```text
[商户] → 创建任务
   → 校验依赖（草稿/列表/SMTP）
   → 写入 email_jobs
   → 立即执行 → Redis 队列
   → 定时执行 → Cron 调度
   → 触发 email-send
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择草稿
2. 选择列表
3. 选择 SMTP
4. 设置调度（立即/定时）
5. 创建任务
6. 监控进度

### 5.2 系统处理流程

1. 鉴权
2. 依赖校验
3. 计算总收件人数
4. 写库
5. 调度（队列/Cron）
6. 实时更新进度
7. 完成后通知

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 草稿已删除 | 404001 | 提示"草稿不可用" |
| SMTP 失效 | 500001 | 任务标记失败 |
| 配额不足 | 403001 | 提示"配额已用完" |

---

## 六、数据库设计

### 6.1 核心表 email_jobs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| draft_id | bigint | FK | 草稿 |
| list_id | bigint | FK | 列表 |
| smtp_id | bigint | FK | SMTP |
| schedule_type | varchar(16) | | immediate/cron |
| cron_expr | varchar(64) | | Cron |
| total_count | int | | 总数 |
| sent_count | int | 默认 0 | 已发送 |
| failed_count | int | 默认 0 | 失败 |
| status | varchar(16) | | pending/running/paused/completed/failed/cancelled |
| started_at | timestamp | | 开始时间 |
| finished_at | timestamp | | 完成时间 |
| created_at | timestamp | 非空 | |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_emailjob_status | status | btree | 状态查询 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建任务 | 完整依赖 | job_id | ✅ |
| TC-002 | 立即发送 | schedule=immediate | 立即入队 | ✅ |
| TC-003 | 定时发送 | cron=0 9 * * * | 9 点执行 | ✅ |
| TC-004 | 暂停任务 | job_id | status=paused | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_RECIPIENTS_PER_JOB | MAX_RECIPIENTS_PER_JOB | 100000 |

---

## 九、参考资料

- [email-list-management.md](email-list-management.md)
- [email-draft-management.md](email-draft-management.md)
- email-smtp-config.md
- [email-send-execution.md](email-send-execution.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
