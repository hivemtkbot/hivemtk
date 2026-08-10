# 架构整改清单（user-server）

> **文档级别**：⭐⭐⭐ 架构治理基线文件
> **审计日期**：2026-08-10
> **审计范围**：`hivemtk/user-server`（Go 后端主体），参照 `platform-server` 对比
> **审计方法**：全量 import 依赖矩阵扫描 + 分层违规 grep 取证 + 与既有规范（ADR-001 五层架构、`GO_FIVE_LAYER_ARCHITECTURE.md`、`DOMAIN_STRUCTURE.md`）对照
> **配套文档**：`GO_FIVE_LAYER_ARCHITECTURE.md`（规范）、`adr/ADR-001`（五层立项）、`internal/service/DOMAIN_STRUCTURE.md`（业务域蓝本）

---

## 一、现状概览（实测数据）

| 维度 | 数据 | 说明 |
|------|------|------|
| 非测试 Go 代码 | ~228,600 行 / ~1,450 文件 | 单体单进程 |
| `internal/service` | **192 个非测试文件 / 69,171 行** | 125 个 `*Service` struct、62 个 interface，单一扁平包 |
| `internal/controller` | 120 文件，117 个 import service | 其中 63 个 import model（违反 L3 规范） |
| `internal/repository` | 116 文件，72 个调 `db.GetDB()` | 全部经全局单例取 DB（Service Locator） |
| `internal/router` | 35 文件，**893 个路由注册点**，187 处 `New*Service()/New*Controller()` | 装配与路由声明混杂 |
| 垂直域包 | `aiagent`(176) / `content`(26) / `ops`(27) / `bridge`(9) / `channelbot`(3) / `platform`(8) | 自带 controller/service/repository/model，与共享层互相引用 |
| 空壳/半成品包 | `reach`、`domain`、`contract`、`identity`、`plugin`、`integration`、`system` | 迁移残留 |

**实测依赖图（与 ADR-001 声明的单向链对照）**：

```
声明:  cmd → router → controller → service → repository → model

实际:
  router ──▶ service/controller（装配，容忍）
  router ──▶ db.GetDB()（14 个文件，越权）
  router ──▶ repository / aiagent/tooluse / bridge（装配混杂）
  controller ──▶ model（63 文件）／repository、gorm（5 文件）／aiagent/llm（2 文件，跨层）
  service ──▶ aiagent（31 文件）⟲ aiagent ──▶ service（6 文件，语义互依）
  service ──▶ db.GetDB()（3 文件：telegram_polling*、lead_mining）
  model ──▶ aiagent/knowledge/model（3 文件，底层反向依赖域包）
  repository ──▶ content/model、aiagent/knowledge/model（2 文件，共享层耦合域模型）
  middleware ──▶ service / repository（3 文件）
  dto ──▶ model（8 文件）
  content/ops ──▶ 共享 internal/controller、internal/repository（12+ 文件）
```

---

## 二、问题清单（按严重度分类，含文件级证据）

### A. 模块边界类

| # | 问题 | 证据 | 影响 |
|---|------|------|------|
| A1 | **service 层 God Package**：192 文件扁平堆放，销冠/触达/客户/渠道/系统 8 大业务域物理混排，包内全可见 | `internal/service/*.go`；`DOMAIN_STRUCTURE.md` 已确认此问题但物理拆分 deferred | 命名冲突、跨域私有方法可被直调、无法按域做可见性收敛与团队分工 |
| A2 | **双范式并存无规则**：扁平五层与垂直域包（content/ops/aiagent）并存，互相引用 | `content/controller/*.go`、`ops/controller/*.go` import `marketing/internal/controller`、`marketing/internal/repository`；`router` 又反向 import `content/service`、`ops/controller` | 新代码不知道放哪；共享层与域层职责重叠（如两处都有 marketing_flow、custom_report 语义） |
| A3 | **空壳/半成品包**：迁移半途而废 | `internal/reach`（仅 `card/`）、`internal/domain`（仅 asset/errors）、`internal/contract`（1 文件）、`internal/identity`、`internal/plugin`、`internal/integration`（1 文件） | 包名承诺与实际内容不符，误导导航；`reach` 域真实代码仍在 `service/reach_pipeline.go` 等扁平文件中 |
| A4 | **pkg/utils 杂货铺**：29 个子包混装纯工具与基础设施 | `internal/pkg/utils/{db,mail,tgbot,cron,httpclient,config,...}` | `utils/db` 承载全局 DB 单例是基础设施而非工具；任何包都可 import utils 绕过分层 |
| A5 | **i18n 双份**：`internal/pkg/i18n` 与 `internal/service/i18n` 并存 | 两处均被 middleware/websocket/router 引用 | 职责不清，修改时不知改哪份 |

