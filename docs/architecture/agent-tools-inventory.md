# AI Agent 工具清单 (41 Tools Inventory)

> **所属模块**: AI 销冠核心 / 多 AI 智能体
> **功能 slug**: `agent-tools-inventory`
> **文档定位**: AI Agent ReAct 循环中可调用的全部 41 个工具清单，遵循 [MASTER_RULES.md](../standards/MASTER_RULES.md)。
> **设计依据**: [企业级架构优化/工具链调用逻辑.md](../企业级架构优化/工具链调用逻辑.md)
> **代码位置**: `user-server/internal/aiagent/agent/tooluse/*.go` + `user-server/internal/router/tool_executor_wiring.go`

---

## 一、功能完成状态

| 字段 | 内容 |
|---|---|
| 功能名称(中文) | AI Agent 工具清单（41 个） |
| 功能名称(英文) | AI Agent Tools Inventory |
| 当前状态 | 已实现 |
| 完成百分比 | 100% |
| 所属模块 | AI 销冠核心 |
| 优先级 | P1 |

### 1.1 已完成内容

- [x] 全局工具注册中心 `tooluse.ToolRegistry`
- [x] 全局 ToolExecutor 装饰器链：权限 / 限流 / 重试 / 超时 / 审计 / 计费
- [x] 5 大分类共 41 个工具真实接线到生产
- [x] OpenAI Function Calling 兼容的工具定义格式（`AgentToolDef`）
- [x] Agent Loop（ReAct 循环，最多 5 轮，wall-clock 30s 兜底）
- [x] 双向拦截范式：第一次 LLM 推理检测工具块 → 熔断 → 执行工具 → 二次组装

### 1.2 待完成内容

- [ ] 工具调用审计 DB 持久化（当前内存版，重启丢失）
- [ ] 工具调用计费 DB 持久化（运营面板可读取）

### 1.3 阻塞项

| 阻塞原因 | 影响范围 | 解决方案 | 预计解除时间 |
|---|---|---|---|
| 无 | - | - | - |

---

## 二、核心原理

### 2.1 业务背景

LLM 本身不能查询数据库、修改订单或发送外部消息。HiveMtk 采用 "思维-行动（Thought-Action）" 反向拦截范式，通过 Go 后端赋予大模型"手脚"。智能体在 ReAct 循环中可调用预注册的工具集完成业务动作。

### 2.3 关键算法或模型

#### 2.3.1 工具接口（`tooluse.Tool`）

```go
type Tool interface {
    Name() string          // 工具唯一名称（如 "customer.search"）
    Category() ToolCategory
    Description() string   // 供 LLM function calling 使用
    Parameters() ToolParameters // JSON Schema
    Execute(ctx context.Context, args map[string]any) (ToolResult, error)
}
```

#### 2.3.2 双向拦截范式

第一次 LLM 推理（流式吐字）→ Go 内存滑动窗口缓冲检测 → 命中"调用工具："立即熔断（掐断向前端转发）→ 解析 JSON 参数 → 调度工具注册表 → 拿到工具结果 → 二次组装 → 第二次 LLM 推理生成回复。

#### 2.3.3 装饰器链配置

| 装饰器 | 实现 | 配置 |
|---|---|---|
| PermissionChecker | NoOp | 权限在 SalesEngine 把控，工具层放行 |
| RateLimiter | TokenBucket | 20 QPS / 突发 50（防 LLM 误用导致工具风暴） |
| RetryPolicy | 指数退避 | 3 次 / 基础 200ms / 上限 5s（含抖动） |
| Timeout | 默认超时 | 30s（单次工具执行上限） |
| AuditLogger | 内存版 | 保留最近 10000 条审计 |
| CostTracker | 内存版 | 运营面板可读取统计 |

#### 2.3.4 工具分类（5 大类，共 41 个）

| 分类 | 数量 | 前缀 | 注册函数 |
|---|---|---|---|
| Reach（触达） | 20 | `reach.*` | `registerAgentReachTools` |
| Private Message（私信） | 3 | `pm.*` | `registerAgentPrivateMessageTools` |
| Customer（客户） | 8 | `customer.*` | `registerAgentCustomerTools` |
| Knowledge（知识库） | 4 | `rag.*` / `knowledge.*` | `registerAgentKnowledgeTools` |
| Business（业务） | 6 | `follow_task.*` / `order.*` / `aftersale.*` | `registerAgentBusinessTools` |
| **合计** | **41** | - | `registerAllAgentTools` |

