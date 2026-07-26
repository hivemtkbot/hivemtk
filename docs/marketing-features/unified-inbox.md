# 统一收件箱 (Unified Inbox)

> **所属模块**: reach-center
> **功能 slug**: `unifiedInbox`
> **文档定位**: 多渠道消息（企微/WhatsApp/Telegram/飞书/网站客服）聚合到统一收件箱，客服在一个工作台回复所有渠道消息。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 统一收件箱 |
| 功能名称(英文) | Unified Inbox |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | reach-center |
| 优先级 | P0 |

### 1.1 已完成内容
- [x] 多渠道消息聚合（企微/WhatsApp/Telegram/飞书/网站客服）
- [x] 统一收件箱工作台
- [x] 消息中台 MQ（自研），多账号聚合时延 < 3 秒
- [x] 按 agent_id 分发消息
- [x] `internal/controller/inbox_controller.go`
- [x] 前端统一收件箱列表页面
- [x] 单元测试与集成测试

### 1.2 待完成内容
- [ ] 收件箱智能分流（按客户优先级自动排序）
- [ ] 跨渠道会话合并（同 OneID 客户）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
商户在多渠道（企微/WhatsApp/Telegram/飞书/网站客服）部署账号后，客服需要在多个平台切换回复消息，效率低下。统一收件箱将所有渠道消息聚合到一个工作台，客服集中处理。

### 2.3 关键算法或模型
- 消息中台 MQ（自研）订阅/发布模型
- 多账号并发聚合
- 消息时序排序（按 timestamp）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | channel_type | string | 是 | 渠道类型 |
| 输入 | from_user | string | 是 | 发送方 |
| 输入 | to_account | string | 是 | 接收账号 |
| 输入 | content | string | 是 | 消息内容 |
| 输入 | timestamp | int64 | 是 | 时间戳 |
| 输出 | message_id | int64 | 是 | 消息 ID |
| 输出 | agent_id | int64 | 否 | 分发的智能体 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 多账号聚合时延 < 3 秒
- 消息投递可靠性 ≥ 99.9%
- 单工作台并发消息处理 ≥ 100 QPS

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/inbox/messages | 收件箱消息列表 | JWT |
| GET | /api/inbox/messages/:id | 消息详情 | JWT |
| POST | /api/inbox/messages/:id/reply | 回复消息 | JWT |
| POST | /api/inbox/messages/:id/assign | 分配座席 | JWT |
| GET | /api/inbox/channels/status | 各渠道连接状态 | JWT |
| WS | /api/inbox/stream | 实时消息推送 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| inbox_messages | 统一收件箱消息 |
| inbox_assignments | 消息分配记录 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| message_id | bigint | 消息 ID |
| channel_type | varchar(32) | 渠道类型 |
| from_user | varchar(128) | 发送方 |
| to_account | varchar(128) | 接收账号 |
| content | text | 消息内容 |
| timestamp | bigint | 时间戳 |
| agent_id | bigint | 分发的智能体 |

---

## 六、业务流程
### 6.1 主流程
1. 各渠道账号接入消息中台 MQ
2. MQ 消费者将消息标准化写入 inbox_messages
3. 按 to_account 查找绑定的 agent_id
4. WebSocket 推送到对应座席工作台
5. 座席在统一收件箱回复消息
6. 回复经消息中台路由到原渠道发送

### 6.2 异常处理
- 渠道连接断开：标记渠道离线，自动重连
- 消息投递失败：MQ 重试，超限后死信队列
- 座席未响应：超时自动转接其他座席或智能体

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 统一收件箱 | /unifiedInbox/list | unifiedInbox/List.vue |
| 渠道状态看板 | /unifiedInbox/status | unifiedInbox/Status.vue |

### 7.2 关键交互
- 收件箱按渠道分组/混合排序切换
- 消息列表实时刷新（WebSocket）
- 回复框支持富文本与快捷回复
- 渠道状态看板展示各渠道连接健康度

---

## 八、测试策略
### 8.1 单元测试
- 消息标准化 service 单测
- agent_id 分发单测
- 消息时序排序单测

### 8.2 集成测试
- 多渠道消息→聚合→工作台展示全链路
- 座席回复→路由到原渠道全链路
- 渠道断连重连验证
