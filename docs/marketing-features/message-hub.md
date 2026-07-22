# 消息中心 (Message Hub)

> **所属模块**: unified-message
> **功能 slug**: `messageHub`
> **文档定位**: 站内消息中心，系统通知/任务提醒/客户消息汇总，支持已读/未读/分类/批量操作。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 消息中心 |
| 功能名称(英文) | Message Hub |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | unified-message |
| 优先级 | P1 |
| 实际完成时间 | 2026-07-15 |
| 最后更新 | 2026-07-22 |

### 1.1 已完成内容
- [x] 站内消息中心（系统通知/任务提醒/客户消息汇总）
- [x] `setupMessageRoutes` 路由注册
- [x] `internal/controller/message_hub_controller.go` 后端控制器
- [x] 已读/未读状态管理
- [x] 消息分类与筛选
- [x] 批量操作（标记已读/删除）
- [x] 未读消息数实时推送（SSE）
- [x] 前端 `user-web/src/views/messageHub/List.vue` 消息中心

### 1.2 待完成内容
- [ ] 消息智能聚合（同类消息合并）

### 1.3 阻塞项
| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景
系统运行中会产生大量需要用户关注的消息（系统通知、任务提醒、客户咨询汇总等）。散落在各模块的消息会让用户遗漏重要信息，需要统一的消息中心集中呈现，支持分类、已读管理、批量操作。

### 2.2 解决思路
各业务模块通过事件总线发布消息到消息中心，按类型分类存储；前端提供统一消息列表，支持按类型筛选、已读/未读切换、批量操作；未读数通过 SSE 实时推送至顶栏徽标。

### 2.3 关键算法或模型
- 消息分类：按 type 字段分类（system/task/customer）
- 未读数推送：SSE 长连接，事件驱动推送
- 消息聚合：同类消息按时间窗口聚合（可选）

### 2.4 输入输出定义
| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | type | string | 否 | 类型筛选 |
| 输入 | read_status | string | 否 | 已读状态筛选 |
| 输出 | message_id | int64 | 是 | 消息 ID |
| 输出 | type | string | 是 | 消息类型 |
| 输出 | title | string | 是 | 标题 |
| 输出 | content | string | 是 | 内容 |
| 输出 | recipient_id | int64 | 是 | 接收人 ID |
| 输出 | read_status | string | 是 | 已读状态 |
| 输出 | created_at | timestamp | 是 | 创建时间 |

---

## 三、设计标准
### 3.1 遵循的规范
- 五层架构：[ARCHITECTURE_DIAGRAM.md](../architecture/ARCHITECTURE_DIAGRAM.md)
- 后端编码规范：controller→service→repository 分层
- 前端编码规范：Vue 3 + Element Plus + Pinia

### 3.2 性能指标
- 消息列表查询 < 300ms
- 未读数推送延迟 < 1s
- 批量操作 < 500ms

---

## 四、API 接口
| 方法 | 路径 | 描述 | 鉴权 |
|---|---|---|---|
| GET | /api/message-hub/list | 消息列表（分页 + 筛选） | JWT |
| GET | /api/message-hub/unread-count | 未读消息数 | JWT |
| GET | /api/message-hub/:id | 消息详情 | JWT |
| PUT | /api/message-hub/:id/read | 标记已读 | JWT |
| PUT | /api/message-hub/batch-read | 批量标记已读 | JWT |
| DELETE | /api/message-hub/:id | 删除消息 | JWT |
| GET | /api/message-hub/sse | SSE 未读数推送 | JWT |

---

## 五、数据模型
### 5.1 数据库表
| 表名 | 说明 |
|---|---|
| messages | 消息主表 |
| message_recipients | 消息-接收人关联表 |
| message_read_status | 已读状态表 |

### 5.2 关键字段
| 字段 | 类型 | 说明 |
|---|---|---|
| message_id | bigint | 消息 ID |
| type | varchar(32) | 类型（system/task/customer） |
| title | varchar(128) | 标题 |
| content | text | 内容 |
| recipient_id | bigint | 接收人 ID |
| read_status | varchar(16) | 已读状态（read/unread） |
| created_at | timestamp | 创建时间 |

---

## 六、业务流程
### 6.1 主流程
1. 业务模块通过事件总线发布消息
2. 消息中心订阅事件，写入 messages 与 message_recipients
3. SSE 推送未读数变化至对应用户
4. 用户在消息中心查看、筛选、标记已读
5. 支持批量操作与删除

### 6.2 异常处理
- 事件总线消息丢失：兜底定时扫描补发
- SSE 连接断开：客户端自动重连，重连后拉取最新未读数
- 批量操作超时：分批处理

---

## 七、前端交互
### 7.1 页面清单
| 页面 | 路由 | 视图组件 |
|---|---|---|
| 消息中心 | /message-hub | messageHub/List.vue |

### 7.2 关键交互
- 顶栏消息图标 + 未读数徽标（SSE 实时更新）
- 消息列表表格（类型、标题、内容摘要、时间、已读状态）
- 左侧分类 Tab（全部/系统/任务/客户）
- 批量选择 + 批量标记已读/删除
- 点击消息查看详情并自动标记已读

---

## 八、测试策略
### 8.1 单元测试
- 消息分类与筛选单测
- 已读状态管理单测
- 批量操作逻辑单测

### 8.2 集成测试
- 事件总线消息发布/订阅端到端测试
- SSE 未读数推送测试
- 高并发下消息写入测试

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
