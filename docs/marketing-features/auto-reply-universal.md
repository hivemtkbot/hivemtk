# 通用自动回复 (Universal Auto Reply)

> **所属系统**: 用户端/商户端（user-server）  
> **功能slug**: `auto-reply-universal`  
> **文档定位**: 营销工具既有功能独立文档，遵循 [FEATURE_DOCUMENTATION_TEMPLATE.md](../standards/FEATURE_DOCUMENTATION_TEMPLATE.md) 与 [MASTER_RULES.md](../standards/MASTER_RULES.md)。

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称（中文） | 通用自动回复（chromedp） |
| 功能名称（英文） | Universal Auto Reply |
| 当前状态 | 已完成 |
| 完成百分比 | 100% |
| 所属模块 | auto-reply |
| 优先级 | P0 |

### 1.1 已完成内容

- [x] 数据库表结构（auto_reply_accounts / rules / logs）
- [x] 后端 Service 与 Controller
- [x] 前端页面（Login/Status/Accounts/Rules/Logs/Stats 6 页）
- [x] chromedp 浏览器自动化引擎
- [x] 规则匹配引擎（关键词 + 置信度）
- [x] 启停控制 + 无头模式
- [x] 速率限制 + 模拟消息测试
- [x] 单元测试 / 集成测试
- [x] UI 自动化测试

---

## 二、核心原理

### 2.1 业务背景

多平台（抖音/快手/小红书/视频号等）消息自动回复，替代人工值守，提高响应速度和转化率。系统通过 chromedp 控制真实浏览器登录、操作。

### 2.3 关键算法或模型

- 关键词匹配：TF-IDF + 编辑距离
- 置信度评估：rule_score × 0.8 + rag_score × 0.6 + llm_score × 0.4
- 速率限制：滑动窗口算法

### 2.4 输入输出定义

| 类型 | 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|---|
| 输入 | account_id | int64 | 是 | 账号ID |
| 输入 | platform | string | 是 | 平台 |
| 输入 | message | string | 是 | 收到的消息 |
| 输入 | sender | string | 是 | 发送者ID |
| 输出 | reply | string | 是 | 回复内容 |
| 输出 | rule_id | int64 | 否 | 命中的规则ID |
| 输出 | confidence | float | 是 | 置信度 |

---

## 三、设计标准

### 3.2 API 契约

| Method | URL | 说明 |
|---|---|---|
| POST | /api/auto-reply/login | 登录平台账号 |
| GET | /api/auto-reply/status | 获取登录状态 |
| POST | /api/auto-reply/logout | 登出 |
| GET | /api/auto-reply/accounts | 账号列表 |
| POST | /api/auto-reply/accounts | 添加账号 |
| DELETE | /api/auto-reply/accounts/:id | 删除账号 |
| GET | /api/auto-reply/rules | 规则列表 |
| POST | /api/auto-reply/rules | 创建规则 |
| PUT | /api/auto-reply/rules/:id | 更新规则 |
| DELETE | /api/auto-reply/rules/:id | 删除规则 |
| GET | /api/auto-reply/logs | 日志 |
| POST | /api/auto-reply/simulate | 模拟消息测试 |
| POST | /api/auto-reply/batch-test | 批量测试 |
| PUT | /api/auto-reply/headless | 切换无头模式 |

### 3.3 安全与合规

- Cookie 加密存储（AES-256）
- 速率限制（防封号）
- 全部消息脱敏存储
- 操作审计

### 3.4 性能指标

| 指标 | 目标值 | 当前值 |
|---|---|---|
| 消息响应延迟 | < 5s | ~3s |
| 并发账号数 | 50+ | 80 |
| chromedp 实例数 | 5+ | 8 |

---

## 四、架构与模块关系

### 4.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| Controller | `internal/controller/auto_reply.go` | 接口 |
| Service | `internal/service/auto_reply_service.go` | 业务编排 |
| Repository | `internal/repository/auto_reply_repo.go` | 数据 |
| Model | `internal/model/auto_reply*.go` | 模型 |
| Infra | `internal/browser/chromedp.go` + Redis | 浏览器自动化 + 缓存 |

### 4.2 依赖模块

| 模块 | 接口/依赖说明 |
|---|---|
| auth | 鉴权 |
| rag-knowledge-base | RAG 知识库（兜底回复） |
| redis | 状态/日志缓存 |

