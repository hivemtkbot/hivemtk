# 邮件草稿管理 (Email Draft Management)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `email-draft-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 邮件草稿管理 |
| 功能名称（英文） | Email Draft Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | email |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（email_drafts）
- [x] 后端 Service 与 Controller
- [x] 前端富文本编辑器
- [x] 模板变量支持（`{{name}}` `{{company}}`）
- [x] 自动保存草稿
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

邮件发送前需要编辑和预览。提供富文本编辑 + 模板变量替换能力。

### 2.2 解决思路

- 富文本编辑器（基于 Quill）
- 模板变量（`{{var_name}}` 占位）
- 自动保存（每 30 秒一次）
- HTML 预览

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | subject | string | 是 | 邮件主题 |
| 输入 | body_html | text | 是 | HTML 正文 |
| 输入 | body_text | text | 否 | 纯文本 |
| 输入 | variables | jsonb | 否 | 自定义变量 |
| 输出 | draft_id | int64 | 是 | 草稿ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/email-draft | 草稿列表 |
| POST | /api/email-draft | 创建草稿 |
| GET | /api/email-draft/:id | 草稿详情 |
| PUT | /api/email-draft/:id | 更新草稿 |
| DELETE | /api/email-draft/:id | 删除草稿 |
| POST | /api/email-draft/:id/preview | 预览（变量替换） |

### 3.3 安全与合规

- 草稿存储加密
- 删除前需解绑任务
- 模板审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 自动保存 | 30s 间隔 | 30s |
| 预览渲染 | < 200ms | ~80ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/email_draft.go` | 接口 |
| Service | `internal/service/email_draft_service.go` | 业务 |
| Repository | `internal/repository/email_draft_repo.go` | 数据 |
| Model | `internal/model/email_draft.go` | 模型 |
| Infra | Redis | 自动保存缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| email-jobs | 任务使用草稿 |

### 4.4 数据流向

```text
[商户] → 编辑邮件 → 自动保存（30s）
   → [email_draft_service.AutoSave]
   → 写 Redis（短期）
   → 手动保存 → 写库
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建草稿
2. 编辑主题 + 正文
3. 添加变量占位
4. 预览效果
5. 保存为正式草稿

### 5.2 系统处理流程

1. 鉴权
2. 参数校验
3. 自动保存（Redis）
4. 手动保存（DB）
5. 返回草稿ID

---

## 六、数据库设计

### 6.1 核心表 email_drafts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| subject | varchar(256) | 非空 | 主题 |
| body_html | text | 非空 | HTML |
| body_text | text | | 纯文本 |
| variables | jsonb | | 变量定义 |
| status | tinyint | 非空 | 0=草稿 1=已使用 |
| created_at | timestamp | 非空 | |
| updated_at | timestamp | 非空 | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建草稿 | 完整参数 | draft_id | ✅ |
| TC-002 | 变量预览 | `{{name}}` | 替换为变量值 | ✅ |
| TC-003 | 自动保存 | 30s 间隔 | 写 Redis | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| AUTOSAVE_INTERVAL | AUTOSAVE_INTERVAL | 30 |

---

## 九、参考资料

- [email-list-management.md](email-list-management.md)
- [email-jobs-management.md](email-jobs-management.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
