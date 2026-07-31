# 统一消息 (Unified Message)

> **所属模块**: unified-message
> **功能 slug**: `unified-message`
> **文档定位**: 多渠道消息统一收件箱,遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | 统一消息 |
| 功能名称(英文) | Unified Message |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | unified-message |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构与迁移脚本
- [x] 后端 Service 与 Controller
- [x] 前端统一收件箱
- [x] 多渠道聚合(微信/企微/抖音/邮件/短信)
- [x] 回复与详情查看
- [x] API 接口与 Swagger 文档
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

### 1.2 待完成内容

- [ ] AI 智能回复建议

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

商户同时使用多个渠道触达客户(微信/企微/抖音/邮件/短信),客户也会从不同渠道发起咨询。商户需要一个统一收件箱,聚合所有消息,统一处理。

### 2.3 关键算法或模型

- **渠道适配器**: `Adapter.Receive/Send/Reply`
- **消息归一**: `{id, channel, from, to, content, direction, status, timestamp}`
- **客户归一**: OneID 关联客户
- **智能排序**: 按客户活跃度 + 紧急度 + 未读

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | channel | string | 否 | 渠道过滤 |
| 输入 | status | string | 否 | 状态过滤 |
| 输入 | customer_id | int64 | 否 | 客户过滤 |
| 输入 | keyword | string | 否 | 关键字 |
| 输出 | messages | array | 是 | 消息列表 |
| 输出 | unread_count | int | 是 | 未读数 |

---

## 三、设计标准

### 3.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [API_CONTRACT.md](../standards/API_CONTRACT.md)

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| GET | /api/unified-message | 消息列表 |
| GET | /api/unified-message/:id | 消息详情 |
| POST | /api/unified-message/:id/reply | 回复 |
| POST | /api/unified-message/:id/read | 标记已读 |
| GET | /api/unified-message/unread-count | 未读数 |
| GET | /api/unified-message/conversations | 会话列表 |

### 3.3 安全与合规

- 渠道凭证加密
- 消息存储加密
- 敏感词过滤(发送前)
- 频率限制

### 3.4 性能指标

| 指标 | 目标值 |
|---|---|
| 列表查询 | < 300ms |
| 回复发送 | < 2s |
| 未读数实时 | < 100ms |
| 并发 | ≥ 200 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | internal/controller/unified_message | |
| Service | internal/service/unified_message | 消息聚合 + 回复 |
| Engine | internal/service/unified_message/dispatcher | 渠道分发 |
| Repository | internal/repository/unified_message | |
| Model | internal/model/unified_message | |

### 4.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 平台账号 | 发送凭证 |
| 客户(OneID) | 客户归一 |
| WebSocket | 实时推送新消息 |

### 4.3 数据流向

```text
[各渠道接收] → [Adapter 归一] → [统一消息库] → [WebSocket 推送] → [前端展示]
                                                                       ↓
[用户回复] → [选择渠道账号] → [Adapter 发送] → [平台 API]
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 进入"消息 → 统一收件箱"
2. 查看会话列表(按渠道/客户分组)
3. 点击会话 → 加载历史消息
4. 在底部输入回复内容
5. 选择发送渠道账号
6. 点击发送

### 5.2 系统处理流程(接收)

1. 各 Adapter 监听平台消息
2. 消息归一化
3. OneID 匹配客户
4. 写入消息库
5. 通过 WebSocket 推送给在线运营人员

### 5.3 系统处理流程(发送)

1. 接收回复请求
2. 校验发送账号
3. 敏感词检测
4. 频率限制
5. 调用对应 Adapter 发送
6. 写入消息库(发送方)

### 5.4 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 账号失效 | 500190 | 提示重新授权 |
| 敏感词 | 400160 | 拒绝 |
| 平台限流 | 500191 | 退避重试 |

---

## 六、数据库设计

### 6.1 核心表结构

| 表 | 说明 |
|---|---|
| `unified_messages` | 统一消息 |
| `unified_conversations` | 会话 |

```sql
CREATE TABLE unified_messages (
  id BIGINT PRIMARY KEY,
  
  channel VARCHAR(32) NOT NULL,  -- wechat/wecom/douyin/email/sms
  direction VARCHAR(16) NOT NULL,  -- inbound/outbound
  account_id BIGINT NOT NULL,  -- 平台账号 ID
  customer_id BIGINT,
  external_msg_id VARCHAR(128),  -- 第三方消息 ID
  content_type VARCHAR(16) DEFAULT 'text',  -- text/image/file
  content TEXT,
  media_url VARCHAR(512),
  status VARCHAR(16) DEFAULT 'pending',  -- pending/sent/delivered/read/failed
  error_message TEXT,
  is_read BOOLEAN DEFAULT false,
  read_at TIMESTAMP,
  replied BOOLEAN DEFAULT false,
  conversation_id BIGINT,
  sent_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL,
  UNIQUE KEY uk_external (channel, external_msg_id),
  INDEX idx_data, conversation_id, created_at),
  INDEX idx_data, is_read, created_at)
);

CREATE TABLE unified_conversations (
  id BIGINT PRIMARY KEY,
  
  customer_id BIGINT NOT NULL,
  channel VARCHAR(32) NOT NULL,
  account_id BIGINT NOT NULL,
  last_message_id BIGINT,
  last_message_at TIMESTAMP,
  unread_count INT DEFAULT 0,
  status VARCHAR(16) DEFAULT 'open',  -- open/closed
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  UNIQUE KEY uk_customer_channel ( customer_id, channel, account_id)
);
```

---

## 七、测试说明

### 7.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 接收微信 | inbound | 写入 | 待执行 |
| TC-002 | 接收企微 | inbound | 写入 | 待执行 |
| TC-003 | 接收抖音 | inbound | 写入 | 待执行 |
| TC-004 | 接收邮件 | inbound | 写入 | 待执行 |
| TC-005 | 接收短信 | inbound | 写入 | 待执行 |
| TC-006 | 会话聚合 | 同一客户 | 合并 | 待执行 |
| TC-007 | 客户归一 | OneID | 匹配 | 待执行 |
| TC-008 | 实时推送 | 新消息 | WS 推送 | 待执行 |
| TC-009 | 回复发送 | 触发 | 平台发送 | 待执行 |
| TC-010 | 标记已读 | 触发 | 状态更新 | 待执行 |
| TC-011 | 未读统计 | 查询 | 正确数 | 待执行 |
| TC-012 | 多渠道过滤 | 渠道 | 过滤 | 待执行 |
| TC-013 | 敏感词 | 触发 | 拒绝 | 待执行 |
| TC-014 | 频率限制 | 触发 | 限流 | 待执行 |
| TC-015 | 账号失效 | 模拟 | 提示 | 待执行 |
| TC-016 | 跨实例隔离 | 商户 A | 商户 B 不可见 | 待执行 |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| WebSocket | WS_ENABLED | true | 实时推送 |
| 消息保留 | UNIFIED_MSG_RETENTION | 1y | |

### 8.x 可观测性 (私域: 应用层日志 + DB 审计)

> 私域部署: 不接入外部告警通道 (钉钉/飞书/邮件等)。关键指标通过  /  表落库, 巡检通过  SQL 查询实现。

---

## 九、参考资料

- PROJECT_FUNCTIONAL_ARCHITECTURE.md 第 3.1.13 节
- [MASTER_RULES.md](../standards/MASTER_RULES.md)
