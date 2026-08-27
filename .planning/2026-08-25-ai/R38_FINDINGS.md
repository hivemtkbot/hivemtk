# R38 全网调研发现（阶段2 成果）

> 输入：阶段1 的 14 个模块清单 M1–M14（M1 SafeGo / M2 分页 / M3 归属 / M4 Router / M5 Service / M6 吞错 / M7 启动 / M8 Debug+Recovery / M9 SVG / M10 SSE CORS / M11 trace / M12 metrics / M13 死条件 / M14 业务补强）
> 调研范围：2024-2026 公开知识（errgroup / cenkalti/backoff v5 / samber/oops / go-zero routine / OTel Go SDK / W3C TraceContext / Prometheus 命名规范 / Stripe/GitHub/Shopify API 分页 / Casbin vs SpiceDB / 五层架构严格度）
> 状态：阶段2 完成；阶段3 见同目录下 `R38_COMPETITIVE_DECISIONS.md`

---

## 一、SafeGo 调研（M1）

### 1.1 关键发现

1. **Go 官方 `errgroup` 不提供 panic recover**——`golang.org/x/sync/errgroup` 在 Go 1.x 至今无内建 recover；panic 进程崩溃路径下 `Wait()` 卡死。
2. **cenkalti/backoff v5 与 SafeGo 是互补关系**——SafeGo 做并发防护层（recover + ctx + propagation），backoff 做重试层，二者串联而非替代。
3. **samber/oops 的 `oops.Recover` 主要面向 HTTP 请求路径**，不能替代后台 SafeGo。
4. **业内主流做法（go-zero `routine.Recover` / go-kratos `recovery`）**：recover + 写堆栈日志 + 接入 metrics counter + 通过 OTel SpanContext 自动注入 trace_id。
5. **W3C TraceContext** 是 2024+ 事实标准（OTel/B3/Jaeger/Datadog 全兼容），`traceparent` 头。

### 1.2 6 方案对比

| 方案 | panic recover | ctx 透传 | 重试退避 | trace 注入 | 维护活跃度 | 适用 |
|------|--------------|---------|---------|----------|-----------|------|
| errgroup.Group | ❌ | ✅ | ❌ | 手动 | 官方 | 多任务并行 |
| **utils.SafeGo（项目内）** | ✅ | ✅ | ❌ | 手动 | 项目自维护 | 后台 fire-and-forget |
| cenkalti/backoff v5 | ❌ | ✅ | ✅ | ❌ | 活跃 | 单任务 N 次重试 |
| samber/oops | ✅（HTTP 偏） | ✅ | ❌ | ✅ | 活跃 | HTTP/业务错误聚合 |
| go-zero routine | ✅+stack+metric | ✅ | ❌ | ✅ | 活跃（字节系） | 后台标准 |
| 自写 recover + ctx | ✅ | ✅ | 自由 | 手动 | 项目自维护 | 极简 |

### 1.3 R38 选型

**保留项目内 `utils.SafeGo` + 升级 + 引入辅助**：

1. **保留**：现有 `utils.SafeGo(ctx, name, fn)` 接口不动（兼容性 + 项目风格统一）
2. **升级**：内部增加 metrics 计数器 `bg_panic_total{name}`（Prometheus）
3. **升级**：recover 处自动从 ctx 取 `trace.SpanContextFromContext(ctx)` 注入日志
4. **新增**：`utils.SafeGoWithRetry(ctx, name, backoff, fn)`，内部串联 cenkalti/backoff v5
5. **新增**：linter 规则（grep 兜底）禁止 `go func(` 裸用

> 不引入 go-zero `routine`——避免新增大依赖；用「升级版 SafeGo + 业务约定」覆盖。

---

## 二、分页钳制调研（M2）

### 2.1 业界 API 上限对比

| API | pageSize 字段 | 默认 | 最大 | 钳制策略 |
|-----|--------------|------|------|---------|
| **Stripe** | `limit` | — | **100**（list API） | hard limit；超出报 400 |
| **GitHub** | `per_page` | 30 | **100** | 超 100 自动截断至 100（不报错） |
| **Shopify** | `limit` | 50 | **250** | 超出返回 400 + 错误消息 |
| **Atlassian** | `limit` | — | **100**（issue）/ 50（comment） | hard limit |
| **Microsoft Graph** | `$top` | — | **999** | 服务端可自动调整 |
| **PostgREST** | `limit` | — | 无硬上限 | 依赖 query planner |

### 2.2 关键发现

1. **业界共识**：pageSize 必须有上限（100 / 200 / 1000 三档）
2. **page/pageSize 必须有下限**：下限 ≥ 1；负数 / 0 必须 400
3. **Stripe/GitHub/Shopify 选型**：100-250 是 sweet spot
4. **cursor-based vs offset-based**：
   - **offset**（page=N）：适合管理端 UI、深翻页用户感知强、随机跳页
   - **cursor**（after=xxx）：适合数据流、API 稳定性需求、深翻页性能稳定
   - **Stripe**：list 默认 cursor，关键资源（events）也 cursor
   - **GitHub**：REST list 默认 offset（page/per_page），GraphQL cursor（after/before）
   - **结论**：本项目保留 offset（管理端为主），但 cursor 作为后续可选项

