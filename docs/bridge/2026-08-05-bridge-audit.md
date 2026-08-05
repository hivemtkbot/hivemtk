# Bridge 前后端全流程 头脑风暴/审查/优化 论证报告

> 评审范围：Chrome 扩展 `user-web/bridge` + Go 服务端 `user-server/internal/bridge`
> 评审时间：2026-08-05
> 评审人：HiveMTK 工程组（Trae 自动化）
> 文档定位：实施前的"补漏清单 + 优先级 + 推荐方案"——后续修复单（issue list）唯一事实源

---

## 0. 阅读指引

- 第 1 节：7 段全链路速览与现状评估
- 第 2 节：43 项创意（按 7 段 × 审查/测试/优化 三视角）
- 第 3 节：10 项跨段系统级议题
- 第 4 节：P0/P1/P2/P3 优先级矩阵
- 第 5 节：测试矩阵缺口与补齐
- 第 6 节：3 周滚动执行路径
- 第 7 节：采纳矩阵（明确哪些做、哪些不做）
- 第 8 节：反向论证（不推荐项）

---

## 1. 7 段全链路现状

```
[上游]  平台网页 DOM
   ↓ ①监听 (MutationObserver + 3s 兜底轮询)
[扩展]  content script (BaseAdapter 4 渠道各自实现)
   ↓ ②上行帧 (chrome.runtime port)
[扩展]  background service worker (MV3)
   ↓ ③WS 上行 (BridgeClient)
[服务端] handler.go → toMessageEvent (同步)
   ↓ ④入站 (InboxIngressService)
[服务端] AgentRuntime → SmartCSOrchestrator
   ↓ ⑤LLM 推理 → BridgeReachAdapter
[服务端] Hub.Deliver (单 buffer, 锁外 seq)
   ↓ ⑥WS 下行 (outbound_reply 帧)
[扩展]  background → content port
   ↓ ⑦回写网页 (fillContentEditable + click send)
[下游]  平台私信发送
```

**已知现状**（来自 `.codebuddy/memory`）：
- ✅ 4 渠道（douyin/xhs/xianyu/tiktok）端到端已通；kuaishou 仅服务端常量
- ✅ LLM 选择器架构已废弃（commit `1bcca2e`），纯 CSS 选择器 + UI 配置
- ✅ 服务端 6 个 P0 大扫除：JWT 归属 / trace_id / send channel 关闭 / 并发安全 / XSS / 离线降级
- ✅ 扩展 51+ 单测 + 服务端 6 单测 + 1 e2e
- ⚠️ 真机未跑自动化巡检（仅 manual）
- ⚠️ kuaishou 前端未实现

---

## 2. 43 项创意（按链路段 × 视角）

### ① 上游 DOM 监听（content script）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| A1 | 审查 | `seen` Set 上限 5000 翻倍清理，会话切换时未清 → 旧指纹可能误判 | WeakSet 自动 GC，但 `seen` 是显式 Set，**会话切换需显式 reset** |
| A2 | 审查 | `isSystemMessage` 5 层漏斗在新版平台若气泡都加 `bubble` → 永远判为系统消息 | 4 渠道各自实现，应**统一收敛到 fallback.js** |
| A3 | 测试 | 缺「会话切换」时序场景测试 | 现有单测偏静态 |
| A4 | 测试 | MutationObserver 抖屏节流 100ms + 上限 50 丢弃，未做真机压测 | OOM 修复（2026-08-05）已加节流但**缺回归** |
| A5 | 优化 | 抽 `BaseAdapter._extractFromItem` → 4 渠道各实现 → 统一 `parseMessageItem` 仅路由 | 当前 4 渠道重复 ~200 行 |
| A6 | 优化 | `historyGraceMs` 硬编码 8000ms → 改 per-channel 配置 | 不同平台 SPA 渲染延迟差异极大（抖音 5s / XHS 2s / TikTok 6s+） |

