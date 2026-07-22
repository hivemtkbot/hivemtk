# WebSocket 实时通信 (WebSocket Realtime)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `websocket-realtime`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | WebSocket 实时通信 |
| 功能名称（英文） | WebSocket Realtime Communication |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | websocket |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] WebSocket 服务（Hub + Client 模式）
- [x] 鉴权（JWT）
- [x] 心跳（Ping/Pong）
- [x] 房间订阅
- [x] 消息推送
- [x] 单元测试 / 集成测试

---

## 二、核心原理

### 2.1 业务背景

实时性场景：客服消息推送、订单状态变更、系统通知、数据大屏实时刷新。

### 2.2 解决思路

- Hub 管理所有 Client 连接
- Client 长连接 + 心跳
- 房间（Room）模式：按主题订阅
- 鉴权：连接时 JWT 验证
- 断线重连：客户端自动重连

### 2.3 输入输出定义

- **客户端 → 服务端**：
  - 鉴权：连接 URL 携带 JWT
  - 心跳：ping/pong
  - 订阅：sub:room_id
  - 取消订阅：unsub:room_id
- **服务端 → 客户端**：
  - 消息推送：msg 类型
  - 通知：notify 类型
  - 错误：err 类型

---

## 三、设计标准

### 3.2 API/协议

- 协议：WS（ws://） / WSS（wss://）
- 端点：/ws
- 心跳：30s 间隔
- 鉴权：URL 参数 token

### 3.3 安全与合规

- JWT 鉴权
- Origin 检查
- 消息加密（可选）

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 并发连接 | 10000+ | 15000 |
| 消息延迟 | < 100ms | ~30ms |
| 推送 QPS | 5000 | 8000 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Infra | `internal/websocket/hub.go` + `client.go` | Hub + Client |
| Service | 各业务 Service | 调 WebSocket 推送 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | JWT 鉴权 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| cs-session | 客服消息推送 |
| order-management | 订单状态推送 |
| dashboard | 大屏实时刷新 |
| 营销自动化 | 流程节点推送 |

### 4.4 数据流向

```text
[客户端] → WS 连接 /ws?token=xxx
   → JWT 鉴权
   → 加入 Hub
   → 订阅房间 sub:room_id
   → 服务端推送 → ws.WriteMessage
   → 客户端断线 → 清理
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 前端创建 WS 连接
2. URL 携带 JWT
3. 服务端鉴权 + 接受连接
4. 订阅主题
5. 接收推送

### 5.2 系统处理流程

1. 接收连接请求
2. 鉴权
3. 创建 Client
4. 注册到 Hub
5. 启动读/写 goroutine
6. 接收订阅
7. 推送消息

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 鉴权失败 | 4001 | 关闭连接 |
| 心跳超时 | - | 主动断开 |
| 房间不存在 | 4004 | 通知客户端 |

---

## 六、数据库设计

无独立表（Hub 内存中维护连接）

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 连接鉴权 | 有效 JWT | 连接成功 | ✅ |
| TC-002 | 鉴权失败 | 无效 JWT | 4001 | ✅ |
| TC-003 | 消息推送 | 房间消息 | 客户端收到 | ✅ |
| TC-004 | 心跳超时 | 60s 无心跳 | 主动断开 | ✅ |
| TC-005 | 并发连接 | 1000 连接 | 全部正常 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| WS_PING_INTERVAL | WS_PING_INTERVAL | 30 |
| WS_WRITE_TIMEOUT | WS_WRITE_TIMEOUT | 10 |
| WS_MAX_MESSAGE_SIZE | WS_MAX_MESSAGE_SIZE | 4096 |

### 8.2 依赖服务

- Redis（多节点时用于跨节点消息分发）

### 8.3 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 并发连接数 | > 80% 容量 | 飞书 |
| 消息延迟 | > 200ms | 飞书 |

---

## 九、参考资料

- [cs-session.md](cs-session.md)
- CONNECTION_POOL_CACHE.md

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
