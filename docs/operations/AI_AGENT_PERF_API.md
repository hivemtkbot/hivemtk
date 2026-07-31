# AI 智能体性能优化 API 文档

> **版本:** 1.0  
> **日期:** 2026-07-31  
> **维护:** HiveMTK 团队

本文档描述 `user-server` AI 智能体性能优化（5 阶段并行 + 双层架构 + WebSocket 流式）的 API 接口、DTO 协议、FeatureFlag 配置、性能指标、鉴权方式和限流规则。

---

## 一、接口列表

AI 智能体性能优化交付两类对外接口：REST 兼容旧通道 + WebSocket 新增流式通道。

| 通道 | Method | Path | 用途 | 状态 |
|------|--------|------|------|------|
| REST | POST | `/api/v1/ai/chat` | 兼容旧接口（同步返回完整回复） | **保留** |
| WebSocket | GET (Upgrade) | `/ws/chat` | 新增流式接口（增量推送 chunk） | **新增** |
| REST | GET | `/api/v1/ai/health` | AI Agent 健康检查 | 新增 |
| REST | GET | `/api/v1/ai/features` | 查询当前 FeatureFlag 状态 | 新增 |
| WebSocket | GET (Upgrade) | `/ws/admin/metrics` | 运维订阅实时指标流 | 新增 |

> TG / WeCom / Feishu / Xianyu 等外部渠道 webhook 继续走 `controller/ai_agent.go` 老路径，不受本次改造影响。

---

## 二、改造背景与目标

### 2.1 业务问题
- 客服对话 wall time 平均 **19.6s**（7B Q5 本地 LLM 实测）
- 主要瓶颈：Step 3 意图识别 LLM 串行阻塞 + Step 6 候选回复 1 次 LLM 调用 + 9 步流水线无重叠

### 2.2 量化目标
| 指标 | 改造前 | 目标 | 降幅 |
|------|--------|------|------|
| P50 wall time | 19.6s | < 1.5s | 92% |
| P90 wall time | 49.5s | < 5s | 90% |
| LCP 首字时间 | 19.6s | < 500ms (WS) | 97% |
| LLM 调用次数 | 2次/对话 | ≤1次/对话 | 50% |
| 规则/模板命中率 | 42% | > 75% | 33pp |

---

## 三、FeatureFlag 开关

通过环境变量控制，开关默认值如下（性能优化全开，调试日志关闭）：

| 开关 | 含义 | **默认值** | 推荐灰度 | 紧急回滚 |
|------|------|---------|----------|----------|
| `FF_PARALLEL` | 启用 SalesEngine 5 阶段并行化 | `1` (开启) | 0% → 5% → 25% → 50% → 100% | `FF_PARALLEL=0` |
| `FF_STREAM` | 启用 WebSocket 流式输出 (LCP < 500ms) | `1` (开启) | 同上 | `FF_STREAM=0` |
| `FF_LAYER1` | 启用 Layer1 FAQ/SOP 模板 SkipLLM | `1` (开启) | 同上 | `FF_LAYER1=0` |
| `FF_FALLBACK_CHAIN` | 启用 4 级降级链 (7B→3B→缓存→模板) | `1` (开启) | 50% 起步 | `FF_FALLBACK_CHAIN=0` |
| `FF_DEBUG_LOG` | 输出 phase 详细日志 (Steps 含 debug) | `0` (关闭) | 内部观察用 | `FF_DEBUG_LOG=0` |

> 5 个开关同时也是 **FeatureFlag 5 个 0/1 整数开关**，可通过环境变量、viper 配置中心、kubectl ConfigMap 注入。
> 5 个开关运行时全部读取，**viper.WatchConfig + SIGHUP 热加载**，无需重启进程即可生效。

**完整环境变量配置（推荐放进 systemd EnvironmentFile 或 k8s ConfigMap）：**

```bash
# /etc/hivemtk/ai-agent.env
FF_PARALLEL=1
FF_STREAM=1
FF_LAYER1=1
FF_FALLBACK_CHAIN=1
FF_DEBUG_LOG=0
```

**一键回滚（5 秒内）：**

```bash
export FF_PARALLEL=0
export FF_STREAM=0
export FF_LAYER1=0
export FF_FALLBACK_CHAIN=0
systemctl reload user-server  # SIGHUP 触发 viper 热加载
```

---

## 四、SalesEngine 5 阶段并行化

### 4.1 数据流