### ② 上行帧（content → background port）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| B1 | 审查 | `safePost` 断开后直接丢弃不缓存，30s 内 5 条客户消息全丢 | 私域容错可接受，但**与"离线消息不丢"承诺矛盾** |
| B2 | 审查 | `bgStats` 用 setInterval 周期打印，SW 30s 闲置后定时器丢失 | MV3 已知问题，应改 `chrome.alarms` |
| B3 | 测试 | 真实 SW 生命周期（30s 闲置终止 → 唤起 → port 重连）未覆盖 | 现有 mock 端口不模拟 |
| B4 | 测试 | popup + content 同时 active 的多 port 竞态 | `registry.js` singleton 竞争 `ensure()` 路径 |
| B5 | 优化 | 扩展端 frame 加 CRC / `frame_id`，服务端可去重重复帧 | 当前仅服务端幂等，**WS 层冗余** |
| B6 | 优化 | meta 帧 5s 周期改 on-change 触发 | 已部分实现（lastMeta 比较）但 timer 仍在跑 |

### ③ WS 上行（BridgeClient → 服务端）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| C1 | 审查 | 旧连接 `readPump` defer `hub.Unregister` → 仍持 `c.conn` 引用 → 异步 Kick 关闭后 readPump 收 `use of closed network connection` | 锁顺序竞态窗口 |
| C2 | 审查 | `pongWait=60s` + `pingPeriod=50s` + 客户端 `serverIdleTimeoutMs=25s` 三者关系复杂 | 易踩坑：客户端先于服务端断开 |
| C3 | 审查 | 客户端 `_startPing` 每 25s 发 `pong` 帧 + 期待回 pong；高频对端会**双倍发 pong** | "乐观超时"在高频场景溢出 |
| C4 | 测试 | 服务端 200 并发已覆盖；缺"1000 并发 + 中途关闭"混合场景 | 需 `-race` 压测 |
| C5 | 测试 | 客户端 WS idle-recovery 真实行为（mock 不会触发 onclose 后 timer 清空） | 建议 `ws-idle-recovery.test.js` |
| C6 | 优化 | 暴露 `onStateChange('connecting'\|'open'\|'closed')` 状态机 | popup 状态更准 |
| C7 | 优化 | WS 帧带 `trace_id` 透传 | 服务端 `reply` 帧不带 → 客户端日志无法关联 |

### ④ 服务端入站（handler → InboxIngressService）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| D1 | 审查 | history 帧**未做整体去重**；同一帧重送达会重复落 N 条 | `InboxIngressService.NormalizeEvent` EventID 幂等，**两层幂等回退**未测 |
| D2 | 审查 | `f.Message == nil` 静默 return，**无错误日志** | handler.go:321-323 |
| D3 | 审查 | `globalReach` 单例 `SetBridgeReachAdapter` 启动期失败**无重试** | 启动期崩溃后整个 AI 出站静默失效 |
| D4 | 测试 | history 帧"History 字段为空但顶层有 EventID"混合场景未覆盖 | 边界 case |
| D5 | 审查 | `m.Timestamp == 0` 兜底 `time.Now()`，扩展端时钟错乱会污染 DB | 应使用服务器时间 + 窗口校验 |
| **D6** | **优化** | **handler 同步处理 Inbound 阻塞 readPump** | **WS 60s 超时**——P0 |
| D7 | 优化 | handler history 失败仅 Error 日志**无重试**；AI 已发但 history 落库失败 | write-ahead log 模式 |

### ⑤ LLM 推理（AgentRuntime → SmartCSOrchestrator）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| E1 | 审查 | AI 回复时延 2-5s，扩展 SW 30s 闲置被回收 → **客户端收到 outbound 时 SW 已重启** | `persistFailedOutbound` 兜底，**无定时器自动重连 SW** |
| E2 | 审查 | AI 内容 `>4KB` 被截断 → 客户看到"半句话" | 应在 AI 端控制 max_tokens，**截断应告警** |
| E3 | 审查 | 客户端 `dedupWindowMs=60s` 去重，**服务端 `ClaimReply` 不感知内容去重** | AI 卡死同内容重复发 |
| E4 | 测试 | AI 失败时（sensenova 熔断）reach_adapter 仍落库 success | 缺 mock provider 失败场景 |
| E5 | 测试 | 桥接渠道熔断/降级测试覆盖为零 | sop-041 webhook 错误已记录 |
| **E6** | **优化** | **AI 回复"礼貌延迟"（humanize）1.5-4s** | **拟人化 UX 提升**——P1 |
| E7 | 优化 | 客户发图片/语音，**没有"先读图/听语音再回复"策略** | 增强 AI 上下文 |

