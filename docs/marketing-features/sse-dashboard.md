# SSE 实时驾驶舱 (SSE Dashboard)

> **所属模块**: system-management
> **功能 slug**: `sseDashboard`
> **文档定位**: Server-Sent Events 实时推送驾驶舱数据，无需 WebSocket 升级。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | SSE 实时驾驶舱 |
| 功能名称(英文) | SSE Dashboard |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | system-management |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] Server-Sent Events 实时推送（无需 WebSocket 升级）
- [x] 活跃会话数实时推送
- [x] 消息量实时推送
- [x] 智能体状态实时推送
- [x] `setupSSEDashboardRoutes` 路由注册
- [x] `internal/controller/sse_dashboard.go` + `dashboard_sse.go` 后端控制器
- [x] 客户端连接管理（client_id）与心跳保活
- [x] 多事件类型订阅（event_type）

### 1.2 待完成内容
- [ ] SSE 连接负载均衡优化

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
运营驾驶舱需要实时展示活跃会话数、消息量、智能体状态等动态指标。相比 WebSocket 需要协议升级、有兼容性问题，Server-Sent Events（SSE）基于 HTTP 长连接，更轻量、对基础设施友好，适合服务端单向推送场景。

### 2.2 解决思路
后端建立 SSE 长连接，按 client_id 管理客户端；订阅事件总线获取实时指标变更，按 event_type 推送至已订阅的客户端；客户端通过 EventSource API 接收并更新驾驶舱视图；心跳保活防止连接断开。

### 2.3 关键算法或模型
- 事件总线订阅：按 event_type 订阅（sessions/messages/agents）
- 客户端管理：client_id 注册 + 连接池
- 心跳保活：每 30 秒发送 ping 事件
- 背压控制：客户端消费慢时丢弃旧事件

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | event_types | array | 否 | 订阅的事件类型 |
| 输出 | event_type | string | 是 | 事件类型（sessions/messages/agents） |
| 输出 | payload | object | 是 | 事件数据 |
| 输出 | timestamp | timestamp | 是 | 事件时间戳 |
| 输出 | client_id | string | 是 | 客户端 ID |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 事件推送延迟 < 500ms
- 单实例支持 ≥ 1000 SSE 连接
- 心跳保活 30s 间隔

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/sse-dashboard/stream | SSE 实时推送长连接 | JWT |
| GET | /api/sse-dashboard/snapshot | 当前快照数据（首次加载） | JWT |
| GET | /api/sse-dashboard/connections | 连接数统计 | JWT |
| POST | /api/sse-dashboard/broadcast | 广播事件（管理接口） | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| sse_connections | SSE 连接记录表 |
| sse_event_logs | 事件推送日志表 |
| dashboard_snapshots | 驾驶舱快照表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| event_type | varchar(32) | 事件类型（sessions/messages/agents） |
| payload | jsonb | 事件数据 |
| timestamp | timestamp | 事件时间戳 |
| client_id | varchar(64) | 客户端 ID |

---

## 六、业务流程
### 6.1 主流程
1. 前端 EventSource 连接 `/stream` 接口，携带订阅的 event_types
2. 后端注册 client_id，加入连接池
3. 事件总线收到指标变更，按 event_type 推送至订阅客户端
4. 每 30 秒发送心跳保活
5. 前端接收事件，增量更新驾驶舱视图
6. 连接断开时清理 client_id

### 6.2 异常处理
- 连接断开：客户端自动重连，重连后拉取快照
- 推送队列积压：丢弃旧事件，保留最新
- 客户端消费慢：背压控制，避免内存溢出
- 实例重启：客户端重连至其他实例

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| SSE 实时驾驶舱 | /sse-dashboard | sseDashboard/List.vue |

### 7.2 关键交互
- 活跃会话数实时数字滚动
- 消息量实时折线图（最近 5 分钟）
- 智能体状态卡片（在线/忙碌/离线）
- 连接状态指示器（已连接/重连中/断开）
- 事件类型订阅切换

---

## 八、测试策略
### 8.1 单元测试
- 客户端连接管理单测
- 事件类型订阅与推送单测
- 心跳保活逻辑单测

### 8.2 集成测试
- SSE 端到端推送测试（事件总线→SSE→客户端）
- 高并发连接下稳定性测试
- 连接断开与重连测试

---

## 九、版本历史
| 版本 | 日期 | 变更说明 |
|---|---|---|
| v1.0 | 2026-07-15 | 初始实现 |
| v1.1 | 2026-07-22 | 补充功能文档 |

---

## 十、相关文档
- [../INDEX.md](../INDEX.md)
- [../architecture/ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- [../CROSS_COMPARISON_REPORT.md](../CROSS_COMPARISON_REPORT.md)
