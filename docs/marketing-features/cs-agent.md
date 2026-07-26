# 客服代理 (Customer Service Agent)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cs-agent`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客服代理 |
| 功能名称（英文） | Customer Service Agent |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（cs_agents）
- [x] 后端 Service 与 Controller
- [x] 创建客服 / 在线状态
- [x] 上下线 / 会话列表
- [x] 技能标签（用于自动分配）
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

客服代理是会话的实际处理人。需要管理客服账号、在线状态、技能、负载。

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | user_id | bigint | 是 | 关联用户 |
| 输入 | nickname | string | 是 | 客服昵称 |
| 输入 | skills | []string | 否 | 技能标签 |
| 输入 | max_sessions | int | 默认 5 | 最大并发 |
| 输出 | agent_id | int64 | 是 | 客服ID |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/cs/agents | 客服列表 |
| POST | /api/cs/agents | 创建客服 |
| GET | /api/cs/agents/:id | 详情 |
| PUT | /api/cs/agents/:id | 更新 |
| DELETE | /api/cs/agents/:id | 删除 |
| PUT | /api/cs/agents/:id/status | 上下线 |
| PUT | /api/cs/agents/:id/heartbeat | 心跳 |
| GET | /api/cs/agents/:id/sessions | 客服会话列表 |
| GET | /api/cs/agents/online | 在线客服列表 |

### 3.3 安全与合规

- 仅管理员可创建客服
- 状态变更审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 心跳响应 | < 50ms | ~10ms |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cs_agent.go` | 接口 |
| Service | `internal/service/cs_agent_service.go` | 业务 |
| Repository | `internal/repository/cs_agent_repo.go` | 数据 |
| Model | `internal/model/cs_agent.go` | 模型 |
| Infra | Redis | 状态缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| user-management | 关联用户 |
| cs-session | 会话分配 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| cs-session | 自动分配 |

### 4.4 数据流向

```text
[WebSocket 连接] → 心跳
   → [cs_agent_service.Heartbeat]
   → Redis 写 agent:online:{id} = ttl 60s
   → 60s 没心跳 → 自动离线
   → 自动分配时检查在线状态 + 负载
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 管理员创建客服
2. 客服登录工作台
3. 自动上线
4. 接收会话
5. 关闭浏览器 → 自动离线

### 5.2 系统处理流程

1. 心跳接收
2. 状态更新
3. 过期自动离线

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 心跳超时 | - | 标记离线 |
| 负载已满 | - | 暂停分配 |

---

## 六、数据库设计

### 6.1 核心表 cs_agents

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| user_id | bigint | FK | 关联用户 |
| nickname | varchar(64) | 非空 | 昵称 |
| avatar | varchar(512) | | 头像 |
| skills | jsonb | | 技能标签 |
| max_sessions | int | 默认 5 | 最大并发 |
| current_sessions | int | 默认 0 | 当前会话数 |
| status | varchar(16) | | online/offline/busy/away |
| last_heartbeat_at | timestamp | | 最后心跳 |
| created_at | timestamp | 非空 | |

### 6.2 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_csagent_user | user_id | UNIQUE | 用户唯一 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建客服 | user_id | agent_id | ✅ |
| TC-002 | 状态切换 | online | status 更新 | ✅ |
| TC-003 | 心跳 | 60s 间隔 | 在线保持 | ✅ |
| TC-004 | 超时离线 | 60s 无心跳 | status=offline | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| HEARTBEAT_TTL | HEARTBEAT_TTL | 60 |
| HEARTBEAT_INTERVAL | HEARTBEAT_INTERVAL | 30 |

---

## 九、参考资料

- [cs-session.md](cs-session.md)
- [user-management.md](user-management.md)
