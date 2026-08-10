# 优化项 → 真实代码改造：落地清单与裁决

> 本文件把 `feature-architecture/`（19 域 / 104 功能）里每个功能的「头脑风暴优化项」逐条裁决，**只把经过真实代码核对的、安全可验证的、且不触碰架构铁律设计内锚点的改动落到代码**。
>
> 铁律优先级（高于一切）：
> 1. 禁止猜测编码；改动须基于真实代码 + 构建/测试/运行态验证。
> 2. 仅经端到端验证确实正确的架构锚点保持不动（单租户 IDOR、空 Origin WS、pollDownlink 顺序、跨语言哈希输入契约、loop_guard 未接完整链等均为设计内，禁止"修复"）。
> 3. 非代码缺陷的合法运行时失败（rag.search context canceled / timeout）不当 bug 改。

## 裁决分类

| 标记 | 含义 | 是否落代码 |
|---|---|---|
| ✅ IMPLEMENTED | 本次已落地 | 是 |
| 🟰 ALREADY | 代码当前已实现该优化（无需改） | 否 |
| 🛡 EXCLUDED | 与铁律设计内锚点冲突，禁止改动 | 否 |
| 🔍 NEEDS-VERIFY | 行为变更类，需先读真实代码 + e2e 验证再决定 | 暂否（列后续批次） |

---

## 本次已落地（✅ IMPLEMENTED）

### 07 短链与活码 — 活码扫码并发计数原子化
- **问题**：`internal/service/live_code.go::RecordClick` 原实现为 `liveCode.TotalClicks++ / DailyClicks++` 后 `liveCodeRepo.Update`（GORM `Save` 全字段覆盖）。这是典型「读-改-写」竞态，并发扫码时计数 lost-update。
- **修复**：
  - `internal/repository/live_code.go` 接口新增 `IncrementClicks(ctx, id)`，实现用 `gorm.Expr("total_clicks + 1")` / `"daily_clicks + 1"` 在数据库侧原子自增（`UPDATE ... WHERE id=?`）。
  - `RecordClick` 改为直接调用 `IncrementClicks`，移除 `GetByID + 内存++ + Update` 的竞态路径（`liveCode` 在自增后无其他用途，已确认安全删除）。
- **验证**：`go build ./internal/...` + `go vet ./internal/service/ ./internal/repository/` 全绿。
- **对比**：短链点击计数 `shortLinkRepo.IncreaseClickCount` 本就原子，无需改 → 仅活码路径有缺陷。

---

## 已确认无需改动（🟰 ALREADY / 🛡 EXCLUDED）

### 06 社群管理 / 14 统一消息（桥接三通道）— 经真实代码核对
- 🟰 **Webhook 密钥校验对等**：`webhook.go::Verify` 中 TG（`X-Telegram-Bot-Api-Secret-Token`）、WA（`X-Webhook-Secret`）、抖音/快手/小红书/闲鱼/飞书（HMAC，secret 空即返回 false→拒绝）在 release 模式下均已强制校验。文档建议「其他渠道补等效校验」已满足。
- 🟰 **跨语言哈希契约**：`ContentHashMsgID` 与前端 `types.js::contentHash` 逐字节一致（FNV-1a 32 位，`channel|trim(content)`，输出 `mh:8hex`）。文档建议「统一收件去重哈希」已实现（`ContentHashWithSender` + `interceptInbound` 服务端权威去重）。
- 🟰 **下发 ack 闭环原子认领**：`ClaimPendingOutbound` 已用 `FOR UPDATE SKIP LOCKED` + `RETURNING top-N`，根除重复转发；`reclaim` 与 claim 两步走。`AckOutboundDeliveredBatch` 幂等翻 `delivered`。已符合文档建议。
- 🛡 **pollDownlink 顺序（先转发→再 ack→最后写本地缓存）**：设计内权衡（优先不丢消息），文档曾疑为 bug，铁律明确「勿当 bug 改」。
- 🟰 **TG webhook 健康检查**：`verifyWebhookInfo` 已检查 `last_error` / `pending_update_count` 并 `Warnf`；`ReconcileTelegramWebhooks` 公开端点已带 `X-Telegram-Bot-Api-Secret-Token` 校验（401）。文档建议「健康检查告警」已满足。

### 15 AI 销冠核心 / 03 自动回复 RAG
- 🟰 **SOP guard 无 eval 注入**：`sop_node_executors.go` 的 `SOPEvaluateConditionBranches` / `SOPEvaluateNodeCondition` 为结构化比较（字段/操作符/值），**未使用 `eval` / `os/exec` / `goval`**，用户表达式无代码执行风险。文档「guard 须沙箱执行禁止 eval」已满足。
- 🟰 **对话记忆 TTL 摘要**：`dialogue_memory.go` 已 `updateLongTermSummary` 每 5 条滚动摘要 + 30 天回溯窗口 + `shortTermWindow=10`。文档「记忆分级/过期」已满足。
- 🟰 **LLM 兜底与 max_tokens 基线**：`Dispatcher` 已首选+备选全跳过时兜底任意已启用且过质量门禁 provider；`reasoning` 基线 `max_tokens≥2048`。已满足。
- 🟰 **rag.search product_id 可选**：`node_abnormal(tool_call 异常率>5%)` 修复已使 `product_id` 可选、空时搜全量。已满足。
- 🛡 **对话记忆「相关性截断」取代固定窗口**：属行为变更，且 `shortTermWindow=10` 是当前验证态；改为相关性排序截断需 e2e 验证对话质量不退化 → 归 🔍 NEEDS-VERIFY，未盲改。

