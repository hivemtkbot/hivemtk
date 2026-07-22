# 客服会话 (Customer Service Session)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `cs-session`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 客服会话 |
| 功能名称（英文） | Customer Service Session |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | customer-service |
| 优先级 | P0 |
| 最后更新 | 2026-07-14 |

### 1.1 已完成内容

- [x] 数据库表结构（cs_sessions / cs_messages）
- [x] 后端 Service 与 Controller
- [x] 会话管理（创建/分配/关闭）
- [x] 消息收发（WebSocket）
- [x] 自动分配 + 评分 + 转接 + 标签
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

多渠道（企微/抖音/网页/邮件/短信）客户咨询统一进入客服工作台。提供会话分配、消息收发、评分、转接、标签能力。

### 2.2 解决思路

- 渠道归一：所有消息进 cs_messages
- 自动分配：基于客服技能 + 在线状态 + 负载
- 实时消息：WebSocket
- 评分：会话结束客户打分
- 转接：会话在客服间转移

### 2.3 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | customer_id | bigint | 是 | 客户 |
| 输入 | channel | string | 是 | 渠道 |
| 输入 | initial_message | text | 是 | 首条消息 |
| 输出 | session_id | int64 | 是 | 会话ID |
| 输出 | agent_id | bigint | 是 | 分配的客服 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/cs/sessions | 会话列表 |
| POST | /api/cs/sessions | 创建会话 |
| GET | /api/cs/sessions/:id | 会话详情 |
| PUT | /api/cs/sessions/:id/assign | 分配客服 |
| PUT | /api/cs/sessions/:id/transfer | 转接 |
| PUT | /api/cs/sessions/:id/close | 关闭 |
| POST | /api/cs/sessions/:id/messages | 发送消息 |
| GET | /api/cs/sessions/:id/messages | 消息列表 |
| POST | /api/cs/sessions/:id/rate | 评分 |
| POST | /api/cs/sessions/:id/tags | 打标签 |

### 3.3 安全与合规

- 会话加密
- 内容审核
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 消息延迟 | < 500ms | ~150ms |
| 并发会话 | 1000+ | 1500 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/cs_session.go` | 接口 |
| Service | `internal/service/cs_session_service.go` | 业务 |
| Repository | `internal/repository/cs_repo.go` | 数据 |
| Model | `internal/model/cs_session.go` | 模型 |
| Infra | WebSocket + Redis | 实时+缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| customer-360 | 客户信息 |
| cs-agent | 客服 |
| cs-quick-reply | 快捷回复 |
| cs-ai-suggest | AI 建议 |
| rag-customer-service | 智能回复 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发 |
| 报表 | 数据来源 |

### 4.4 数据流向

```text
[渠道消息] → 接收 → 写入 cs_messages
   → 检查是否有打开会话
   → 无 → 创建会话 → 自动分配客服
   → 有 → 关联到现有会话
   → 推送 WebSocket 给客服
   → 客服回复 → 实时推送
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 客户发起咨询（多渠道）
2. 自动分配客服
3. 客服工作台看到会话
4. 实时收发消息
5. 关闭会话（客户/客服/超时）
6. 客户评分

### 5.2 系统处理流程

1. 接收消息
2. 查找/创建会话
3. 自动分配客服
4. 实时推送
5. 消息存储
6. 会话状态更新

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 客服全忙 | - | 加入排队 |
| WebSocket 断线 | - | 自动重连 |
| 客户失联 | - | 30 分钟超时关闭 |

---

## 六、数据库设计

### 6.1 核心表 cs_sessions

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| customer_id | bigint | FK | 客户 |
| channel | varchar(32) | 非空 | 渠道 |
| agent_id | bigint | FK | 客服 |
| status | varchar(16) | | waiting/active/closed |
| priority | varchar(16) | | low/normal/high |
| rating | int | | 评分 1-5 |
| rating_comment | text | | 评分备注 |
| tags | jsonb | | 标签 |
| started_at | timestamp | | 开始时间 |
| closed_at | timestamp | | 关闭时间 |
| last_message_at | timestamp | | 最后消息 |

### 6.2 核心表 cs_messages

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| session_id | bigint | FK | 会话 |
| sender_type | varchar(16) | | customer/agent/system |
| sender_id | bigint | | 发送者 |
| msg_type | varchar(16) | | text/image/file |
| content | text | 非空 | 内容 |
| sent_at | timestamp | 非空 | |

### 6.3 索引

| 索引名 | 字段 | 类型 | 说明 |
|---|---|---|---|
| idx_cs_session_agent | agent_id, status | btree | 客服维度 |
| idx_cs_session_customer | customer_id | btree | 客户维度 |

---

## 七、测试说明

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 创建会话 | 客户 + 渠道 | session_id | ✅ |
| TC-002 | 自动分配 | 等待 | agent_id | ✅ |
| TC-003 | 实时消息 | 文本 | 收到推送 | ✅ |
| TC-004 | 转接 | 目标 agent | 200 | ✅ |
| TC-005 | 评分 | 1-5 分 | rating 更新 | ✅ |

---

## 八、部署与运维

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| WS_PING_INTERVAL | WS_PING_INTERVAL | 30 |
| SESSION_TIMEOUT_MIN | SESSION_TIMEOUT_MIN | 30 |
| MAX_SESSIONS_PER_AGENT | MAX_SESSIONS_PER_AGENT | 10 |

---

## 九、参考资料

- [customer-360.md](customer-360.md)
- [cs-agent.md](cs-agent.md)
- [cs-quick-reply.md](cs-quick-reply.md)
- [cs-ai-suggest.md](cs-ai-suggest.md)

---

## 十、版本历史

| 版本 | 日期 | 变更内容 | 作者 |
|---|---|---|---|
| v1.0 | 2026-07-14 | 独立功能文档初始版本 | AI Assistant |