### ⑥ WS 下行（Hub.Deliver → BridgeClient）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| **F1** | **审查** | **seq 锁外生成，多 goroutine 拿到的 seq ≠ 投递顺序** | **多 AI 智能体并发时乱序**——P0 |
| **F2** | **审查** | **ping 与业务 share buffer (256×4KB=1MB)，buffer 满时 ping 也卡** | **应分通道**——P0 |
| F3 | 审查 | `rateBucket` `sync.Mutex` 保护 → 100+ 并发账号时锁竞争 | 改 `atomic.Int64` + CAS |
| F4 | 测试 | 100 账号同时 Deliver 的锁竞争 | 需 `-race` |
| F5 | 测试 | `OnBridgeClientOnline` 异步触发 + N 个消息重投，任一失败后面不重试 | 改 worker pool |
| F6 | 优化 | `Hub.Deliver` 失败仅 `persistFailedOutbound`，persist 失败时具体原因丢失 | 按 `errClass` 分桶 |
| F7 | 优化 | `frame_id` `Date.now()+Math.random()` 16 字符冲突概率（生日悖论：每 65k 帧/秒同毫秒） | 改 ulid/uuid |

### ⑦ 下游回写（content → 网页 DOM）

| # | 视角 | 创意 | 论证摘要 |
|---|---|---|---|
| **G1** | **审查** | **`fillContentEditable` 走 `execCommand('insertText')`，Vue/React 受控组件会拦截合成事件** | **抖音/XHS 已知需 `composed: true`**——P0 |
| G2 | 审查 | `sendOutbound` 切到目标会话后**没有等待渲染**直接 fill | `openConversation` 后有 `waitActiveMs=5000` 但 `sendOutbound` 不复用 |
| G3 | 审查 | 发送按钮未找到时 fallback `Enter` 键，**抖音 Enter 触发"换行"而非"发送"** | `xianyu.js:828` `keydown`/`keyup` 应改 `dispatchEvent` 完整模拟 |
| G4 | 测试 | "目标会话 ≠ 当前会话" 的 `openConversation` + `sendOutbound` 完整路径 | 关键 UX 路径未端到端 |
| G5 | 测试 | 4 渠道**真机未跑自动化** | 建议 Playwright 真机巡检 |
| **G6** | **优化** | **发送后等 200ms 再 fillContentEditable 硬编码** | **改 polling `waitFor`**——P1 |
| G7 | 优化 | 发送成功/失败无视觉反馈 | popup 显示"最近 10 条发送历史" |

---

## 3. 10 项跨段系统级议题

| # | 议题 | 优先级 |
|---|---|---|
| **H1** | 端到端 trace_id 贯穿 DOM→上行→handler→AI→下行→DOM 发送 | P2 |
| H2 | 选择器配置无版本管理（platform 改版后旧选择器污染） | P3 |
| H3 | `ProtocolVersionV2` 已加未真正使用，未来字段如何协商 | P2 |
| **H4** | **WS origin 限制（默认放行）→ 扩展被恶意站点重定向到假冒 server 时连接被劫持** | **P1** |
| **H5** | **无平台域名白名单 → 扩展可在任何网页注入（manifest matches 未严格化）** | **P0** |
| H6 | BaseAdapter 抽象后 kuaishou 接入 -80% | P3 |
| H7 | 扩展全失效时降级到网页客服 widget | P3 |
| H8 | rate limiter 调参无 A/B 框架 | P3 |
| H9 | `uid=0` 跳过归属校验、归 `user_id=0`（多商户部署会全部账号混在一起） | P0（私域单租户豁免，扩多租户是大坑） |
| H10 | `RetryFailedOutbound` 新 eventID 重投，客户端 seen 误判新消息为旧 | P2 |

---

## 4. 优先级矩阵

### P0（生产风险，优先做）

| # | 项 | 影响 | 工作量 |
|---|---|---|---|
| **D6** | handler 异步 + ack 立即回 | WS 60s 超时 | 1d |
| **F1** | 下行 seq 锁内生成 | 多 AI 并发错位 | 0.5d |
| **F2** | ping 与业务分通道 | buffer 满 ping 卡 | 0.5d |
| **G1** | Vue/React 受控组件填值 | 抖音/XHS 发送失败 | 1d |
| **H5** | 平台域名白名单（manifest + 运行时） | 注入污染 | 0.5d |
| **A6** | historyGraceMs per-channel | 新版平台首屏误触 AI | 0.5d |