### 4.3 被依赖模块

| 模块 | 被依赖说明 |
|---|---|
| 营销自动化 | 流程触发自动回复 |

### 4.4 数据流向

```text
[平台消息 Webhook]
   → auto_reply_service.Match → 关键词匹配
   → 置信度评估 → 命中规则
   → 拼装回复文本
   → browser_chromedp.Send
   → 写 auto_reply_logs 表
   → Redis 累加统计
```

---

## 五、流程说明

### 5.1 用户操作流程

1. 添加平台账号（扫码登录）
2. 创建自动回复规则（关键词 + 回复内容）
3. 启动服务
4. 查看消息日志
5. 根据命中率调整规则

### 5.2 系统处理流程

1. 接收平台消息（WebSocket / 轮询）
2. 关键词匹配 → 候选规则
3. 置信度评估
4. 选择策略（规则 / RAG / LLM）
5. 拼装回复
6. 速率限制等待
7. chromedp 发送
8. 写日志 + 统计

### 5.3 异常处理流程

| 异常场景 | 错误码 | 处理方式 |
|---|---|---|
| 账号掉线 | 401001 | 提示重新登录 |
| 消息超限 | 429001 | 限流等待 |
| chromedp 崩溃 | 500001 | 自动重启实例 |

---

## 六、数据库设计

### 6.1 核心表 auto_reply_accounts

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| platform | varchar(32) | 非空 | 平台 |
| account_name | varchar(64) | 非空 | 账号名 |
| cookie_enc | text | | 加密 Cookie |
| status | tinyint | 非空 | 0=离线 1=在线 2=禁用 |
| last_active_at | timestamp | | 最后活跃 |

### 6.2 核心表 auto_reply_rules

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| account_id | bigint | FK | 账号 |
| name | varchar(64) | 非空 | 规则名 |
| keywords | jsonb | 非空 | 关键词列表 |
| match_type | varchar(16) | | exact/fuzzy/regex |
| reply_type | varchar(16) | | text/image/card |
| reply_content | text | 非空 | 回复内容 |
| priority | int | 默认 0 | 优先级 |
| status | tinyint | 非空 | 0=禁用 1=启用 |

### 6.3 核心表 auto_reply_logs

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| id | bigint | PK | 主键 |
| rule_id | bigint | FK | 命中的规则 |
| account_id | bigint | FK | 账号 |
| sender | varchar(128) | | 发送者 |
| message | text | | 收到的消息 |
| reply | text | | 回复内容 |
| confidence | float | | 置信度 |
| status | varchar(16) | | success/failed |
| error_msg | text | | 错误信息 |
| created_at | timestamp | 非空 | 时间 |

---

## 七、测试说明

### 7.2 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 关键词命中 | "价格" | 命中价格规则 | ✅ |
| TC-002 | 模糊匹配 | "价钱多少" | 命中价格规则 | ✅ |
| TC-003 | 无规则命中 | "未知问题" | 走 RAG/LLM | ✅ |
| TC-004 | 速率限制 | 连续 10 条 | 后 5 条延迟 | ✅ |
| TC-005 | 无头模式 | 切换 headless | 资源占用降低 | ✅ |
| TC-006 | 批量测试 | 100 条测试 | 命中率报告 | ✅ |

---

## 八、部署与运维

### 8.1 配置项

| 配置项 | 环境变量 | 默认值 |
|---|---|---|
| CHROMEDP_HEADLESS | CHROMEDP_HEADLESS | true |
| REPLY_RATE_LIMIT | REPLY_RATE_LIMIT | 5 |
| REPLY_INTERVAL_MIN | REPLY_INTERVAL_MIN | 5 |
| REPLY_INTERVAL_MAX | REPLY_INTERVAL_MAX | 15 |
| CHROMEDP_INSTANCES | CHROMEDP_INSTANCES | 5 |

### 8.2 依赖服务

- chromedp（本地 Chrome 浏览器）
- Redis 7.x

### 8.3 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 账号掉线率 | > 30% | 飞书 |
| 回复失败率 | > 10% | 飞书 |
| chromedp 实例存活 | < 3 | 飞书 |

---

## 九、参考资料

- BROWSER_ASSISTANT.md
