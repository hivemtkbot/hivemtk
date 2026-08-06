# AI 回复后用户新发消息 — 极限场景处理设计

> 场景：上一条是 AI 返回消息（outbound, is_ai_reply=true），用户新发消息（inbound）
> 关注点：判断时机在哪里？此时查库是否需要返回？
> 修复时间：2026-08-05

---

## 1. 极限场景描述

```
时序A（正常）：
  用户消息1(inbound) → 入库 → 查最后一条=inbound → 触发AI → AI回复(outbound)
  → 用户看到回复后发消息2(inbound) → 入库 → 查最后一条=inbound(消息2) → 触发AI ✓

时序B（极限场景 — 消息遗漏 Bug）：
  用户消息1(inbound) → 入库 → 触发AI（设置 ai_processing 标记，5min TTL）
  → AI 异步推理中（几秒）
  → 用户消息2(inbound) → 入库 → 查最后一条=inbound(消息2)
  → 但 ai_processing 标记存在 → 跳过触发（避免重复回复消息1）
  → AI 回复消息1完成 → 落库 outbound → 释放标记
  → ❌ 消息2(inbound) 成为"孤儿"，无人回复！
```

**根因**：`sendOutbound` 释放 `ai_processing` 标记后，没有重新检查是否有未回复的客户消息。

---

## 2. 判断时机分析

### 2.1 当前判断时机

判断位于 `HandleIngressBatch` step 3（`inbox_ingress.go`）：

```
消息入库 → 查 DB 最后一条方向 → 决定是否触发 AI
```

- `HasUnrepliedCustomerMessage(convID, 5min)` 查 `message_hub` 表最后一条消息的 `direction` 字段
- `outbound` → 已回复，不触发
- `inbound` + 5min 内 → 未回复，触发 AI
- `inbound` + 超 5min → 历史消息，不触发

### 2.2 三道前置卡点（不触发 AI 的情况）

| 卡点 | 位置 | 说明 |
|------|------|------|
| 最后一条是 outbound | `HasUnrepliedCustomerMessage` | AI/人工已回复 |
| 超 5min 窗口 | `HasUnrepliedCustomerMessage` | 历史补录消息不逐一自动回复 |
| ai_processing 标记存在 | `HandleIngressBatch` line 646 | AI 推理中，防 bridge 重复扫描导致重复触发 |

### 2.3 修复新增的第四道补触发

| 时机 | 位置 | 说明 |
|------|------|------|
| AI 回复释放标记后 | `sendOutbound` → `RecheckUnrepliedAndTrigger` | 延迟 800ms 重检查，补触发遗漏消息 |

---

## 3. 查库策略论证

### 3.1 查库清单

| 查询项 | 是否查库 | 业务理由 |
|--------|----------|----------|
| `message_hub` 最后一条方向 | **必须查** | 判断"是否已回复"的唯一依据 |
| RAG 知识库 | **必须查** | 新轮次意图可能转移，知识库可能更新 |
| `dialogue_memories` 记忆库 | **必须查** | 上下文连续性是多轮对话的基础 |
| 人工接管锁 | **必须查** | 用户可能在 AI 回复后转人工 |
| ai_processing 标记 | **必须查** | 防止与新一轮 AI 触发竞态 |

### 3.2 为什么每次触发 AI 都重新查 RAG（不复用上轮结果）

1. **意图可能转移**：用户新发消息可能问不同问题（如从"价格咨询"转到"退款流程"）
2. **知识库可能更新**：管理员可能在两轮对话间更新了知识库内容
3. **RAG 召回基于当前消息语义**：与上一轮消息不同，召回结果不同
4. **结论**：当前实现（每次触发都查 RAG）是业务正确的，不引入缓存复用

### 3.3 5min 窗口的语义

`sent_at` 是消息原始发生时间（bridge 上报时携带），**不是**上报时间：

| 消息类型 | sent_at | 窗口判断 | 行为 |
|----------|---------|----------|------|
| 实时消息 | ≈ now | 在窗口内 | 触发 AI ✓ |
| 历史补录（5min 前） | 5min 前 | 超窗口 | 不触发 ✓ |

设计意图：**区分实时消息 vs 历史补录**，避免对存量消息逐一自动回复。

---

## 4. 修复方案

### 4.1 核心修复：RecheckUnrepliedAndTrigger

