# 短信任务调度 (SMS Jobs Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `sms-jobs-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 短信任务调度 |
| 功能名称（英文） | SMS Jobs Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | sms |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（sms_jobs）
- [x] 后端 Service 与 Controller
- [x] 任务创建/暂停/恢复/停止/删除
- [x] 执行记录
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

短信批量任务 = 草稿 + 列表 + 服务商 + 调度。提供完整生命周期管理。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | draft_id | int64 | 是 | 草稿 |
| 输入 | list_id | int64 | 是 | 列表 |
| 输入 | config_id | int64 | 是 | 服务商配置 |
| 输入 | schedule_type | string | 是 | immediate/cron |
| 输入 | cron_expr | string | 否 | Cron |
| 输出 | job_id | int64 | 是 | 任务ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/sms-jobs | 创建 |
| GET | /api/sms-jobs | 列表 |
| GET | /api/sms-jobs/:id | 详情 |
| DELETE | /api/sms-jobs/:id | 删除 |
| POST | /api/sms-jobs/:id/pause | 暂停 |
| POST | /api/sms-jobs/:id/resume | 恢复 |
| POST | /api/sms-jobs/:id/stop | 停止 |
| GET | /api/sms-jobs/:id/logs | 执行记录 |

### 3.3 安全与合规

- 鉴权
- 配额限制
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 任务创建 | < 300ms | ~150ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/sms_jobs.go` | 接口 |
| Service | `internal/service/sms_jobs_service.go` | 业务 |
| Repository | `internal/repository/sms_jobs_repo.go` | 数据 |
| Model | `internal/model/sms_jobs.go` | 模型 |
| Infra | `internal/cron/` + Redis | 调度+队列 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| sms-draft | 草稿 |
| sms-list | 列表 |
| sms-config | 服务商 |
| sms-provider | 实际发送 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[商户] → 创建任务
   → 校验依赖
   → 写 sms_jobs
   → 调度（队列/Cron）
   → 实际发送 → 写 sms_send_logs
   → 更新任务进度
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 选择草稿 + 列表 + 服务商
2. 设置调度
3. 创建任务
4. 监控 / 暂停 / 恢复 / 停止

### 5.2 系统处理流程

1. 鉴权
2. 依赖校验
3. 写库
4. 调度
5. 异步发送
6. 进度更新

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 草稿删除 | 404001 | 任务失败 |
| 余额不足 | 500001 | 任务失败 |

---

## 六、数据库设计

### 6.1 核心表 sms_jobs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| draft_id | bigint | FK | 草稿 |
| list_id | bigint | FK | 列表 |
| config_id | bigint | FK | 服务商 |
| schedule_type | varchar(16) | | immediate/cron |
| cron_expr | varchar(64) | | Cron |
| total_count | int | | 总数 |
| sent_count | int | 默认 0 | 已发送 |
| failed_count | int | 默认 0 | 失败 |
| status | varchar(16) | | pending/running/paused/completed/failed/cancelled |
| cursor | int | 默认 0 | 暂停时的游标 |
| started_at | timestamp | | |
| finished_at | timestamp | | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建任务 | 完整参数 | job_id | ✅ |
| TC-002 | 暂停 | job_id | status=paused | ✅ |
| TC-003 | 恢复 | job_id | 从游标继续 | ✅ |
| TC-004 | 停止 | job_id | status=stopped | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_RECIPIENTS_PER_JOB | MAX_RECIPIENTS_PER_JOB | 50000 |

---

## 九、参考资料

- [sms-draft-management.md](sms-draft-management.md)
- [sms-list-management.md](sms-list-management.md)
- sms-config.md