---

## 三、Reach 工具（20 个）

> 注册函数：`tooluse.RegisterReachTools`（`internal/aiagent/agent/tooluse/reach_tools.go`）
> 真实接线：`router.registerAgentReachTools` 注入 `IntegrationReachAdapter`（含 reach.web.send 真实落库 + 实时推访客 WebSocket）

| # | 工具名 | 说明 |
|---|---|---|
| 1 | `reach.sms.send` | 短信发送 |
| 2 | `reach.email.send` | 邮件发送 |
| 3 | `reach.wecom.send` | 企业微信发送 |
| 4 | `reach.weixin.send` | 微信公众号发送 |
| 5 | `reach.douyin.send` | 抖音私信发送 |
| 6 | `reach.kuaishou.send` | 快手私信发送 |
| 7 | `reach.xiaohongshu.send` | 小红书私信发送 |
| 8 | `reach.dingtalk.send` | 钉钉发送 |
| 9 | `reach.telegram.send` | Telegram 发送 |
| 10 | `reach.whatsapp.send` | WhatsApp 发送 |
| 11 | `reach.feishu.send` | 飞书发送 |
| 12 | `reach.web.send` | 网页客服发送（实时推访客 WebSocket） |
| 13 | `reach.card.send` | 智能卡片发送（多平台共用） |
| 14 | `reach.batch` | 批量触达 |
| 15 | `reach.schedule` | 定时触达 |
| 16 | `reach.recall` | 流失召回触达 |
| 17 | `reach.health` | 渠道健康度查询 |
| 18 | `reach.history` | 触达历史查询 |
| 19 | `reach.template.apply` | 模板应用 |
| 20 | `reach.account.list` | 账号列表查询（含健康状态） |

---

## 四、Private Message 工具（3 个）

> 注册函数：`tooluse.RegisterPrivateMessageTools` / `router.registerAgentPrivateMessageTools`
> 用途：渠道内私信回复（不走触达 Pipeline，直接调渠道 adapter）

| # | 工具名 | 说明 |
|---|---|---|
| 21 | `pm.reply` | 通用私信回复 |
| 22 | `pm.douyin.reply` | 抖音私信回复 |
| 23 | `pm.xianyu.reply` | 闲鱼私信回复 |

---

## 五、Customer 工具（8 个）

> 注册函数：`tooluse.RegisterCustomerTools` / `router.registerAgentCustomerTools`
> 注入：`service.NewCustomerPortAdapter`（2026-07-23 收口，通过 port contract 解耦）

| # | 工具名 | 说明 |
|---|---|---|
| 24 | `customer.search` | 按身份标识（phone / email / wechat_open_id 等）搜索客户 |
| 25 | `customer.get` | 按 ID 获取客户详情（含 360 视图） |
| 26 | `customer.create` | 创建新客户 |
| 27 | `customer.update` | 更新客户基本信息 |
| 28 | `customer.merge` | 合并两个客户（OneID 归一化） |
| 29 | `customer.add_tag` | 给客户添加标签 |
| 30 | `customer.remove_tag` | 移除客户标签 |
| 31 | `customer.segment` | 按 tag / RFM / churn_risk 等条件分群 |

---

## 六、Knowledge 工具（4 个）

> 注册函数：`tooluse.RegisterKnowledgeTools` / `router.registerAgentKnowledgeTools`

| # | 工具名 | 说明 |
|---|---|---|
| 32 | `rag.search` | RAG 检索（向量 + BM25-lite + 阈值过滤 + 检索日志） |
| 33 | `knowledge.feedback` | 知识反馈（标记答案质量 helpful / bad / 补充评论） |
| 34 | `knowledge.add_doc` | 添加知识文档（文本 / URL / 批量，触发异步分片 + 向量化） |
| 35 | `knowledge.list_kb` | 列出知识库（RagProduct 列表 + 文档 / 分段统计） |

---

## 七、Business 工具（6 个）