**文件**：`internal/service/inbox_ingress.go`

```
sendOutbound（AI 回复落库后）
  ├─ ReleaseAIProcessingFlag（释放标记）
  └─ go RecheckUnrepliedAndTrigger（异步补触发）
       ├─ 延迟 800ms（让期间消息入库）
       ├─ 检查人工接管锁 → 锁定则跳过
       ├─ 查 HasUnrepliedCustomerMessage
       │   ├─ 最后一条 outbound → 无遗漏，跳过
       │   └─ 最后一条 inbound + 5min 内 → 有遗漏
       ├─ 再次检查 ai_processing 标记 → 存在则跳过（新一轮已触发）
       ├─ GetLastInboundByConversation（获取遗漏消息内容）
       └─ triggerAIForEvent（补触发 AI）
```

### 4.2 安全保障

1. **延迟 800ms**：让 AI 推理期间到达的消息完成入库再查 DB
2. **人工锁检查**：AI 回复后用户可能转人工，不补触发
3. **ai_processing 标记重检**：防止与新一轮触发竞态（释放后可能已被其他路径重新设置）
4. **aiTrigger nil 守卫**：未注入 AI 触发器时记日志跳过，不 panic
5. **context.WithoutCancel**：异步 goroutine 不受 sendOutbound 15s timeout 限制

### 4.3 装配注入

**文件**：`internal/router/router.go`

```
bridgeIngressSvc.SetAITrigger(webhookSvc)       // ingress → webhook AI 触发
webhookSvc.SetIngressSvc(bridgeIngressSvc)      // webhook → ingress 释放标记 + 补触发
```

确保 `sendOutbound` 使用的 ingressSvc 拥有完整能力（aiTrigger + hubRepo + cache）。

---

## 5. 测试覆盖

**文件**：`internal/service/inbox_ingress_recheck_test.go`

| 测试用例 | 验证点 |
|----------|--------|
| `TestRecheck_NoHubRepo_SafelySkips` | hubRepo==nil 安全跳过不 panic |
| `TestRecheck_EmptyConvID_SafelySkips` | conversationID 为空安全跳过 |
| `TestRecheck_LastIsOutbound_NoTrigger` | 最后一条是 outbound → 不补触发 |
| `TestRecheck_LastInboundWithinWindow_TriggersAI` | 最后一条 inbound + 窗口内 → 补触发 |
| `TestRecheck_LastInboundOutsideWindow_NoTrigger` | 最后一条 inbound 超 5min → 不补触发 |
| `TestRecheck_AIProcessingFlagExists_NoTrigger` | ai_processing 标记存在 → 不补触发 |
| `TestRecheck_HumanLocked_NoTrigger` | 人工接管锁 → 不补触发 |
| `TestRecheck_FullScenario_OrphanMessage` | 完整极限场景端到端验证 |
| `TestRecheck_ContextCanceled_NoBlock` | context 取消不阻塞 |

运行命令：
```bash
POSTGRES_TEST_PORT=8232 go test -timeout 120s -run 'TestRecheck' ./internal/service/ -v
```

---

## 6. 已知局限

### 6.1 场景：AI 推理期间用户发消息，AI 回复落库在用户消息之后

```
时序：in1 → in2(用户在AI推理期间发) → out1(AI回复in1)
DB 最后一条：out1 → unreplied=false → 补触发不触发
```

此时 `HasUnrepliedCustomerMessage` 查最后一条是 outbound（AI 回复），判定"已回复"。
但 in2 实际未被回复（AI 回复的是 in1，不是 in2）。

**影响**：in2 被遗漏。但此场景概率极低（用户需在 AI 推理的几秒内发消息，且 AI 回复落库在 in2 之后）。

**未来修复方向**：引入"未回复 inbound 消息队列"（pending queue），AI 回复后检查队列中是否有比 AI 回复时间更早的未回复 inbound。

### 6.2 sessionID 传递

`sendOutbound` 中 `hubMsg`（MessageHub）无 SessionID 字段，补触发时 sessionID 传空字符串。
`RecheckUnrepliedAndTrigger` 在 sessionID 为空时跳过人工锁检查。

**影响**：如果用户在 AI 回复后到补触发之间的 800ms 内转人工，补触发不会检查人工锁。
但此场景概率极低，且 SmartCSOrchestrator 内部有 HandlerType 检查兜底。