### P1（健壮性，Q3 必做）

| # | 项 | 影响 | 工作量 |
|---|---|---|---|
| C6 | onStateChange 状态机 | popup 状态更准 | 0.5d |
| B1 | 扩展断线期 IndexedDB 缓存 | 离线消息不丢 | 1d |
| D1 | history 帧 EventID 集合去重 | DB 重复数据 | 0.5d |
| E6 | AI 礼貌延迟 1.5-4s | UX 拟人化 | 0.5d |
| G6 | 发送后 polling wait | 发送成功率↑ | 0.5d |
| H4 | WS origin 白名单 | 防劫持 | 0.5d |
| A5 | BaseAdapter 公共逻辑抽象 + kuaishou | 减 ~200 行 | 1d |

### P2（性能/可观测性，Q4）

| # | 项 | 影响 | 工作量 |
|---|---|---|---|
| H1+C7 | 端到端 trace_id 贯穿 | 排查效率 ↑↑ | 1d |
| D5 | 客户端时钟校验 | DB 污染 | 0.5d |
| A1 | seen Set 会话切换清理 | 旧指纹误判 | 0.5d |
| F7 | frame_id 改 ulid | 冲突概率 | 0.5d |
| H3 | 协议版本协商 | 演进能力 | 1d |
| B6 | meta 帧 on-change | 减背景噪音 | 0.5d |
| A3+B3 | session-switch + port 重连单测 | 覆盖率↑ | 0.5d |
| B2 | bgStats chrome.alarms | MV3 兼容 | 0.5d |
| G7 | popup 发送历史 | 运营可观测 | 1d |

### P3（优化与未来，backlog）

| # | 项 | 影响 | 工作量 |
|---|---|---|---|
| H2 | 选择器配置版本管理 | 平台改版管理 | 1d |
| H7 | 降级到 widget | 全平台失效 fallback | 3d |
| H8 | A/B 框架 | 调参科学化 | 3d |
| H10 | RetryFailedOutbound 内容指纹 | 误判新消息 | 0.5d |
| E7 | 图片/语音先读再回 | AI 上下文 | 1d |
| D3 | globalReach 启动重试 | 启动崩溃兜底 | 0.5d |

---

## 5. 测试矩阵缺口

| 维度 | 已覆盖 | 缺口 |
|---|---|---|
| 服务端单测 | hub 注册/抢占/限速/优雅关闭/并发 Deliver | FrameInbound 异步化、history 混合场景、AI 失败降级 |
| 服务端 e2e | `bridge_ingress_e2e`（HTTP 入站） | WS 双向 e2e + 真 LLM 推理 |
| 扩展单测 | 51 用例：协议/sanitize/fallback/rate-limiter/adapter/bridge-client | session-switch、port 重连、meta 帧 dedupe |
| 扩展 e2e | 无（manual） | Playwright 4 渠道真机巡检 |
| 真机回归 | 手动 | 4 渠道选择器过期检测脚本 |

---

## 6. 3 周滚动执行路径

### Week 1：P0 紧急修复（5 项 + 测试）
1. **D6** handler 异步化 + ack 立即回
2. **F1+F2** 下行分通道（ping/data）+ seq 在锁内
3. **G1** Vue/React 受控组件填值（dispatchEvent 完整化）
4. **H5** manifest `matches` 严格白名单 + 运行时域名校验
5. **A6** historyGraceMs per-channel 配置

### Week 2：P1 健壮性（7 项 + 测试）
1. **C6** onStateChange 状态机
2. **B1** 扩展断线期 IndexedDB 缓存
3. **A5** BaseAdapter 抽公共逻辑（kuaishou 顺便做）
4. **E6** AI 礼貌延迟
5. **D1** history 帧 EventID 集合去重
6. **G6** 发送后 polling wait
7. **H4** WS origin 限制

### Week 3：P2 可观测性（8 项 + 测试）
1. **H1+C7** 端到端 trace_id 贯穿
2. **A3+B3** session-switch + port 重连单测
3. **真机 e2e** 4 渠道 Playwright 巡检脚本
4. **B2** bgStats 改 chrome.alarms
5. **A1** seen Set 会话切换清理
6. **F7** frame_id 改 ulid
7. **D5** 客户端时钟校验
8. **H3** 协议版本协商 + v2 启用
9. **G7** popup 发送历史
10. **B6** meta 帧 on-change 触发