```
[入站消息]
    ↓
[Phase 0: 并行 fan-out]  ←  errgroup.WithContext
    ├─ resolveCustomer     ─┐
    ├─ recallMemory        ─┤ 4 任务并行
    ├─ IntentSpeculative   ─┤ (LLM 异步落库)
    └─ recallRAG           ─┘
    ↓
[Phase 1: 串行决策]
    ├─ 3.5 shouldTransfer
    ├─ 4    matchSOP
    ├─ 5.5  matchScript
    ├─ 5.6  playbook
    └─ 6    generateCandidate (Layer1 优先 → Layer2 LLM)
    ↓
[Phase 2: 异步收割]
    └─ 收割 IntentSpeculative LLM 结果 (10ms 超时, 不阻塞)
    ↓
[出站 SalesResponse]
```

### 4.2 接口

`SalesEngine.Handle(ctx, req) (*SalesResponse, error)`  
- 开关关闭时回退到原 9 步串行（向后兼容）
- 开关开启时走 5 阶段并行

### 4.3 步骤日志

`resp.Steps` 包含执行详情（可观测性）：

```go
type SalesStepLog struct {
    Step     string  // "0_phase_parallel" / "1_phase_serial" / "2_phase_async"
    Status   string  // "ok" / "fail" / "skip" / "timeout"
    LatencyMs int
    Detail   string
    Error    string
    Extra    map[string]any
}
```

---

## 五、双层架构 (Layer1 + Layer2)

### 5.1 决策逻辑

```
LayerRouter.Route(ctx, req) -> *LayerDecision
    ↓
1. FF_LAYER1 关闭 -> Layer2
    ↓
2. FAQ 匹配 (Top 1 score >= 0.6) -> Layer1 SkipLLM
    ↓
3. SOP 模板匹配 (Top 1 conf >= 0.65) -> Layer1 SkipLLM
    ↓
4. 默认 -> Layer2 (LLM 兜底)
```

### 5.2 LayerDecision DTO

```go
type LayerDecision struct {
    Layer      string  // "layer1" / "layer2"
    SkipLLM    bool    // true = Layer1 命中
    Reply      string  // Layer1 命中时的模板回复
    Reason     string  // "faq_hit" / "sop_hit" / "fallback" / "layer1_disabled"
    Confidence float64
    FAQID      uint
    SOPID      uint
    Intent     string
    WallMs     int
    Metadata   string
}
```

### 5.3 决策日志（落库 layer_decision_logs）

| 字段 | 类型 | 说明 |
|------|------|------|
| `trace_id` | varchar(64) | 端到端 trace |
| `layer` | varchar(32) | layer1 / layer2 / fallback_template / fallback_cache |
| `reason` | varchar(64) | faq_match / sop_template / llm_response / 7b_fail_3b / cache_hit |
| `conf_in` | decimal(5,4) | 输入意图置信度 |
| `conf_out` | decimal(5,4) | 决策后置信度 |
| `wall_ms` | int | 决策耗时 |
| `llm_skipped` | boolean | 是否跳过 LLM (Layer1 命中) |

---

## 六、请求/响应示例 (curl + wscat)

### 6.1 REST `/api/v1/ai/chat` (curl)

**请求：**

```bash
curl -X POST http://localhost:8080/api/v1/ai/chat \
  -H "Content-Type: application/json" \
  -H "X-Auth-Token: tk_live_abc123" \
  -H "X-Request-Id: req-20260731-0001" \
  -d '{
    "session_id": "s-20260731-001",
    "customer_id": "c-9988",
    "text": "韵达发货吗",
    "stream_mode": false,
    "metadata": {
      "channel": "web"
    }
  }'
```

**响应 (Layer1 命中 / 同步返回)：**

```json
{
  "trace_id": "t-20260731-abc123",
  "session_id": "s-20260731-001",
  "reply": "亲，韵达不发的哦，我们默认发邮政/顺丰～",
  "layer": "layer1",
  "reason": "faq_hit",
  "intent": "logistics",
  "confidence": 0.92,
  "wall_ms": 87,
  "model": "",
  "tokens": 0,
  "llm_skipped": true,
  "steps": [
    {"step": "0_phase_parallel", "status": "ok", "latency_ms": 23, "detail": "customer+memory+intent+rag"},
    {"step": "1_phase_serial", "status": "ok", "latency_ms": 5, "detail": "Layer1 FAQ hit, skip LLM"},
    {"step": "2_phase_async", "status": "skip", "latency_ms": 0}
  ]
}
```