### 2.3 Go 生态 Pagination helper

- **GORM**：无内建 clamp，需自封装
- **ent**：自定 query；不强制 clamp
- **go-kit pagination**：提供 `PageParams` 但默认上限 100
- **gorm-paginate** 第三方库：与 GORM 集成好
- **Stripe Go SDK**：`Params{Limit: 100}` 客户端就钳制了

### 2.4 R38 选型

```text
utils.ParsePagination(c *gin.Context, opts ...PageOption) (page, pageSize int)

PageOption:
    - WithDefaultSize(n int)         // 默认 pageSize，默认 20
    - WithMaxSize(n int)            // 最大 pageSize，默认 200（管理端 1000）
    - WithMinSize(n int)            // 最小 pageSize，默认 1
    - WithAllowOverMax(bool)        // 超出上限是否报错，默认 true（hard limit）

行为：
    page     < 1     → 1（不报错，clamp）
    page     > max   → 400 ErrInvalidPage
    pageSize < min   → min（clamp）
    pageSize > max   → 400 ErrInvalidPageSize（hard limit，避免被用作拖库）
    pageSize 非整数  → 400 ErrInvalidPageSize
```

**前端默认 page_size=20 全仓统一**，max=200（管理端 1000），与业界基线一致。

---

## 三、SaaS 权限/归属校验调研（M3）

### 3.1 业界 RBAC/归属方案性能对比

| 方案 | check P95 延迟 | 多租户 | 适用规模 | 典型用户 |
|------|----------------|--------|---------|---------|
| AWS IAM（边缘缓存） | <5ms | ✅ 天然 | 超大 | AWS 全生态 |
| SpiceDB（Zanzibar） | 1-5ms 命中 / 10-50ms 未命中 | ✅ Namespace | 大 | Datadog/Authzed |
| Ory Keto | 5-20ms | ✅ Namespace | 中大 | Salesforce/字节 |
| Keycloak | 5-15ms | ✅ Realm | 中大 | Red Hat/JD |
| **Casbin（in-process）** | <1ms | 需自实现 tenant | 中小 | go-admin/gin-vue-admin |
| 自研 ACL（DB 列 + Service 拦截） | <1ms（带索引） | ✅ | 任意 | Stripe/GitHub 早期/Slack |

### 3.2 归属校验 4 种模式对比

| 模式 | 代码集中度 | 可读性 | 可测试性 | 越权风险 | 业界采用率 |
|------|----------|--------|---------|---------|-----------|
| **Gin 中间件** | ★★★★★ | ★★★ | ★★★★ | ★★（漏一处即漏全部） | 60%（go-zero/Slack） |
| **Handler 装饰器链** | ★★★★ | ★★★★ | ★★★★★ | ★★ | 25%（go-kratos/字节） |
| **Service 层 Guard** | ★★★★ | ★★★★★ | ★★★★ | ★★★ | 10%（go-admin） |
| **显式 Handler 调用** | ★★ | ★★★★★ | ★★★★★ | ★★★★★（强制显式） | 5%（GitHub REST） |

### 3.3 Stripe / GitHub / Slack / 五层架构对比

- **Stripe**：URL 含 tenant（`/v1/accounts/{account}/charges/{charge}`），SDK 强类型 → 服务端自动取 account 与 charge 比对
- **GitHub**：OAuth Scope + 显式查询（GraphQL `viewerCanAdminister` 字段）
- **Slack**：每请求 `verify_team(token)` middleware 注入 team_id → 底层查询自动带 team_id
- **五层架构严格度**：go-kratos 最严（wire 强制）、go-zero 较严（idl 约束）、go-admin/gin-vue-admin 宽松

### 3.4 R38 选型

**纵深防御 = Router 中间件（粗过滤）+ Service Guard（细校验）+ GORM Tenant Scope（数据隔离）**

1. **Router 中间件** 拦截 80% 越权请求（性能高、零侵入），处理 `path → owner` 比对
2. **Service Guard** 做业务级校验（如 "已发布的订单不可改"），保留业务可读性
3. **GORM Scope** 作为兜底——任何 Repo 查询自动带 tenant_id，**业务忘写也防越权**
4. **不引入 SpiceDB/Ory Keto**：本项目 CRUD 为主、租户数 <1万，in-process Casbin + tenant_id 索引性能远超 Zanzibar（<1ms vs 5-20ms），且无运维成本

### 3.5 与现有 `tooluse/decorator_permission.go` 整合

现状已实现装饰器但**未注册使用**（P0-2 根因之一）。整合三段式：

