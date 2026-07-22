# 短信草稿 (SMS Draft)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `sms-draft-management`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 短信草稿管理 |
| 功能名称（英文） | SMS Draft Management |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | sms |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（sms_drafts）
- [x] 后端 Service 与 Controller
- [x] 模板变量 + 字符数统计
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

短信发送前需要编辑内容。提供模板变量和字符数统计（70 字/条计费）。

### 2.2 解决思路

- 模板变量（`{{name}}` `{{code}}`）
- 字符数统计：纯中文算 1 字符/2 字节，英文/数字算 1 字符/1 字节
- 70 字/条，超出分条计费
- 自动保存

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 草稿名 |
| 输入 | content | text | 是 | 短信内容 |
| 输入 | variables | jsonb | 否 | 变量定义 |
| 输出 | draft_id | int64 | 是 | 草稿ID |
| 输出 | char_count | int | 是 | 字符数 |
| 输出 | segment_count | int | 是 | 计费条数 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/sms-draft | 草稿列表 |
| POST | /api/sms-draft | 创建 |
| GET | /api/sms-draft/:id | 详情 |
| PUT | /api/sms-draft/:id | 更新 |
| DELETE | /api/sms-draft/:id | 删除 |

### 3.3 安全与合规

- 字符数限制 500
- 退订关键字提示

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 字符数统计 | < 50ms | ~10ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/sms_draft.go` | 接口 |
| Service | `internal/service/sms_draft_service.go` | 业务 |
| Repository | `internal/repository/sms_draft_repo.go` | 数据 |
| Model | `internal/model/sms_draft.go` | 模型 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| sms-jobs | 任务使用草稿 |

### 4.4 数据流向

```text
[商户] → 编辑短信
   → 字符数实时统计
   → 自动保存 → 写库
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建草稿
2. 填写内容
3. 查看字符数 + 计费条数
4. 保存

### 5.2 系统处理流程

1. 鉴权
2. 字符数统计
3. 写库

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 字符数超限 | 400101 | 拒绝保存 |

---

## 六、数据库设计

### 6.1 核心表 sms_drafts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(128) | 非空 | 草稿名 |
| content | text | 非空 | 内容 |
| variables | jsonb | | 变量 |
| char_count | int | | 字符数 |
| segment_count | int | | 计费条数 |
| status | tinyint | 非空 | 0=草稿 1=已使用 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 字符数统计 | 70 中文字符 | segment=1 | ✅ |
| TC-002 | 变量替换 | `{{code}}` | 替换值 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| SMS_MAX_CHARS | SMS_MAX_CHARS | 500 |
| SMS_SEGMENT_SIZE | SMS_SEGMENT_SIZE | 70 |

---

## 九、参考资料

- sms-config.md
- [sms-jobs-management.md](sms-jobs-management.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