**响应 (Layer2 LLM 兜底)：**

```json
{
  "trace_id": "t-20260731-def456",
  "session_id": "s-20260731-002",
  "reply": "可以的亲，这边推荐您看一下热销款 #A102…",
  "layer": "layer2",
  "reason": "llm_response",
  "intent": "product_recommend",
  "confidence": 0.78,
  "wall_ms": 1843,
  "model": "qwen-7b-q5",
  "tokens": 86,
  "llm_skipped": false,
  "steps": [...]
}
```

### 6.2 WebSocket `/ws/chat` (wscat)

**安装 wscat（如未装）：**

```bash
npm install -g wscat
```

**连接 + 鉴权 + 收发：**

```bash
# 1. 连接 (Header 携带 Token + 协议子协议)
wscat -c "ws://localhost:8080/ws/chat" \
  -H "X-Auth-Token: tk_live_abc123" \
  -H "X-Request-Id: req-20260731-0002" \
  -s "ai-agent-v1"

# 2. 发送用户消息 (在 wscat > 提示符下)
> {"type":"message","session_id":"s-20260731-001","customer_id":"c-9988","text":"你好,有什么推荐吗","stream_mode":true}

# 3. 服务端流式响应 (依次出现)
< {"type":"start","trace_id":"t-20260731-xyz789","session_id":"s-20260731-001","intent":"product_recommend","layer":"layer2","ts":"2026-07-31T10:00:00.123Z"}
< {"type":"delta","text":"亲，您好！","ts":"2026-07-31T10:00:00.523Z"}
< {"type":"delta","text":"推荐您看看","ts":"2026-07-31T10:00:00.612Z"}
< {"type":"delta","text":"热销款 #A102","ts":"2026-07-31T10:00:00.781Z"}
< {"type":"final","text":"亲，您好！推荐您看看热销款 #A102","wall_ms":1843,"layer":"layer2","model":"qwen-7b-q5","tokens":42,"steps":3,"llm_skipped":false,"ts":"2026-07-31T10:00:01.965Z"}

# 4. 主动取消
> {"type":"cancel","session_id":"s-20260731-001"}

# 5. 心跳 (推荐 30s 一次)
> {"type":"ping","ts":"2026-07-31T10:01:00.000Z"}
< {"type":"pong","ts":"2026-07-31T10:01:00.001Z"}

# 6. 关闭
> {"type":"close","session_id":"s-20260731-001"}
```

**Python 客户端示例（requests + websocket-client）：**

```python
import websocket, json, threading

def on_message(ws, msg):
    chunk = json.loads(msg)
    if chunk["type"] == "delta":
        print(chunk["text"], end="", flush=True)
    elif chunk["type"] == "final":
        print(f"\n[wall_ms={chunk['wall_ms']}] done")

def on_open(ws):
    ws.send(json.dumps({
        "type": "message",
        "session_id": "s-001",
        "customer_id": "c-9988",
        "text": "你好",
        "stream_mode": True,
    }))

ws = websocket.WebSocketApp(
    "ws://localhost:8080/ws/chat",
    header={"X-Auth-Token": "tk_live_abc123"},
    on_message=on_message,
    on_open=on_open,
)
ws.run_forever()
```

---

## 七、鉴权方式

AI Agent 通道对**前端 ChatWidget**、**内部微服务**、**第三方渠道** 三类调用方采用不同鉴权策略。

### 7.1 Token 类型矩阵

| 调用方 | Token 类型 | 传递方式 | 鉴权位置 |
|--------|-----------|---------|----------|
| 网页 ChatWidget (REST) | `tk_live_xxx` (会话级 JWT) | `X-Auth-Token` Header | 边缘网关 + user-server 中间件 |
| 网页 ChatWidget (WebSocket) | `tk_live_xxx` | `X-Auth-Token` Header (WS 握手) | WS Hub 握手阶段 |
| 内部微服务 (TG/WeCom) | `tk_svc_xxx` (服务级) | `X-Auth-Token` Header | internal middleware |
| 运维查询 (`/api/v1/ai/features`) | `tk_ops_xxx` (RBAC) | `X-Auth-Token` Header | admin middleware |
| 平台心跳上报 | `tk_platform_xxx` (固定) | mTLS + Header | 平台端独立监听 |

### 7.2 Token 格式