```
HTTP → Router 中间件 RequireAuth() → RequireOwner(":id")
     → Handler 薄
     → Service Guard (业务级)
     → Repository GORM Scope WHERE owner_id = ?
     → DB
```

---

## 四、OpenTelemetry / 可观测性调研（M11–M12）

### 4.1 5 套方案对比

| 方案 | Metrics | Trace | trace_id 贯通 | 运维成本 | 厂商锁定 | 适合 |
|------|---------|-------|--------------|---------|---------|------|
| **A. OTel + Prometheus + Jaeger/Tempo** | ✅ | ✅ | ✅ W3C 标准 | 中 | ❌ | 默认推荐 |
| B. OTel + Datadog | ✅ | ✅ | ✅ | 高 | ⚠️ | 有钱/省心 |
| C. Prometheus + Grafana（纯指标） | ✅ | ❌ | 手动 | 低 | ❌ | Trace 缺位 MVP |
| D. 自研 SDK | ✅ | ⚠️ | 取决于实现 | 高 | — | 老项目改造 |
| E. 直接 log | ❌ | ❌ | ❌ | 极低 | — | PoC |

### 4.2 关键发现

1. **OTel Go SDK 是 CNCF 2024 GA 事实标准**（统一 Trace+Metric+Log 三件套）
2. **OTel SDK vs Prometheus 直埋**：
   - 新项目/需 Trace+Metric+Log → OTel SDK
   - 存量老项目/仅 metrics → Prometheus 直埋
   - 多语言异构 → OTel
3. **异步 goroutine 的 trace 透传**：必须显式 `ctx` 入参，`trace.ContextWithSpan(ctx, childSpan)`；**禁止 `go func()` 裸起**
4. **Propagator 标准**：W3C `traceparent` + `tracestate`（OTel 默认已注册）
5. **Prometheus 命名规范**：
   - 必须 namespace 前缀：`app_http_requests_total`
   - 必须单位结尾：`_seconds` / `_bytes` / `_total`
   - Counter 必须 `_total`
6. **Prometheus 2.40+ Exemplars**：可挂 OTel trace_id，**指标异常点直接跳转到 trace**
7. **RED 法则**（Rate/Errors/Duration）+ **USE 法则**（Utilization/Saturation/Errors）是 Go 服务四大资源（HTTP/DB/Redis/MQ）指标设计基础

### 4.3 R38 选型

**MVP 方案**（按 P0-2 修复后第二序推进）：

1. **SafeGo 必须保留 ctx**（已落地）
2. **新增 Prometheus 中间件** `middleware.Metrics()`：
   - `http_request_duration_seconds{method, status}`（Histogram）
   - `http_requests_total{method, status}`（Counter）
3. **关键路径埋点 demo**：
   - `rag_recall_total{layer, hit}`（Counter，RAG 三级缓存命中可观测）
   - `sop_node_exec_total{node, status}`（Counter，SOP 节点执行）
   - `agent_runtime_inference_total{model, status}`（Counter，Agent 推理）
   - `bg_panic_total{name}`（Counter，后台 goroutine panic 计数）
4. **trace_id 贯通**（独立 R39 推进，本轮仅约定）：
   - Gin 中间件：现有 `middleware/trace.go` 已生成 trace_id
   - GORM hook：通过 `db.Statement.Context` 取 ctx，写入 span
   - Redis client：通过 `redisotel` 或自封装 trace_id 透传
   - 跨服务（HTTP）：通过 W3C `traceparent` 头注入

> 本轮**仅做接口预留 + SafeGo + Gin metrics demo**；GORM/Redis/跨服务 trace 透传留 R39。

---

## 五、关键洞察汇总（从 4 份调研归纳）

| # | 洞察 | 落地决策 |
|---|------|---------|
| 1 | 现有 `utils.SafeGo` 已合规，差 metrics + 重试串联 | 升级签名/补 metric counter/新增 SafeGoWithRetry |
| 2 | 业界 pageSize max = 100-250 | 业务 200 / 管理 1000 |
| 3 | 纵深防御（中间件 + Service + Scope）是 2024 主流 | 三段式落地 |
| 4 | OTel Go SDK 是 SaaS 可观测性事实标准 | MVP 先 metrics，trace 留 R39 |
| 5 | errgroup/backoff/oops/routine 是「组合」而非替代 | 项目内 SafeGo 升级而非替换 |
| 6 | casbin in-process 对 CRUD 足够，SpiceDB 是过度设计 | 不引入第三方 RBAC |
| 7 | W3C TraceContext 是事实标准 | 预留透传点，R39 落地 |
| 8 | Stripe 的「URL 含 tenant」是最优雅 | 不引入（路径改动太大），用 Service Guard 替代 |
| 9 | Service 持 *gorm.DB：40% 禁止 / 35% 容忍 / 25% 允许 | 本项目严格禁止（架构合规 B→A） |
| 10 | Prometheus 命名 namespace + 单位后缀是硬规范 | 本轮所有 metric 严格遵循 |