### B. 依赖方向类（违反 ADR-001）

| # | 问题 | 证据文件 | 整改动作 |
|---|------|---------|---------|
| B1 | controller 直接依赖 model（63 文件） | `controller/*.go` import `marketing/internal/model` | 出入参一律走 dto；确需实体透传的补 dto 映射 |
| B2 | controller 直连 repository/gorm | `controller/{knowledge_base,sop_template,agent_kb_binding,lead_mining,faq}.go` | 下沉逻辑到 service，controller 只留绑定+响应 |
| B3 | controller 跨层直引 aiagent/llm | `controller/{llm_routing,llm_provider}.go` | 经 `service`（system 域 LLM 路由服务）中转 |
| B4 | service 直连 DB | `service/{telegram_polling,telegram_polling_lock,lead_mining}.go` 调 `db.GetDB()` | 补 repository 方法收口 |
| B5 | **model 反向依赖域包**（最严重，底层污染） | `model/{ai_agent,platform_account_config,knowledge_aliases}.go` import `aiagent/knowledge/model` | 把别名/关联字段内联到 model 本身，或将 knowledge 共享类型下沉为叶子包 `internal/shared/model`（不得反向） |
| B6 | repository 跨域引用域模型 | `repository/{rag_health,system_stats}.go` import `content/model`、`aiagent/knowledge/model` | 迁移到各自域的 repository（`aiagent/knowledge/repository` 已存在） |
| B7 | aiagent ↔ service 语义互依 | service 31 文件 import aiagent（正常）；aiagent 6 文件反向 import service：`agent/tooluse/{private_message_tools,reach_tools,business_tools,reach_integration_adapter}.go`、`agent/bridge/sales_bridge.go`、`knowledge/controller/knowledge_workspace.go` | 反向侧改为**端口接口**：aiagent 只定义能力接口，由业务侧实现并在装配期注入（见 C3） |
| B8 | middleware 依赖 service/repository | `middleware/{permission,audit,app_key_auth}.go` | middleware 仅依赖窄接口（`PermChecker`、`AuditSink`、`AppKeyVerifier`），装配期注入实现 |
| B9 | dto 引用 model（8 文件） | `dto/{confidence,sales,clue_score,integration_template,customer_rfm,glossary,recovery_queue,asset_bundle}.go` | dto 自包含字段；枚举引用 `pkg/utils/type` |
| B10 | router 越权持 DB/业务装配 | router 14 文件调 `db.GetDB()`；`sales_engine_factory.go`、`tool_executor_wiring.go`、`inference_orchestrator_wiring.go`、`reach_sender_wiring.go` 等工厂文件 | 装配逻辑统一迁到 `cmd/api`（或新建 `internal/app`），router 只做"路径→handler"映射 |

### C. 封装与调用方式类

| # | 问题 | 证据 | 影响 |
|---|------|------|------|
| C1 | **Pull 式依赖获取（Service Locator）**：`NewXxxController()` 内部 `NewXxxService()` → `NewXxxRepository()` → `db.GetDB()`，依赖图完全隐式 | `controller/customer.go`、`service/customer.go`、`repository/customer.go`；`pkg/utils/db/db.go` 全局 `var DB *gorm.DB` | 单测必须起真 DB；无法替换 mock；循环依赖只能靠全局变量掩盖 |
| C2 | **service 包内自组装风暴**：466 处 `New*Service()` 互相实例化 | `grep New*Service() internal/service` | 同一服务多实例、生命周期不可控、无法统一注入事务/上下文 |
| C3 | **跨域调用直连具体类型**：tooluse 工具直接构造业务 Service；无端口/适配器约束 | `aiagent/agent/tooluse/reach_tools.go`（1,848 行）等 | AI 能力层与业务层焊死，任何 service 签名变更都波及 aiagent |
| C4 | **全局 setter 注入**：`router/globals.go` 持 `globalDispatcher/globalProviderFailover`；`agent_runtime.SetReplyGuardRedis` | `router/globals.go`、`cmd/api/main.go` | 隐式依赖、测试竞态；与 C1 同根 |
| C5 | **God 文件**：12 个非测试文件 >1,000 行 | `service/webhook.go`(2665)、`service/sales_engine.go`(2061)、`aiagent/agent/tooluse/reach_tools.go`(1848)、`service/inbox_ingress.go`(1785)、`repository/message_hub_inbox.go`(1690)、`service/reach_pipeline.go`(1498)、`aiagent/llm/dispatcher.go`(1447)、`content/service/marketing_flow.go`(1436)、`service/integration.go`(1134)、`service/inbox.go`(1122)、`service/asset_bundle.go`(1090)、`service/reach_send_pipeline.go`(1077) | 单文件承载多职责，评审与回归成本高 |
| C6 | **事件总线存在但未成为跨域通信主干**：`internal/event` 仅 bus/subscribers/types 三件套，跨域仍以直接调用为主 | `internal/event/bus.go`；`router/event_bus.go` | webhook→sales、reach→customer 等强编排链全部硬耦合 |