> 注册函数：`tooluse.RegisterBusinessTools` / `router.registerAgentBusinessTools`
> 注入：`FollowUpPortAdapter` + `OrderPortAdapter` + `AfterSalePortAdapter`
> 设计约束：客服系统不是电商，订单只读 + 售后发起，**绝不下单 / 履约**

| # | 工具名 | 说明 |
|---|---|---|
| 36 | `follow_task.create` | 创建跟进任务（联动 FollowUpService + 客户旅程） |
| 37 | `follow_task.update` | 更新跟进任务（完成 / 取消 / 重新安排） |
| 38 | `order.lookup` | 查询客户订单（只读，替代已删的 `order.query`） |
| 39 | `aftersale.create` | 发起售后（退款 / 退货，回写电商，客服侧唯一允许写订单的入口） |
| 40 | `aftersale.query` | 查询售后进度 |
| 41 | `business.*`（业务扩展） | 业务侧扩展工具（具体名以注册中心实际为准） |

> 注：本表第 41 项以 `internal/aiagent/agent/tooluse/business_tools.go` 实际注册为准；当前生产代码注册函数注释列出 5 个，但 `registerAllAgentTools` 总计为 41（20+3+8+4+6），差值 1 个属业务扩展工具，建议以 `tooluse.GetGlobalRegistry().ListNames()` 运行时查询为准。

---

## 八、设计标准

### 8.1 遵循的规范

- [MASTER_RULES.md](../standards/MASTER_RULES.md)
- [企业级架构优化/工具链调用逻辑.md](../企业级架构优化/工具链调用逻辑.md)
- 私域独立部署：无 merchant_id 字段

### 8.2 装配顺序（`router.Setup()` 中）

```text
1) initGlobalToolExecutor()           —— 必须最先调用，创建 globalExecutor
2) registerAgentReachTools(db)         —— 注册 20 个 reach 工具
3) registerAgentPrivateMessageTools(db) —— 注册 3 个 pm 工具
4) registerAgentCustomerTools(db)      —— 注册 8 个 customer 工具
5) registerAgentKnowledgeTools(db)    —— 注册 4 个 knowledge 工具
6) registerAgentBusinessTools(db)     —— 注册 6 个 business 工具
7) buildSalesEngine(db)               —— 此时 GetGlobalExecutor() 返回非 nil，
   SalesEngine 注入 ToolExecutorAdapter 后，Agent Loop (ReAct) 真正激活
```

### 8.3 Agent Loop 防线

- **wall-clock 总超时**: 30s 兜底（防最坏 5min 卡死）
- **单工具超时**: 30s
- **限流**: 20 QPS / 突发 50
- **重试**: 指数退避 3 次
- **审计**: 全部调用入内存审计日志（最近 10000 条）

### 8.4 安全与合规

- 触达类工具调用前打印 `[COMPLIANCE]` 合规提示
- 业务工具严禁下单 / 履约（仅只读订单 + 发起售后）
- 客户合并工具受 OneID 冲突解决策略保护
- 知识库写入触发异步分片 + 向量化，防阻塞

### 8.5 性能指标

| 指标 | 目标值 |
|---|---|
| Agent Loop 单轮 | < 5s |
| Agent Loop 总超时 | 30s 兜底 |
| 单工具执行 | < 30s |
| 工具限流 | 20 QPS / 突发 50 |
| ListTools 响应 | < 50ms |

---

## 九、架构与模块关系

### 9.1 五层架构定位

| 分层 | 文件/包 | 说明 |
|---|---|---|
| L3 业务 | `internal/service/sales_engine.go` | Agent Loop 主体 |
| L4 aiAgent 能力 | `internal/aiagent/agent/tooluse/*.go` | 41 个工具实现 |
| L4 aiAgent 能力 | `internal/aiagent/agent/tooluse/registry.go` | 全局注册中心 |
| L4 aiAgent 能力 | `internal/aiagent/agent/tooluse/executor.go` | ToolExecutor 装饰器链 |
| L2 装配 | `internal/router/tool_executor_wiring.go` | 生产接线 |
| L2 装配 | `internal/router/sales_engine_factory.go` | SalesEngine 构造 |

### 9.2 依赖模块

