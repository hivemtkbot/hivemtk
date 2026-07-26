# 客服会话标签 (Session Tag)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cs-session-tag`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客服会话标签 |
| 功能名称（英文） | Customer Service Session Tag |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 数据库表结构（session_tags / session_tag_relations）
- [x] 后端 Service 与 Controller
- [x] 标签 CRUD
- [x] 会话打标 / 取消打标
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

会话标签用于归类咨询类型（投诉/咨询/建议/购买意向等），便于后续统计分析和自动化处理。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | name | string | 是 | 标签名 |
| 输入 | color | string | 否 | 颜色 |
| 输入 | category | string | 否 | 分类 |
| 输出 | tag_id | int64 | 是 | 标签ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/cs/session-tags | 标签列表 |
| POST | /api/cs/session-tags | 创建 |
| PUT | /api/cs/session-tags/:id | 更新 |
| DELETE | /api/cs/session-tags/:id | 删除 |
| POST | /api/cs/sessions/:id/tags | 会话打标 |
| DELETE | /api/cs/sessions/:id/tags/:tagId | 取消打标 |
| GET | /api/cs/sessions/:id/tags | 会话标签 |

### 3.3 安全与合规

- 仅管理员可管理标签
- 软删除

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 打标响应 | < 100ms | ~30ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cs_session_tag.go` | 接口 |
| Service | `internal/service/cs_session_tag_service.go` | 业务 |
| Repository | `internal/repository/cs_session_tag_repo.go` | 数据 |
| Model | `internal/model/session_tag.go` | 模型 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| cs-session | 会话 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 报表 | 数据来源 |

### 4.4 数据流向

```text
[客服] → 会话详情 → 选择标签
   → [cs_session_tag_service.TagSession]
   → 写 session_tag_relations
   → 通知会话更新
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 管理员创建标签
2. 客服在会话中打标
3. 报表聚合

### 5.2 系统处理流程

1. 鉴权
2. 写关联
3. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 标签已存在 | 409001 | 拒绝 |

---

## 六、数据库设计

### 6.1 核心表 session_tags

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(64) | 非空 | 标签名 |
| color | varchar(16) | | 颜色 |
| category | varchar(32) | | 分类 |
| use_count | int | 默认 0 | 使用次数 |

### 6.2 核心表 session_tag_relations

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| session_id | bigint | FK | 会话 |
| tag_id | bigint | FK | 标签 |
| tagged_at | timestamp | 非空 | |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建标签 | name | tag_id | ✅ |
| TC-002 | 会话打标 | session_id + tag_id | 200 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_TAGS_PER_MERCHANT | MAX_TAGS_PER_MERCHANT | 200 |

---

## 九、参考资料

- [cs-session.md](cs-session.md)