### D. 工程卫生类

| # | 问题 | 证据 |
|---|------|------|
| D1 | 源码树内存在 AI 工具生成的备份/临时目录：`internal/service/backups/`（54 个目录）、`internal/service/restore_tmp/`、`.data/` ×6（service/controller/model/content/platform） | 虽已 gitignore（`.gitignore` L74/L94），但污染工作区、干扰 IDE 检索，且工具行为不受控 |
| D2 | **架构护栏形同虚设**：`.golangci.yml` 中 depguard 规则 `files: ["$all"]` 匹配器配置错误（depguard v2 中 `$all` 不是合法关键字，规则不会生效）；且 Makefile 与 `.github/workflows` 均无 lint 任务 | `.golangci.yml` 全文仅 59 行；`grep lint Makefile` 为空 |
| D3 | 模块名 `marketing` 与目录 `user-server` 不一致 | `go.mod` L1；ADR-013 立项中未执行 |
| D4 | `cmd/api/main.go`（389 行）混杂 Redis 构建、健康探针适配、装配，无独立 bootstrap | `cmd/api/main.go` |

---

## 三、目标架构（整改后应达到的状态）

### 3.1 模块边界（单一范式：**共享基础层 + 垂直业务域**）

```
internal/
├── pkg/                      # 叶子基础设施：纯工具 + 无业务语义的基础组件
│   ├── db/ redis/ logger/ response/ pagination/ i18n/ tracing/ ...
│   └── (禁止出现业务类型；禁止 import 任何 internal 业务包)
├── model/                    # 仅保留"跨域共享实体"（Customer/Message/Merchant 等）——叶子包
├── dto/                      # 叶子包，禁止 import model/repository/service
├── aiagent/                  # AI 能力域（llm/rag/knowledge/agent），只暴露接口，禁止 import 业务 service
├── sales/ reach/ customer/ channel/ card/ rag/ marketing/ system/   # 8 业务域（对齐 DOMAIN_STRUCTURE.md）
│   └── 每域内部: controller/ service/ repository/ (model 仅域私有实体)
├── router/                   # 仅 URL→handler 映射 + 中间件挂载，禁止 New*、禁止 db.GetDB()
└── app/ (或 cmd/api)         # 唯一装配点：构造 DB/Redis/dispatcher → 注入各域
```

### 3.2 调用矩阵（强制，入 CI 护栏）

| 调用方 ↓ \ 被调方 → | pkg | model | dto | repository | service | controller | aiagent | event |
|---|---|---|---|---|---|---|---|---|
| router/app | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| controller | ✅ | ❌→dto | ✅ | ❌ | ✅(本域) | — | ❌ | ❌ |
| service | ✅ | ✅ | ✅ | ✅(本域+他域 repository 只读接口) | ✅(仅他域**导出窄接口**) | ❌ | ✅ | ✅ |
| repository | ✅ | ✅ | ❌ | — | ❌ | ❌ | ❌ | ❌ |
| aiagent | ✅ | ✅ | ✅ | ❌ | ❌(用端口接口) | ❌ | — | ❌ |
| model/dto | ✅ | — | — | ❌ | ❌ | ❌ | ❌ | ❌ |