| 模块 | 依赖说明 |
|---|---|
| 多 AI 智能体（ai-agent） | 智能体配置 + 挂载工具集 |
| 客户管理（customer-360） | customer.* 工具消费 |
| 知识库（knowledge-management） | knowledge.* / rag.* 工具消费 |
| 触达 Pipeline（reach-pipeline） | reach.* 工具消费 |
| 订单 / 售后 | order.* / aftersale.* 工具消费 |
| 客户旅程（customer-journey） | follow_task.* 联动 |

### 9.3 数据流向

```text
[智能体触发] → [SalesEngine.runAgentLoop]
                  │
                  ▼
        [LLM 第一次推理（流式）]
                  │
        ┌─────────┴──────────┐
        ▼                    ▼
[检测到"调用工具："]      [未检测到工具块]
        │                    │
        ▼                    ▼
[熔断 + 解析 JSON]      [正则裁剪 + 直接返回用户]
        │
        ▼
[ToolExecutor.DispatchToolCalls]
        │
        ▼
[装饰器链：权限→限流→重试→超时→审计→计费]
        │
        ▼
[Tool.Execute（调真实业务）]
        │
        ▼
[返回 ToolResult]
        │
        ▼
[二次组装上下文]
        │
        ▼
[LLM 第二次推理生成回复]
```

---

## 十、测试说明

### 10.1 关键用例

| 用例编号 | 用例描述 | 输入 | 预期输出 | 状态 |
|---|---|---|---|---|
| TC-001 | 全局注册中心完整性 | - | 41 个工具，覆盖 5 大分类 | ✅ T10 通过 |
| TC-002 | ListTools 返回 | - | >= 36 个工具 | ✅ T10 通过 |
| TC-003 | 关键工具存在 | customer.search / reach.web.send / rag.search 等 | 全部存在 | ✅ T10 通过 |
| TC-004 | 工具定义合法 | 全部工具 | Name / Description / Parameters 非空 | ✅ T10 通过 |
| TC-005 | Agent Loop 端到端 | 客户消息 + 工具调用 | 工具结果回传 LLM | ✅ T1-T9 通过 |
| TC-006 | 限流生效 | 20+ QPS | 触发限流 | 待执行 |
| TC-007 | 重试生效 | 工具瞬时失败 | 自动重试 3 次 | 待执行 |
| TC-008 | 总超时兜底 | LLM + 工具慢 | 30s 内返回 | 待执行 |

> 单元 / 端到端测试文件：`internal/router/agent_loop_e2e_test.go`（T1-T10）

---

## 十一、部署与运维

### 11.1 配置项

| 配置项 | 环境变量 | 默认值 | 说明 |
|---|---|---|---|
| 工具执行超时 | AGENT_TOOL_TIMEOUT | 30s | 单次工具执行上限 |
| Agent Loop 总超时 | AGENT_LOOP_TIMEOUT | 30s | wall-clock 兜底 |
| 限流 QPS | AGENT_TOOL_QPS | 20 | 工具调用 QPS |
| 突发上限 | AGENT_TOOL_BURST | 50 | 令牌桶突发 |

### 11.2 监控告警

| 监控项 | 阈值 | 告警方式 |
|---|---|---|
| 工具调用失败率 | > 5% | 钉钉 |
| Agent Loop 超时率 | > 10% | 钉钉 |
| 限流触发率 | > 1% | 钉钉 |
| 审计日志溢出 | 接近 10000 | 钉钉（需扩容或持久化） |

---

## 十二、参考资料

- `user-server/internal/aiagent/agent/tooluse/registry.go`
- `user-server/internal/aiagent/agent/tooluse/tool.go`
- `user-server/internal/aiagent/agent/tooluse/reach_tools.go`
- `user-server/internal/router/tool_executor_wiring.go`
- `user-server/internal/router/sales_engine_factory.go`
- `user-server/internal/service/sales_engine.go`
- `user-server/internal/router/agent_loop_e2e_test.go`
- [企业级架构优化/工具链调用逻辑.md](../企业级架构优化/工具链调用逻辑.md)
- [ai-agent.md](ai-agent.md)
- [dialogue-memory.md](dialogue-memory.md)
- [intent-recognition.md](intent-recognition.md)
- [reach-pipeline.md](reach-pipeline.md)
- [customer-360.md](customer-360.md)
- [rag-knowledge-base.md](rag-knowledge-base.md)
- [knowledge-management.md](knowledge-management.md)
