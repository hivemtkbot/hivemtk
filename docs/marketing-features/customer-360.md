# 客户 360 视图 (Customer 360)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `customer-360`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客户 360 视图 |
| 功能名称（英文） | Customer 360 |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（customers / customer_basic / customer_behavior / customer_tags）
- [x] 后端 Service 与 Controller
- [x] 前端客户详情页
- [x] 客户基本信息 + 统计 + 会话 + 消息 + 标签
- [x] CDP 事件聚合
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

从线索转化为客户后，跨渠道行为数据汇总到统一客户档案（OneID），运营人员可在一个页面看到客户全貌。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | one_id | string | 是 | 统一客户 ID |
| 输出 | basic | object | 是 | 基本信息 |
| 输出 | stats | object | 是 | 统计（消费次数/金额/最后活跃） |
| 输出 | sessions | []object | 是 | 会话列表 |
| 输出 | messages | []object | 是 | 消息列表 |
| 输出 | tags | []string | 是 | 标签 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/customer-360 | 客户列表 |
| POST | /api/customer-360 | 创建客户 |
| GET | /api/customer-360/:id | 客户 360 详情 |
| GET | /api/customer-360/:id/sessions | 会话列表 |
| GET | /api/customer-360/:id/messages | 消息列表 |
| GET | /api/customer-360/:id/timeline | 时间线 |
| GET | /api/customer-360/:id/stats | 统计 |
| POST | /api/customer-360/:id/tags | 打标签 |
| DELETE | /api/customer-360/:id/tags/:tagId | 移除标签 |

### 3.3 安全与合规

- 客户隐私保护
- 手机号/邮箱脱敏
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 详情聚合 | < 500ms | ~200ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/customer360.go` | 接口 |
| Service | `internal/service/customer360_service.go` | 聚合 |
| Repository | `internal/repository/customer_*.go` | 数据 |
| Model | `internal/model/customer.go` | 模型 |
| Infra | Redis | 缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| cdp-event-tracking | 行为事件 |
| cs-session | 会话 |
| customer-tag | 标签 |
| clue-management | 线索转化 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 客服会话 | 关联客户 |
| 营销自动化 | 流程触发 |

### 4.4 数据流向

```text
[运营人员] → GET /api/customer-360/:id
   → 并行查询：基本信息 + 行为 + 会话 + 消息 + 标签
   → 数据聚合 → 返回
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入客户列表
2. 选择客户查看 360
3. 查看基本信息 / 统计 / 时间线
4. 打标签 / 触发会话

### 5.2 系统处理流程

1. 鉴权
2. 并行查询多个子表
3. 数据聚合
4. 返回

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 客户不存在 | 404001 | 提示不存在 |

---

## 六、数据库设计

### 6.1 核心表 customers（主表）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| one_id | varchar(64) | UNIQUE | 统一 ID |
| name | varchar(64) | | 姓名 |
| phone | varchar(20) | | 手机号 |
| email | varchar(128) | | 邮箱 |
| level | varchar(16) | | 客户等级 |
| total_orders | int | 默认 0 | 订单数 |
| total_amount | decimal(10,2) | | 总消费 |
| last_active_at | timestamp | | 最后活跃 |
| created_at | timestamp | 非空 | |

### 6.2 核心表 customer_basic（子表）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| customer_id | bigint | FK | 客户 |
| gender | tinyint | | 性别 |
| birthday | date | | 生日 |
| address | text | | 地址 |
| company | varchar(128) | | 公司 |
| position | varchar(64) | | 职位 |
| wechat | varchar(64) | | 微信 |
| qq | varchar(20) | | QQ |

### 6.3 核心表 customer_behavior（子表）

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| customer_id | bigint | FK | 客户 |
| event_type | varchar(32) | | 事件类型 |
| event_data | jsonb | | 事件数据 |
| source | varchar(32) | | 来源 |
| occurred_at | timestamp | 非空 | 发生时间 |

### 6.4 核心表 customer_tags

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| customer_id | bigint | FK | 客户 |
| tag_id | int | | 标签 ID |
| tagged_at | timestamp | 非空 | 打标时间 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 客户 360 详情 | one_id | 聚合数据 | ✅ |
| TC-002 | 时间线查询 | 时间范围 | 事件列表 | ✅ |
| TC-003 | 打标签 | tag_id | 200 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| TIMELINE_PAGE_SIZE | TIMELINE_PAGE_SIZE | 50 |

---

## 九、参考资料

- [cdp-event-tracking.md](cdp-event-tracking.md)
- [cs-session.md](cs-session.md)
- [clue-management.md](clue-management.md)
