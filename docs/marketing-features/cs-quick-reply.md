# 客服快捷回复 (Customer Service Quick Reply)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cs-quick-reply`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客服快捷回复 |
| 功能名称（英文） | Customer Service Quick Reply |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P1 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（quick_replies / quick_reply_categories）
- [x] 后端 Service 与 Controller
- [x] CRUD + 分类
- [x] 关键词检索
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

客服高频使用标准化回复。提供快捷短语库，提升响应效率。

### 2.2 解决思路

- 分类管理（如：问候/产品/物流/退款/结束语）
- 关键词检索
- 变量替换（`{{customer_name}}`）
- 个人 + 公共分组

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | title | string | 是 | 标题 |
| 输入 | content | text | 是 | 内容 |
| 输入 | category | string | 否 | 分类 |
| 输入 | keywords | []string | 否 | 关键词 |
| 输入 | scope | string | 是 | personal/public |
| 输出 | reply_id | int64 | 是 | 回复ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/cs/quick-reply | 列表 |
| POST | /api/cs/quick-reply | 创建 |
| GET | /api/cs/quick-reply/:id | 详情 |
| PUT | /api/cs/quick-reply/:id | 更新 |
| DELETE | /api/cs/quick-reply/:id | 删除 |
| GET | /api/cs/quick-reply/search | 关键词搜索 |
| GET | /api/cs/quick-reply/categories | 分类列表 |
| POST | /api/cs/quick-reply/categories | 创建分类 |

### 3.3 安全与合规

- 公共回复仅管理员可创建
- 私人回复仅自己可见

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 搜索响应 | < 100ms | ~30ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cs_quick_reply.go` | 接口 |
| Service | `internal/service/cs_quick_reply_service.go` | 业务 |
| Repository | `internal/repository/cs_quick_reply_repo.go` | 数据 |
| Model | `internal/model/quick_reply.go` | 模型 |
| Infra | Redis | 搜索缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| cs-session | 会话中使用 |

### 4.3 被依赖模块

无

### 4.4 数据流向

```text
[客服] → 搜索关键词
   → [cs_quick_reply_service.Search]
   → ES / PostgreSQL 全文索引
   → 返回匹配的回复列表
   → 客服点击 → 插入会话
   → 变量替换 → 发送
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 创建快捷回复
2. 客服工作台输入 `/` 触发
3. 关键词搜索
4. 选择 → 插入会话

### 5.2 系统处理流程

1. 鉴权
2. 搜索
3. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 越权访问私人回复 | 403001 | 拒绝 |

---

## 六、数据库设计

### 6.1 核心表 quick_replies

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| user_id | bigint | FK | 创建人 |
| title | varchar(128) | 非空 | 标题 |
| content | text | 非空 | 内容 |
| category | varchar(32) | | 分类 |
| keywords | jsonb | | 关键词 |
| scope | varchar(16) | | personal/public |
| use_count | int | 默认 0 | 使用次数 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

### 6.2 核心表 quick_reply_categories

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| name | varchar(64) | 非空 | 分类名 |
| sort_order | int | 默认 0 | 排序 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建回复 | 完整参数 | reply_id | ✅ |
| TC-002 | 关键词搜索 | "价格" | 匹配回复 | ✅ |
| TC-002 | 变量替换 | `{{name}}` | 替换为客户名 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| MAX_REPLIES_PER_MERCHANT | MAX_REPLIES_PER_MERCHANT | 500 |

---

## 九、参考资料

- [cs-session.md](cs-session.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