**跨域调用三原则**：
1. 域 A 调域 B，只能调 B 的 `service` 包导出类型，且优先用 B 暴露的窄接口（Interface Segregation）；
2. 通知型交互（状态变更、事件广播）走 `internal/event` 总线，不直接调用；
3. 任何域不得 import 他域的 `controller` / `repository` / 私有 model。

### 3.3 封装与调用约定

- **构造注入**：所有 repository/service/controller 构造函数第一参数为依赖（`*gorm.DB`、依赖接口），禁止函数体内 `db.GetDB()` / `New*Service()` 拉取；
- **唯一装配点**：`cmd/api`（建议抽 `internal/app/app.go`）完成全部对象图构造，域包零全局变量；
- **事务归属**：事务由 service 开启并以 `tx` 传入 repository，repository 不自开事务；
- **接口位置**：端口接口定义在**使用方**（Go 惯例），实现在提供方；aiagent 的工具能力接口由 aiagent 定义、业务域实现。

---

## 四、整改清单（按优先级，含验收标准）

### P0 — 依赖方向纠偏与护栏（1~2 周，不改业务逻辑）

> **执行状态（2026-08-10）**：✅ P0-1~P0-7 全部完成。depguard v2 语法修复后实跑 0 issues 闭环；P0-5 中 B2（5 个越权 controller 下沉）完成，B3（llm_routing/llm_provider 跨层）顺延 P2 滚动。

| # | 动作 | 涉及文件 | 验收标准 |
|---|------|---------|---------|
| P0-1 | 修复 depguard 配置为 depguard v2 正确语法（按文件 glob 分域：`**/internal/controller/**`、`**/internal/model/**`...），并在 Makefile 增加 `make lint`、CI workflow 增加 lint job | `.golangci.yml`、`Makefile`、`.github/workflows/` | `make lint` 可复现跑出当前全部违规清单（作为基线），新代码违规 CI 红灯 |
| P0-2 | 消除 model→aiagent 反向依赖（B5） | `model/ai_agent.go`、`model/platform_account_config.go`、`model/knowledge_aliases.go` | `grep -r aiagent internal/model` 为空 |
| P0-3 | 迁移 repository 跨域 model 引用（B6） | `repository/rag_health.go`、`repository/system_stats.go` | 共享 repository 只 import 共享 model |
| P0-4 | service 3 处直连 DB 收口到 repository（B4） | `service/telegram_polling.go`、`service/telegram_polling_lock.go`、`service/lead_mining.go` | `grep db.GetDB() internal/service` 为空 |
| P0-5 | controller 5 个越权文件下沉（B2）+ 2 个 aiagent/llm 跨层文件中转（B3） | B2/B3 所列 7 文件 | controller 层不再出现 repository/gorm/aiagent import |
| P0-6 | 清理工作区垃圾目录并约束工具行为（D1）：删除 `internal/service/backups/`（54 目录）、`restore_tmp/`、各 `.data/`；在 `.githooks` 或工具文档中禁止在源码树内生成备份 | 见 D1 | 目录不存在；新增 pre-commit 检查拒绝 `internal/**/backups` 入库 |
| P0-7 | dto 8 个文件去除 model 引用（B9） | B9 所列 8 文件 | `grep -r 'internal/model' internal/dto` 为空 |

### P1 — 装配收敛与全局状态清除（2~4 周）

> **执行状态（2026-08-10）**：✅ P1-1（internal/app 装配包+wiring/globals/event_bus 迁移）、P1-2（router 清零 db.GetDB()，参数穿透）、P1-3（middleware 三窄接口）、P1-5（i18n 合并，纠偏：业务实现并入 `service/translation` 而非 pkg/i18n）完成。⚠️ 偏差：P1-2 验收「router 文件数 ≤20」实际 28 个非测试文件（纯路由表进一步拆分随 P2 滚动）；P1-4（repository 构造注入双轨）未单独立项，随 P2-1 域拆分推进。