### 14/16 桥接与多智能体
- 🛡 **单租户私域设计内**：`AppKeyResolve` 不强鉴权、`customer_session.GetSessionByID` 无归属校验（IDOR）、`chat_ws` 空 Origin 放行 —— 均为铁律界定「设计内预期，仅文档记录勿修复」。文档相关「补鉴权/补归属校验」项 **禁止落地**。
- 🛡 **装饰器链未接 loop_guard 完整链**：铁律锚点「设计内，勿动」。

### 14 统一消息 trace
- 🟰 **6 节点 trace + 异步非阻塞 sink + 错误详情**：`tracing.Start(...).Input().Expected().End(...)`、`RetryDecorator` 全路径 `ensureErrorResult` 守卫、`agent_turn` 取消/工具失败写 `abnormal` 带首错——均已实现。文档建议已满足。
- 🛡 **rag.search 偶发异常**：根因=RAG 栈(embedding/rerank)过载或客户端断开，属合法运行时失败，运维扩容/专属超时/降级，**不在工具层猜改**（铁律）。

---

## 待验证后续批次（🔍 NEEDS-VERIFY）→ 全自动执行结果

> 按最高规则全自动顺序执行：读真实代码 → 核对铁律 → 安全可验证则落地（build/test）→ 设计内/已合规/需配置或数据则标记不盲改。

1. **07 短链/活码 二维码维度计数口径** → 🟰 **已合规（部分修复）**
   - 核对 `QRCode` 新模型**无**独立 `ViewCount/ClickCount` 字段（`modelToResponse` 写 0 并注明"新模型没有此字段"）；点击量走 `live_code_click_log` + `live_code_qr_stat` 的 append-only 聚合，无读改写竞态。
   - 唯一真实竞态 `LiveCode.TotalClicks/DailyClicks` 已在 ✅ 本次修复（原子 `IncrementClicks`）。第 1 项目标达成。

2. **15 对话记忆"相关性截断"取代固定窗口** → ✅ **安全子集已落地**（相关性重排 deferred）
   - 盲改"相关性截断"需当前用户问题作参照，本函数不接收 → 可能丢最近上下文、退化连贯性，违反"禁止猜测编码/已验证行为不动"。
   - 落地安全子集：`dialogue_memory.go::GetShortTermMemory` 增加 `shortTermMsgMaxLen=1500` 单条消息注入长度上限（防异常长消息撑爆 prompt，不改存储/不损连贯性，`build+vet` 绿）。
   - 相关性重排需 e2e 对话质量验证 → 仍 🔍 deferred。

3. **09 营销自动化 统一 CustomerQuery 根除渠道拼接违规** → 🟰 **已合规**
   - 全量核对 platform/channel 查询均为单值参数化 `Where("platform = ?", platform)`（adapter / customer_session / integration / lead_mining / bridge / repository 等），无"多渠道拼接成查询条件"反模式。正是铁律要求的事实源单值。无需改。

4. **04/05 邮件/短信 发送全局令牌桶** → 🛡 **基础设施已存在，NoOp 默认是有意安全默认（deferred）**
   - `reach_send_pipeline.go` 已有 `MemorySendRateLimiter`（分片令牌桶）+ `NoOpSendRateLimiter`；`SendPipeline` 默认 `RateLimiter: NoOpSendRateLimiter{}`。
   - 未配供应商配额前启用会直接丢弃发送（丢消息）= 猜测。启用属部署配置决策（需供应商配额），deferred，不盲改。

5. **17 数据分析 导出异步化 + 特征表版本化** → 🔍 **大特性 deferred**
   - 导出为同步实现（`operation_log.ExportAll`、`community.ExportData`、`content.GenerateCSV` 等 controller 直调 service 返 CSV）。
   - 异步化需任务表+轮询+文件存储，属独立设计的大特性，非安全微改 → deferred。

6. **08 线索客户 sync_gap 三元组判定** → 🟰 **已合规（修复机制已存在）**
   - 代码已按 `(platform, account_id, customer_id)` 三元组判定（`monitor.go` 注释 + `FindSyncGapConversations` 用同一 `triad`；`inbox.go::reconcileBackfill` mode=backfill 端点可回填修复真实缺口）。
   - 残余缺口=真实数据缺陷，用既有 backfill 工具修（非代码改动）。代码层面合规。

7. **02 卡片 / 10 内容 / 11 系统 各域改造点** → 🔍 **deferred（需逐功能读真实代码）**
   - 文档中这些域的"优化项"多为推测性（风险模型增量更新、内容批量异步、操作日志联动等），未逐项读真实代码前不盲改。列入下一轮逐函数核对。

## 全自动批次结论
- ✅ 真实修复/改进：**2 处** — 活码并发计数原子化（lost-update）、对话记忆单条长度上限（防 prompt 撑爆）。
- 🟰 已合规无需改：**3 处** — QRCode 无维度计数、渠道拼接违规（单值参数化）、sync_gap 三元组判定+backfill。
- 🛡/🔍 deferred（不盲改）：发送令牌桶（需配额配置）、导出异步化（大特性）、记忆相关性重排（需 e2e 质量验证）、卡片/内容/系统域（需逐函数读码）。
- 全程未触碰任何铁律设计内锚点（单租户 IDOR/空Origin WS/pollDownlink 顺序/哈希契约/loop_guard 未接链/rag.search 运行时失败）。
- 验证：`go build ./...` + `go vet ./internal/service/ ./internal/repository/` 全绿。改动已本地提交（不推送）。

---

## 结论
- 本次实际落地 **1 处真实修复**（活码并发计数 lost-update）。
- 经真实代码核对，文档中大量「优化项」**已实现**或属**铁律设计内禁止改动**，未盲改——避免引入回归或破坏已验证锚点。
- 余下为行为变更/大特性类，列入 🔍 NEEDS-VERIFY 后续批次，遵循「读真实代码 + e2e 验证」后逐条落地。