```
tk_live_<user>_<session>_<signature>
tk_svc_<service>_<env>_<signature>
tk_ops_<user>_<role>_<signature>
```

> 所有 token 由 `auth-service` 签发，HS256 签名，TTL 默认 1h，过期自动 401。

### 7.3 WebSocket 鉴权握手

```http
GET /ws/chat HTTP/1.1
Host: api.hivemtk.com
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
Sec-WebSocket-Version: 13
X-Auth-Token: tk_live_user001_s9988_eyJhbGciOiJIUzI1NiJ9...
X-Request-Id: req-20260731-0001
Sec-WebSocket-Protocol: ai-agent-v1
```

握手成功返回 `101 Switching Protocols`，失败返回：

| HTTP Code | 含义 | 处理 |
|-----------|------|------|
| `401` | Token 缺失/过期/签名错误 | 客户端重新获取 token |
| `403` | Token 有效但无权访问该 session | 客户端清理本地状态 |
| `429` | 触发限流 (见下文) | 客户端退避重试 |

### 7.4 智能体知识库隔离

- 每个 `tk_live_xxx` token 内嵌 `agent_id` claim
- 所有 FAQ / SOP / LayerDecisionLog 查询强制 `WHERE owner_agent_id IN (...)`
- 越权访问立即返回 `403 agent_mismatch`，**写入审计日志**

---

## 八、限流规则

为防止恶意流量击穿 LLM 推理栈，AI Agent 通道在 3 个层级实施限流。

### 8.1 限流策略表

| 层级 | 维度 | 算法 | 默认阈值 | 超限行为 |
|------|------|------|---------|----------|
| **L7 边缘** | 每 IP 每秒 | 令牌桶 (token bucket) | 20 req/s | `429` + `Retry-After: 1s` |
| **L4 网关** | 每 Token 每秒 | 滑动窗口 (sliding window) | 10 req/s | `429` + 退避 2s |
| **L4 网关** | 每 Token 并发 WS | 信号量 (semaphore) | 5 并发 | WS 关闭 + 1008 |
| **L1 进程** | 全局 LLM 调用 | 漏桶 (leaky bucket) | 5 req/s | 排队 + 60s 超时 |
| **L1 进程** | 单 session 60s 请求 | 计数器 (counter) | 30 req/60s | `429` + 锁定 60s |

### 8.2 限流响应头

```http
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 2
X-RateLimit-Limit: 10
X-RateLimit-Remaining: 0
X-RateLimit-Reset: 1722441600

{"error":"rate_limited","scope":"token","retry_after_ms":2000}
```

### 8.3 WebSocket 限流

WebSocket 通道在握手时检查 Token 级别限流，**握手成功后仍受单 session 60s 30 req 计数限制**。超限流程：

1. 服务端主动发送 `{"type":"error","error":"rate_limited","retry_after_ms":5000}`
2. 等待 1s 后关闭连接 (Close Code `1008` Policy Violation)
3. 客户端需等待 5s 后重新连接

### 8.4 内部降级开关

```bash
# 全局限流熔断 (P0 故障时启用)
export AI_RATE_LIMIT_DISABLED=1
systemctl reload user-server
```

> 该开关仅作为 **P0 故障逃逸** 用，平时保持关闭。

---

## 九、WebSocket 流式输出协议

### 9.1 端点

`/ws/chat` (WebSocket Upgrade)

### 9.2 协议 (JSON over WS)

```json
// 客户端发送
{"type":"message","text":"你好","session_id":"s1","customer_id":"c1"}

// 服务端流式响应
{"type":"start","trace_id":"t-123","intent":"greeting","layer":"layer1"}
{"type":"delta","text":"你好"}
{"type":"delta","text":"，" }
{"type":"delta","text":"有什么能帮你？"}
{"type":"final","text":"完整回复","steps":[...],"wall_ms":1234,"layer":"layer2","model":"qwen-7b","tokens":42}
```

### 9.3 Chunk 类型

| `type` | 含义 | 字段 |
|--------|------|------|
| `start` | 流开始 | `trace_id`, `intent`, `layer` |
| `delta` | 增量文本 | `text` |
| `final` | 流结束 | `text`, `steps`, `wall_ms`, `model`, `tokens` |
| `error` | 错误 | `error`, `retry_after_ms`(可选) |
| `cancel` | 取消 | - |
| `ping` / `pong` | 心跳 | `ts` |

### 9.4 LCP 优化