| # | 动作 | 要点 | 验收标准 |
|---|------|------|---------|
| P1-1 | 新建 `internal/app` 装配包：集中构造 DB/Redis/llm.Dispatcher/ProviderFailover/event.Bus，逐域注入；移除 `router/globals.go` 全局 setter 与 `agent_runtime.SetReplyGuardRedis` 之类的散落注入 | 保持"禁用 DI 容器"约定，纯手工构造函数装配 | router 包不再出现任何 `New*Service()`/`New*Controller()` 以外的构造；`globals.go` 删除 |
| P1-2 | router 瘦身：`sales_engine_factory.go`、`tool_executor_wiring.go`、`inference_orchestrator_wiring.go`、`reach_sender_wiring.go`、`tool_provider_wiring.go` 迁入 `internal/app`；router 14 处 `db.GetDB()` 改由装配传入 | router 只保留路由表与中间件挂载 | `grep db.GetDB() internal/router` 为空；router 文件数 ≤20 |
| P1-3 | middleware 接口化（B8）：定义 `PermChecker`、`AuditSink`、`AppKeyVerifier` 窄接口，装配期注入 | middleware 不再 import service/repository | grep 验证 |
| P1-4 | repository 构造注入化（第一批）：`NewXxxRepository(db *gorm.DB)` 双轨过渡——保留无参构造函数内部转发 `db.GetDB()` 并标记 `// Deprecated`，新代码一律带参 | 从高频域（customer/message/merchant）开始 | 新代码不再出现无参 New；一个域完成后删除该域 Deprecated 构造函数 |
| P1-5 | 合并双份 i18n（A5）：以 `internal/pkg/i18n` 为唯一实现，`service/i18n` 业务词条加载并入后删除 | 引用方统一 import 路径 | 仅存一个 i18n 包 |

### P2 — service 域拆分与跨域封装（1~2 个月，按域滚动）

> **执行状态（2026-08-10）**：✅ P2-3 完成（aiagent 反向依赖全部端口化，含 event→repository 传递依赖修复；生产+测试对 service/repository/controller 引用 grep 为空，lint 0 issues）。⏳ P2-1/2/4/5/6 本次未执行——清单原定「1~2 个月按域滚动」，每域需先补 golden 测试并确认拆分窗口，不满足一次性整改的风控约束，列入下一阶段。现状盘点：service 顶层非测试文件 198（验收 ≤20）、>800 行 God 文件 22 个、content/ops 对共享 controller/repository 引用 16 文件。

| # | 动作 | 要点 | 验收标准 |
|---|------|------|---------|
| P2-1 | 执行 `DOMAIN_STRUCTURE.md` 物理拆分：按 sales/reach/card/customer/rag/marketing/channel/system 8 域建子目录，`git mv` + `package` 改名 + import 重写；跨域共享类型（SalesRequest/Message/Customer/ScriptTemplate）落 `internal/model` 或 `service/shared` | 单域单次提交，每域完成后 `go build ./... && go test ./...` | `internal/service` 顶层非测试文件 ≤20（仅跨域共享） |
| P2-2 | 域间窄接口化：每个域在 service 包内定义 `Port` 接口文件（如 `customer/port.go`），跨域调用只经接口；消除 466 处包内任意互调中的跨域部分 | 先治理 sales↔reach、channel→sales、webhook→customer 三条最热链路 | service 域间不再出现对方具体 struct 类型的字段/参数 |
| P2-3 | aiagent 反向依赖端口化（B7/C3）：`tooluse` 4 文件、`sales_bridge.go`、`knowledge_workspace.go` 改为依赖 aiagent 自定义端口接口，由 `internal/app` 注入业务实现 | reach_tools.go 同步拆分（见 P2-4） | `grep -r 'marketing/internal/service' internal/aiagent` 为空 |
| P2-4 | God 文件拆分（C5）：12 个 >1000 行文件按职责拆分，目标单文件 ≤600 行；优先 `webhook.go`（按渠道/方向拆）、`sales_engine.go`（9 步链路各成一节）、`message_hub_inbox.go` | 拆分不改行为，配 golden 测试 | 无 >800 行非测试文件 |
| P2-5 | content/ops 双范式归位（A2）：二选一——① 升格为独立业务域并入 8+N 域布局（marketing 域吸收 content，system/ops 吸收 ops）；② 保留垂直包但切断其对共享 `internal/controller`、`internal/repository` 的引用（自带 dto/依赖注入） | 推荐 ①，与 P2-1 合并执行 | content/ops 不再 import 共享 controller/repository |
| P2-6 | 事件总线主干化（C6）：webhook 入库→AI 触发、reach 发送→状态回写、客户标签变更 3 条链路改为事件驱动 | `internal/event` 增加按域订阅与失败重试语义 | 上述链路无跨域直接函数调用 |

### P3 — 长期治理（滚动）

