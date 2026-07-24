# 邮件列表与收件人管理 (Email List Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `email-list-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 邮件列表与收件人 |
| 功能名称（英文） | Email List & Recipients |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（email_lists / email_subscribers / email_tracks）
- [x] 后端 Service 与 Controller
- [x] 前端页面（列表/详情/导入）
- [x] CSV 批量导入
- [x] 邮件追踪（打开/点击/退订）
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

邮件营销需要管理大量收件人列表，支持分组、标签、导入、追踪。

### 2.2 解决思路

- 列表分组：按活动/产品/地区划分
- 批量导入：CSV/Excel 解析（异步任务）
- 去重：邮箱唯一性
- 追踪：像素追踪打开、链接追踪点击、退订链接

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 列表名 |
| 输入 | description | text | 否 | 描述 |
| 输入 | subscribers | []object | 否 | 收件人列表（导入） |
| 输出 | list_id | int64 | 是 | 列表ID |
| 输出 | subscriber_count | int | 是 | 收件人数量 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/email-list | 列表 |
| POST | /api/email-list | 创建 |
| GET | /api/email-list/:id | 详情 |
| PUT | /api/email-list/:id | 更新 |
| DELETE | /api/email-list/:id | 删除 |
| POST | /api/email-list/:id/subscribers | 添加收件人 |
| DELETE | /api/email-list/:id/subscribers/:subId | 删除收件人 |
| POST | /api/email-list/:id/import | 批量导入 |
| GET | /api/email-list/:id/stats | 列表统计 |
| GET | /api/email-list/:id/tracks | 邮件追踪记录 |

### 3.3 安全与合规

- 邮箱格式校验
- 退订链接（CAN-SPAM 合规）
- 黑名单过滤
- 个人敏感信息加密

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 批量导入 | 10k/分钟 | 12k |
| 列表查询 | < 300ms | ~150ms |
| 追踪回调 | < 50ms | ~20ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/email_list.go` | 接口 |
| Service | `internal/service/email_list_service.go` | 业务 |
| Repository | `internal/repository/email_list_repo.go` | 数据 |
| Model | `internal/model/email_list.go` | 模型 |
| Infra | `internal/cron/` + Redis | 异步任务+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| email-jobs | 任务引用列表 |
| email-send | 发送时读取列表 |
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| email-jobs | 任务选择列表 |
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[商户] → 上传 CSV
   → [email_list_service.Import]
   → 异步任务解析 → 去重 → 校验格式
   → 批量插入 email_subscribers
   → 返回导入报告（成功/失败/重复）
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建列表
2. 导入收件人（CSV/手动添加）
3. 查看列表统计
4. 编辑/删除收件人
5. 关联到邮件任务

### 5.2 系统处理流程

1. 鉴权 + 配额
2. CSV 解析（流式）
3. 邮箱格式校验
4. 去重
5. 批量插入
6. 更新列表计数
7. 返回导入结果

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 邮箱格式错误 | 400101 | 跳过并记录 |
| 重复邮箱 | 400102 | 跳过并统计 |
| 列表已禁用 | 403001 | 提示"列表已禁用" |

---

## 六、数据库设计

### 6.1 核心表 email_lists

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 列表名 |
| description | text | | 描述 |
| subscriber_count | int | 默认 0 | 收件人数量 |
| status | tinyint | 非空 | 0=禁用 1=启用 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |

### 6.2 核心表 email_subscribers

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| list_id | bigint | FK | 列表 |
| email | varchar(255) | 非空 | 邮箱 |
| name | varchar(64) | | 姓名 |
| status | tinyint | 默认 0 | 0=正常 1=退订 2=投诉 |
| tags | jsonb | | 标签 |
| created_at | timestamp | 非空 | |

### 6.3 核心表 email_tracks

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| job_id | bigint | FK | 邮件任务 |
| subscriber_id | bigint | FK | 收件人 |
| sent_at | timestamp | | 发送时间 |
| opened_at | timestamp | | 打开时间 |
| clicked_at | timestamp | | 点击时间 |
| unsubscribed_at | timestamp | | 退订时间 |

### 6.4 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_subscriber_list_email | list_id, email | UNIQUE | 列表内邮箱唯一 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建列表 | 完整参数 | list_id | ✅ |
| TC-002 | CSV 导入 | 1000 邮箱 | 报告 + 列表更新 | ✅ |
| TC-003 | 邮箱去重 | 重复邮箱 | 跳过重复 | ✅ |
| TC-004 | 退订追踪 | 收件人退订 | status 翻转 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_SUBSCRIBERS_PER_LIST | MAX_SUBSCRIBERS_PER_LIST | 100000 |

---

## 九、参考资料

- [email-jobs-management.md](email-jobs-management.md)
- [email-send-execution.md](email-send-execution.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