- **chunk 1** (LCP): Layer1 命中 → `<100ms` 返回 start + final
- **chunk 2-N**: Layer2 LLM → 流式 delta (每 50ms 一批)
- **fallback**: 60s 仍无 LLM 响应 → 推送 `default_template` chunk

---

## 十、智能降级链

### 10.1 4 级降级

```
请求 → Provider "default" (本地 7B)
       ↓ 失败/超时
       Provider "3b_local" (本地 3B 兜底)
       ↓ 失败/超时
       Cache Hit (Redis 24h TTL)
       ↓ 失败/超时
       Default Template (内置 10 条兜底话术)
```

### 10.2 触发条件

- 单 provider P99 延迟 > 配置的 `MaxLatency`
- 连续 3 次失败 → 临时禁用 5 分钟（自动恢复）
- token_rate 异常突增 → 切到缓存或模板

---

## 十一、性能指标

### 11.1 核心指标

| 指标名 | 类型 | 标签 |
|--------|------|------|
| `ai_agent_wall_time_seconds` | Histogram | agent_type, layer, intent |
| `ai_agent_lcp_time_seconds` | Histogram | agent_type, stream_mode |
| `ai_agent_layer_decision_total` | Counter | layer, reason |
| `ai_agent_llm_call_total` | Counter | scenario, model, result |
| `ai_agent_fallback_total` | Counter | from_layer, to_layer, reason |

> 指标采集由 `layer_decision_logs` 表落库审计; 不依赖外部监控面板/告警通道, 故障排查通过 SQL 查询即可。

---

## 十二、FAQ 数据集

### 12.1 数据源

从 `E_commerce_Customer_Service/test_clean_v2.jsonl` 自动提取 Top 50 高频问答。

### 12.2 提取脚本

```bash
python3 scripts/extract_faq.py
# 生成 scripts/faq_seed.json (50 条)
```

### 12.3 导入工具

```bash
cd hivemtk/user-server
go run cmd/importfaq/main.go -input ../scripts/faq_seed.json
# 输出: [OK] / [SKIP] / [FAIL] + 统计
```

### 12.4 分类

| 分类 | 关键词 | 占比 |
|------|--------|------|
| logistics | 邮/快递/发货/韵达 | ~20% |
| pricing | 价格/优惠/折扣 | ~12% |
| aftersales | 退/换/退款 | ~10% |
| product | 尺码/颜色/材质 | ~10% |
| order | 活动/促销/订单 | ~8% |
| general | 其他 | ~40% |

---

## 十三、兼容性

### 13.1 向后兼容

- ✅ `SalesEngine.Handle` 入口签名不变
- ✅ `SalesResponse` 字段全部保留
- ✅ `llm_routing_logs` 落库格式不变
- ✅ PG schema 新增 3 表，不改现有表
- ✅ Controller 路由新增 `/ws/chat`，REST 端点保留
- ✅ FeatureFlag 默认全开启，关闭时回退到旧版 9 步串行

### 13.2 升级路径

1. 部署新代码 → FeatureFlag 全开启（性能优化生效）
2. 跑 FAQ 提取 + 导入 → 验证 Layer1 命中
3. 灰度 5% `FF_PARALLEL=1` → 观察 P50/P90
4. 灰度 5% `FF_LAYER1=1` → 观察 P50/Layer1 命中率
5. 灰度 5% `FF_STREAM=1` → 观察 LCP
6. 逐步放量到 100%

---

## 十四、错误码

| 错误 | 含义 | 处理 |
|------|------|------|
| `AI_LAYER1_DISABLED` | FF_LAYER1=0 | 显式回退到 Layer2 |
| `AI_FAQ_NOT_FOUND` | FAQ 库空 | 走 SOP 模板 |
| `AI_SOP_RENDER_FAIL` | 模板渲染失败 | 走 Layer2 LLM |
| `AI_LLM_TIMEOUT` | LLM 60s 超时 | 走降级链 |
| `AI_ALL_FALLBACK_FAIL` | 4 级全失败 | 返回默认错误模板 |
| `AI_RATE_LIMITED` | 触发限流 | 客户端按 `Retry-After` 退避 |
| `AI_AGENT_MISMATCH` | 越权访问其他智能体数据 | 拒绝 + 审计日志 |
| `AI_WS_AUTH_FAIL` | WS 鉴权失败 | 关闭连接 (1008) |

---

**版本:** v1.0  
**最后更新:** 2026-07-31  
**审查:** HiveMTK 架构组