> **执行状态（2026-08-10）**：✅ P3-1（ADR-015 六包逐项决策：plugin 删除，reach/domain/contract/identity 保留定性，integration 定性 test-only 集成落点）；✅ P3-2（utils 解体，ADR-012 已执行，config 合并入 internal/config）；✅ P3-3（ADR-013 已执行，方案 B：模块名 `marketing`→`hivemtk-user`，与 platform-server 的 `hivemtk-platform` 对称，未用原案 user-server）；✅ P3-5（depguard aiagent-layer 启用 + `scripts/dependency-matrix.sh` 季度审计脚本 + 首份归档 `docs/architecture/dependency-matrix-2026-08-10.md`，反向依赖快检为空）。⏳ P3-4 随 P2-1 域拆分滚动。

| # | 动作 | 要点 |
|---|------|------|
| P3-1 | 消灭空壳包（A3）：`reach` 域代码（service 内 reach_*）迁入后删除占位目录；`domain`/`contract`/`identity`/`plugin`/`integration` 按"补齐或删除"逐项决策并记录到 ADR |
| P3-2 | `pkg/utils` 解体（A4）：`db`/`mail`/`tgbot`/`cron`/`httpclient`/`config` 升为 `internal/pkg/<name>` 独立基础包；`functions`/`text`/`strhash` 等纯工具保留；禁止新增 `utils/*` 子包 |
| P3-3 | 执行 ADR-013：模块名 `marketing` → `user-server`（与 platform-server 对称），与 P2 拆分错峰执行避免大规模冲突 |
| P3-4 | controller→model 63 处引用逐步换 dto（B1），随各域 P2 拆分顺带完成，不单独立项 |
| P3-5 | 架构守卫常态化：depguard 规则随 P2 每次域拆分同步收紧；每季度跑一次依赖矩阵报告（可用 `go list -deps` 脚本化）归档到 `docs/architecture/` |

---

## 五、执行路线图

```
第 1~2 周   P0：护栏生效 + 方向性违规清零（纯纠偏，零业务风险）
第 3~6 周   P1：装配收敛、全局状态清除、repository 注入化（第一批）
第 2~3 月   P2：8 域物理拆分（每域一个迭代）+ 接口化 + God 文件拆分
持续        P3：空壳清理、模块改名、季度依赖审计
```

**回滚与风控**：
- P0/P1 每项独立提交、独立可回滚；P2 每域一次提交（沿用 `DOMAIN_STRUCTURE.md` §物理移动计划 1~8 步）；
- 所有拆分前先补 golden/集成测试（现有 `tests/*_regression_test.sh` 与 service 层 79 处 testutil 引用可作为基线）；
- 执行期间 air 热重载与 IDE git lock 问题（`DOMAIN_STRUCTURE.md` 所述历史阻塞）须在拆分窗口前确认解除。

---

## 六、既有资产说明（本清单不重复已有结论）

- 五层规范与违规清单：`GO_FIVE_LAYER_ARCHITECTURE.md`（本清单 B 节是其违规取证与量化）；
- 业务域蓝本：`internal/service/DOMAIN_STRUCTURE.md`（P2-1 直接执行它，不再重新设计域边界）；
- Bridge 链路功能审计：`.planning/2026-08-07-bridge-audit/`（与本清单正交，不覆盖其结论）；
- ADR 状态：ADR-013（模块改名 → P3-3）**已执行**（2026-08-10，方案 B `hivemtk-user`）、ADR-012（config 包迁移 → 随 P3-2）**已执行**（2026-08-10）、ADR-015（空壳包处置）**已执行**（2026-08-10）。

---

## 七、一次性执行记录（2026-08-10）

- **已完成**：P0 全部（7 项）、P1-1/2/3/5、P2-3、P3-1/2/3/5。
- **验证基线**：`go build ./...`、`go vet ./...`、`go test ./...` 全绿；`golangci-lint run` 0 issues（depguard 含 aiagent-layer 护栏生效）；依赖矩阵归档反向依赖快检为空。
- **遗留偏差**：P2-1/2/4/5/6 为按域滚动项（需逐域 golden 测试与拆分窗口，见 P2 状态注记）；P1-2 router 文件数 28>20；P1-4 未单独立项；P0-5 B3 顺延 P2；P3-4 随域拆分滚动。
- **后续抓手**：`scripts/dependency-matrix.sh` 每季度归档；depguard 随 P2 每次域拆分同步收紧。