---

## 7. 采纳矩阵（明确做与不做）

| 推荐项 | 收益 | 风险 | 决定 |
|---|---|---|---|
| **A5 BaseAdapter 抽象** | 4 渠道减 200 行，kuaishou 接入 -80% | 行为回归 | **采用**（先 helper 层 + 4 渠道迁移单测） |
| **D6 handler 异步化** | 60s 超时风险归零 | AI 链路失败需独立 ack | **采用**（保留同步可配置） |
| **H1+C7 trace_id 全链路** | 排查时间 -80% | 需对齐协议 | **采用**（Week 3 主线） |
| **A6 historyGraceMs per-channel** | 误触 AI 风险归零 | 需平台实测定值 | **采用**（+ 灰度可配置） |
| **E6 AI 礼貌延迟** | 拟人化 UX | 时延感知变长 | **采用**（上限 4s） |
| **G1 Vue/React 填值** | 抖音/XHS 成功率↑ | 逐平台适配 | **采用**（每渠道独立 PR） |
| **H5 域名白名单** | 注入污染归零 | 平台域名变更需同步 | **采用**（manifest + 运行时双校验） |
| **H4 WS origin 限制** | 防劫持 | 需配置 | **采用** |
| **F1+F2 分通道** | 高频 AI 并发正确 | 协议小改 | **采用** |
| **C6 onStateChange** | popup 状态更准 | 小 | **采用** |
| **B1 IndexedDB 缓存** | 离线消息不丢 | 复杂度 | **采用**（轻量实现：内存 LRU + chrome.storage） |
| **G6 polling wait** | 发送成功率↑ | 行为略改 | **采用** |
| **D1 history 去重** | DB 干净 | 需加内存集合 | **采用** |
| **B2 chrome.alarms** | MV3 兼容 | 小 | **采用** |
| **H2 配置版本管理** | 平台改版管理 | 引入复杂度 | **P3 推迟** |
| **H8 A/B 框架** | 调参科学化 | 业务量不足 | **不做**（过早优化） |
| **C5+request_id 双层幂等** | 看似严密 | 引入两层幂等复杂度 | **不做**（得大于失） |
| **D7 history 失败阻塞 AI** | write-ahead 严谨 | 增加时延 | **不做**（history 缺失不影响 AI 回复） |
| **F7 ulid** | 冲突概率 10^-9 → 更低 | 改动大 | **不做**（理论够用） |

---

## 8. 反向论证（不推荐项）

- **C7 加 `request_id` + DB 去重**：现有 `event_id` 已做幂等，叠加会引入"两层幂等"复杂度，**得不偿失**
- **D7 history 落库失败阻塞 AI**：增加时延且 history 缺失不影响 AI 回复，**应 fire-and-forget**
- **H8 A/B 框架**：当前业务量不足以支持 A/B，**过早优化**
- **F7 改 ulid/uuid**：当前 16 字符随机后缀在低频场景**实际冲突概率 ~10^-9**，**理论够用**
- **H9 `uid=0` 跳过校验** 完整修复：私域单租户已豁免，**多租户演进时再处理**

---

## 9. 立即可落地的 5 件事（按 ROI 排序）

1. **真机 Playwright 巡检**（4 渠道 × 3 场景 = 12 用例）— 1d，立即发现选择器过期
2. **A5 BaseAdapter 抽公共逻辑** — 1d，长期收益最大
3. **D6 handler 异步化 + ack** — 1d，生产事故预防
4. **G1 Vue/React 填值补全** — 1d，抖音/小红书发送成功率提升
5. **H1 端到端 trace_id 透传** — 1d，排查效率翻倍

---

## 10. 关联文档

- `user-web/bridge/bridge.md`：bridge 主设计文档（架构/数据流/协议）
- `docs/bridge/DEFAULTS.md`：桥接默认值单一文档源（端口/限速/超时/协议）
- `.codebuddy/memory/2026-08-04.md`：上一次修复进展（Xianyu 接入）
- `.codebuddy/memory/2026-08-05.md`：本次大扫除
- `user-server/internal/bridge/`：服务端实现（handler/hub/frames/ai_selector/reach_adapter）
- `user-web/bridge/src/`：扩展实现（core/background/content/channels）
