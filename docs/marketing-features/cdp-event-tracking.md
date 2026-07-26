# 客户事件追踪 CDP (CDP Event Tracking)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cdp-event-tracking`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客户事件追踪 CDP |
| 功能名称（英文） | Customer Data Platform Event Tracking |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | cdp |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（cdp_events / cdp_event_stats）
- [x] 后端 Service 与 Controller
- [x] 事件类型：浏览/点击/购买/注册/登录/加购
- [x] 事件追踪 SDK（JS 嵌入）
- [x] 历史查询 + 统计
- [x] 单元测试 / 集成测试

---

## 二、核心原理

### 2.1 业务背景

CDP（Customer Data Platform）采集客户在 Web/App/小程序上的全量行为数据，构建统一客户画像。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | one_id | string | 是 | 客户 ID |
| 输入 | event_type | string | 是 | 事件类型 |
| 输入 | event_data | jsonb | 否 | 事件数据 |
| 输入 | source | string | 否 | 来源 |
| 输出 | event_id | int64 | 是 | 事件ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/cdp/track | 单事件上报（公开） |
| POST | /api/cdp/track/batch | 批量上报（公开） |
| GET | /api/cdp/events | 事件列表 |
| GET | /api/cdp/events/:id | 事件详情 |
| GET | /api/cdp/one_id/:id/history | 客户历史 |
| GET | /api/cdp/stats | 统计 |

### 3.3 安全与合规

- SDK 鉴权（app_key）
- PII 加密
- 同意授权
- 7 天后匿名化

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 事件上报 | < 100ms | ~30ms |
| 批量上报 | 100 条/批 | 100 条/批 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cdp.go` | 接口 |
| Service | `internal/service/cdp_service.go` | 业务 |
| Repository | `internal/repository/cdp_repo.go` | 数据 |
| Model | `internal/model/cdp_event.go` | 模型 |
| Infra | Kafka + Redis | 消息队列+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| customer-360 | 客户聚合 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 行为触发 |
| 报表 | 数据来源 |

### 4.4 数据流向

```text
[Web/App SDK] → POST /api/cdp/track
   → 鉴权 → 解析事件
   → Kafka 异步队列
   → 批量入库 cdp_events
   → 客户聚合更新
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 商户嵌入 JS SDK
2. 客户行为自动触发事件
3. 后台查看事件流
4. 多维度分析

### 5.2 系统处理流程

1. 接收事件
2. 鉴权
3. 写入 Kafka
4. 异步消费入库
5. 客户档案更新

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| SDK 鉴权失败 | 401001 | 拒绝 |
| 事件格式错误 | 400101 | 丢弃 |

---

## 六、数据库设计

### 6.1 核心表 cdp_events

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| one_id | varchar(64) | 非空 | 客户 ID |
| event_type | varchar(32) | 非空 | 事件类型 |
| event_data | jsonb | | 事件数据 |
| source | varchar(32) | | 来源 |
| ip | varchar(64) | | IP |
| ua | varchar(255) | | UA |
| occurred_at | timestamp | 非空 | 发生时间 |
| created_at | timestamp | 非空 | 入库时间 |

### 6.2 核心表 cdp_event_stats

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| event_type | varchar(32) | | 事件类型 |
| stat_date | date | 非空 | 日期 |
| count | int | 默认 0 | 次数 |
| unique_users | int | | UV |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_cdp_event_one | one_id, occurred_at | btree | 客户历史 |
| idx_cdp_event_type | event_type | btree | 类型查询 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 事件上报 | 完整事件 | event_id | ✅ |
| TC-002 | 批量上报 | 100 事件 | 200 | ✅ |
| TC-003 | 客户历史 | one_id | 事件流 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| KAFKA_TOPIC | KAFKA_TOPIC | cdp-events |
| BATCH_SIZE | BATCH_SIZE | 100 |

---

## 九、参考资料

- [customer-360.md](customer-360.md)
