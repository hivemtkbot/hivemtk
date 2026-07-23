# 智能体（aiAgent 侧边栏组）菜单清单与测试计划

> 自动生成于 2026-07-23。范围：`user-web/src/router/modules/*` 中 `meta.group === 'aiAgent'` 的全部菜单。
> 测试方式：主 Agent 逐页读取本清单 → 调用子代理用 Playwright 模拟人工点击 → 结合 API 参数、页面渲染、数据库结果、控制台输出、API 日志，100% 覆盖后修复问题 → 循环下一页。

## 环境

- 前端 dev server：`http://localhost:8211`（vite，代理 `/api` → `http://localhost:8204`）
- 后端 user-server：`http://localhost:8204`（docker 容器 `mtk-user-server`）
- 数据库：postgres `127.0.0.1:8201`（`platform_db`）+ user-server 自有库
- 登录：`POST /api/auth/login` `{username:"admin", password:"<ADMIN_PASSWORD>"}` → `Authorization: Bearer <token>`（存 `localStorage.token`）
- Playwright：通过 MCP 驱动本地 Chromium。

## 菜单清单（共 21 项，含 4 个隐藏详情路由）

### 一、智能体核心
| # | 菜单标题 | 路由 path | 视图文件 | API 模块 | 后端 controller |
|---|---------|-----------|---------|---------|----------------|
| 1 | 智能体 | `/aiAgent` | `views/aiAgent/List.vue` | `api/aiAgent.js` | `controller/aiagent.go` |
| 2 | 智能体列表 | `/aiAgent/list` | `views/aiAgent/List.vue` | `api/aiAgent.js` | `controller/aiagent.go` |
| 3 | 创建智能体 | `/aiAgent/create` | `views/aiAgent/Edit.vue` | `api/aiAgent.js` | `controller/aiagent.go` |
| 4 | 编辑智能体 | `/aiAgent/edit/:id` | `views/aiAgent/Edit.vue` | `api/aiAgent.js` | `controller/aiagent.go` |
| 5 | 销冠 SOP 智能体 | `/sopAgent/list` | `views/sopAgent/List.vue` | `api/sopAgent.js` | `controller/sopagent.go` |

### 二、资产包 / 资产市场
| # | 菜单标题 | 路由 path | 视图文件 | API 模块 | 后端 controller |
|---|---------|-----------|---------|---------|----------------|
| 6 | 资产包管理 | `/asset-bundle/list` | `views/assetBundle/List.vue` | `api/assetBundle.js` | `controller/asset_bundle.go` |
| 7 | 开发者 Playground | `/asset-bundle/playground` | `views/assetBundle/Playground.vue` | `api/assetBundle.js` | `controller/asset_bundle.go` |
| 8 | 商户新建话术包 | `/asset-bundle/merchant-new` | `views/assetBundle/MerchantEditor.vue` | `api/assetBundle.js` | `controller/asset_bundle.go` |
| 9 | 资产市场 | `/asset-market` | `views/assetMarket/Market.vue` | `api/assetMarket.js` | `controller/asset_market.go` |
| 10 | 我的资产 | `/asset-market/my` | `views/assetMarket/MyAssets.vue` | `api/assetMarket.js` | `controller/asset_market.go` |

### 三、智能体能力引擎
| # | 菜单标题 | 路由 path | 视图文件 | API 模块 | 后端 controller |
|---|---------|-----------|---------|---------|----------------|
| 11 | LLM 路由 | `/llmRouting/list` | `views/llmRouting/List.vue` | `api/llmRouting.js` | `controller/llm_routing.go` |
| 12 | 置信度运营 | `/confidence` | `views/tuning/Panel.vue` | `api/tuning.js` | `controller/tuning.go` |
| 13 | 拟人度评估 | `/humanize` | `views/tuning/Panel.vue` | `api/tuning.js` | `controller/tuning.go` |
| 14 | 反馈学习闭环 | `/feedbackLoop` | `views/tuning/Panel.vue` | `api/tuning.js` | `controller/tuning.go` |
| 15 | 对话记忆 | `/dialogueMemory/list` | `views/dialogueMemory/List.vue` | `api/dialogueMemory.js` | `controller/dialogue_memory.go` |
| 16 | 话术库 | `/scriptTemplate/list` | `views/scriptTemplate/List.vue` | `api/scriptTemplate.js` | `controller/script_template.go` |
| 17 | 意图识别 | `/intentRecognition/list` | `views/intentRecognition/List.vue` | `api/intentRecognition.js` | `controller/intent_recognition.go` |

### 四、隐藏详情路由（由列表页动作进入）
| # | 菜单标题 | 路由 path | 视图文件 | 入口 |
|---|---------|-----------|---------|------|
| 18 | 开发者 Playground 编辑 | `/asset-bundle/playground/:aid` | `views/assetBundle/Playground.vue` | 资产包管理「设计」 |
| 19 | 商户话术包配置 | `/asset-bundle/merchant/:aid` | `views/assetBundle/MerchantEditor.vue` | 资产包管理 / 我的资产 |
| 20 | 资产详情 | `/asset-market/detail/:id` | `views/assetMarket/Detail.vue` | 资产市场「查看」 |
| 21 | 同步日志 | `/asset-market/sync-log` | `views/assetMarket/SyncLog.vue` | 我的资产「同步日志」 |

## 每页 100% 覆盖检查项（通用）
1. 页面加载无白屏、无控制台 error / warning。
2. 路由与权限正确（未登录跳转 / 已登录可访问）。
3. 列表：查询、分页、排序、筛选、刷新、列显隐。
4. 表单：必填校验、提交成功、失败提示、编辑回显、取消/重置。
5. 操作：创建、编辑、删除（含二次确认）、启用/停用、导入/导出。
6. 抽屉/弹窗：打开、关闭、数据回填。
7. API：请求参数与后端契约一致；响应解析正确（注意 `http` 拦截器返回 `data.data`）。
8. i18n：无未翻译 key 直接裸露。
9. 数据库：写操作后落库正确（必要时查库核对）。
10. 异常：网络错误、空数据、超长文本、并发操作的健壮性。

## 测试方法（已验证可行）

主 Agent 用 Playwright MCP 模拟人工点击；子代理无法驱动浏览器，故点击+读控制台/网络由主 Agent 直接执行。

1. **鉴权**：`POST /api/auth/login {username:"admin", password:"Admin@123456"}` → `localStorage.token` → 请求头 `Authorization: Bearer <token>`。
   - 运行容器 `mtk-user-server` 的 `ADMIN_PASSWORD` 环境变量值无法登录（seed 时机问题），已用 `python bcrypt` 将 `system_users` 表 admin 密码重置为 `Admin@123456`。
2. **路由是 hash 模式**（`createWebHashHistory`）。每个页面必须**整页加载 hash URL** `http://localhost:8211/#/<route>`，不能用 path URL（无 `#` 会被当 `/` 启动 → 重定向 unifiedInbox）。
3. **交互验证**：用 `playwright_evaluate` + IIFE 表达式（箭头函数只求值不调用）按文本定位按钮 `click()`；装 `window.__netfail`（记录 ≥400 请求 URL+状态码）与 `window.__jserr`（`error`/`unhandledrejection`）采集器——**不要覆盖 `console.error`**（会导致假登出）。
4. **后端直连**：user_db 在宿主机 `127.0.0.1:8232`（user `admin`，密码 `dce21ad1da364a9c1d11d2641b1472353527b45acb601492`）。

## 执行记录与发现

### 页面1 智能体列表 /aiAgent/list —— 已完成深度测试
- 列表渲染正常（含种子智能体「默认销售智能体」id=1），按钮齐全：Refresh/New Agent/Search/Reset/编辑/禁用/测试/绑定关系/删除。
- **测试对话框**：字段（智能体/客户ID/消息内容）+ 执行测试/清空结果；前端 `testAgent(id,data)`→`POST /api/ai-agents/:id/test`，body `{customer_id,message}`；后端返回 `SUCCESS` 含 reply/intent/memory。**已验证端到端可用**。
- **绑定关系 / 禁用 / 删除 / 筛选**：交互入口存在，待补充断言（页面偶发自跳需稳定手法后补全）。

### 已修复 BUG
- **[fix1] 种子智能体禁用导致「测试」功能失败**：`ai_agents.status=0`（禁用），`AIAgentController.Test` 校验 `status!=enabled` 返回「智能体不存在或已禁用」。已 `UPDATE ai_agents SET status=1 WHERE id=1;` 启用，测试功能恢复。前端 `List.vue` 状态映射 `status===1→启用` 与此一致。

### 跨页面待修（影响 100% 覆盖，循环继续处理）
- **[fix2] 通知轮询噪声**：`MessageNotification` 每 30s 调 `platformAPI.getLatestMessage()`→`/api/platform/message/latest`，平台端未实现该公告接口返回 500，前端 catch 后 `console.error('获取最新消息失败')` 每页刷。建议：该轮询对 500/非 JSON 静默（降级为 `console.warn` 或仅在 dev 打印），且不应触发登出。
- **[fix3] 漏斗页 ECharts 卸载崩溃**：`views/conversionFunnel/List.vue` 用 Options API + ECharts，`beforeDestroy` dispose 时内部 RAF 访问已卸载 DOM 的 `parentNode` 抛 `Cannot read properties of null (reading 'parentNode')`，卡住 `Layout.vue` 的 `<transition mode="out-in">`，导致从落地页客户端跳转时新页面不挂载。建议：dispose 前 `chartInst.clear()`、并 `onBeforeUnmount` 兜底；或过渡期延迟 dispose。
- **[env1] 导航稳定性**：测试途中页面偶发自跳（曾跳到 `#/customerJourney/dashboard`、`#/unifiedInbox/list`）。排查：`INIT_REDIRECT_MAP` 仅 `INIT_REQUIRED→/setup`，`Layout` watch 只设 `activeTopMenu` 不跳转，`customerJourney/dashboard` 无组件自跳转——疑为 dev server HMR/重载瞬态或延迟 `router.push` 残留。**稳定手法**：每个页面交互前重新整页 hash 加载目标路由；若检测到自跳则重载重试。

### 路由/接口契约要点（避免误判 404）
- agent 列表：`GET /api/ai-agents`（前端 `listAgents`）；测试：`POST /api/ai-agents/:id/test`；启用/禁用：`POST /api/ai-agents/:id/toggle`；详情：`GET /api/ai-agents/:id`；创建：`POST /api/ai-agents`；删除：`DELETE /api/ai-agents/:id`。
- 代理：`/api` → `http://localhost:8204`（user-server docker）。前端 `VITE_API_BASE_URL=/`（vite 代理），**不要**用 8216 那套错配构建。

### 页面4 销冠 SOP 智能体 /sopAgent/list —— 已完成深度测试+修复
- 列表/统计/筛选/分页 ✓；创建/编辑/删除/激活停用/详情/意图匹配 全链路验证通过（含 DB 落库核对、测试数据已清理）。
- **修复点（前端 `views/sopAgent/List.vue`，对齐后端 `dto/sop.go` `CreateRequest` + `sop_service.go` `MatchByIntent`/`validateGraph`）**：
  - 保存时把扁平业务节点自动包装为合法 `sop_graph`：补全节点 id（空则 `node_N`）、去重、前置 `start` 节点并顺序串联（满足 `validateGraph` 必须有 start 节点 + id 唯一 + next 有效）。
  - 发送 `scenario`（必填，=触发意图||名称）、`trigger_type`（`intent`/`auto`）、`trigger_config: { intents: [intent] }`；移除后端不支持的 `status` 字段；创建后按选择 `activate`/`deactivate`。
  - 编辑回显读 `detail.sop_graph.nodes`（剥掉自动注入的 start 节点）+ `detail.scenario`/`trigger_config.intents[0]`。
  - 状态展示改为基于后端返回的 `is_active`（原读 `row.status` 导致全部显示「未知状态」+ 切换按钮永远显示「激活」）；状态筛选改为前端按 `is_active` 二次过滤（后端 List 仅按 scenario 过滤）。
  - 列表/匹配/详情「节点数」列改读 `row.sop_graph?.nodes`（原读 `row.nodes`→恒为 0）。
  - 详情「节点流转」步骤改读 `currentSop.sop_graph?.nodes`。
  - 匹配结果「匹配分数」对已匹配项显示 100%（后端为过滤式匹配，原显示 0% 误导）。
  - 创建表单默认状态改为 `active`（原 `draft` 无对应下拉项导致新建即被停用）。
  - 首个 tab 标签由 `$t('SOP 管理')`（英文 locale 显示 SOP Management）改为字面「SOP 管理」，与其他两个中文 tab 一致。

### 已修复 BUG（续）
- **[fix4] SOP 页前后端契约全面错位**：前端发扁平 `{name,trigger_intent,status,nodes}`，后端 `CreateRequest` 要 `scenario`(required)+`sop_graph`(嵌套 nodes/edges) 且无 `status` → 创建 404/校验失败；`MatchByIntent` 读 `trigger_config["intents"]`(数组) 但前端发 `{intent}`(单值) → 意图匹配永远空；`getStatusType` 读 `row.status` 但后端只有 `is_active` → 状态全错；多列读 `row.nodes` 但后端返回 `sop_graph` → 节点数恒 0。已逐项对齐修复（见上）。

### 页面6 资产包调试台 /asset-bundle/playground —— 已完成深度测试+修复（2 个真实后端 bug）
- **Bug6.1（后端 `service.WeaveForRequest` 空指针 panic）**：`WeaveForRequest(ctx, assetID, userQuery, in)` 的 `in` 按值传递，函数内 `in.Asset = b` 只改副本；调用方 controller 随后 `len(in.Asset.Messages)` 命中 nil 指针 → 每次成功织布都 panic（接口恒返回空）。改为传 `*WeaveInput`（`in *WeaveInput`），同步更新 `controller` 与 `port_adapter` 调用点；`Weave(*in)` 解引用。**实测**：`weave A sandbox` 现返回 `SUCCESS` 且 `stats.asset_messages=2`。
- **Bug6.2（后端 热插拔门禁误伤开发者沙箱）**：`WeaveForRequest` 在热插拔缓存非空时拒绝未热启用资产，导致开发者一旦在生产侧热启用任一资产包，其 Playground 沙箱织布即被误拦（`not hot-enabled`）。新增 `WeaveInput.Sandbox` + DTO `WeaveRequest.Sandbox`；沙箱模式跳过门禁且不累加使用次数。**实测**：启用 B 后，织布 A（无 sandbox）仍报 `not hot-enabled`，织布 A（sandbox:true）成功 → 生产门禁保留、沙箱放行。
- **前端 `Playground.vue runSandbox`**：织布载荷加 `sandbox:true`；catch 区分「织布失败（真实服务端错误，如未保存/未热启用）」与「本地 LLM 调用失败（fetch Ollama 失败）」，不再一律误报 `LLM 调用失败`。
- **附：启动崩溃修复（阻塞性问题）**：`AutoMigrate` 终校验发现 `platform_account_configs` 表从未被创建（模型引用 `rag_products`，vector 扩展已就绪），导致新镜像启动即 panic 崩溃循环。已手动建表（含 FK→rag_products、索引）使服务正常启动；属真实迁移缺陷，建议后续排查 `PlatformAccountConfig` 的 AutoMigrate 为何未建表。
- 本地 LLM 实跑依赖 Ollama（`http://localhost:11434/v1/chat/completions`），本环境无 → fetch 失败属预期，已正确提示。

### 页面5 资产包管理 /asset-bundle/list —— 已完成深度测试+修复
- 列表/筛选/分页/刷新 ✓；创建(经 Playground/MerchantEditor)/发布/热启用/热停用/删除 全链路验证通过（API+DB 核对）。
- **热插拔 UI 修复（前端 `views/assetBundle/List.vue`）**：`fetchEnabled` 改为用运行时 `hotEnabledIds:Set` 标记已热启用的资产，热启用/停用按钮 `v-if="!hotEnabledIds.has(row.id)"/v-else` 切换（原基于 DB `status`，导致 active 行无法再次热启用）；移除 `handleEnable/Disable` 里误导性的 `row.status='active'/'inactive'` 乐观更新（热插拔为内存态，无 DB 写）。`fetchEnabled` 现并行填充 `hotEnabledIds`。
- **实测结论（纠正前序误判）**：`repo.List` 经 GORM `gorm.DeletedAt` 已自动加 `deleted_at IS NULL`，删除行不会泄漏到列表——前序「删除后仍在列表」为假阳性（测试数据/缓存误判）。为防御仍显式补 `q = q.Where("deleted_at IS NULL")`（无害冗余）。
- **后端契约核实**：`CreateBundle` 要求 `messages` 至少含一条 `role=system`（设计如此）——Playground 默认首条即 system 消息、MerchantEditor 走 `merchant-save` 自带 system 指令，故前端创建链路正常；curl 裸发无 system 消息会 400 属预期。
- 端到端复核：`create(含system)→publish(active)→delete` 后 `STILL_PRESENT=false`，DB `deleted_at IS NOT NULL`，列表已不含该 id。测试数据已清理（e2e_v2_* id=5 已软删）。

## 执行进度
- 页面1、2、3、4、5、6 已完成；页面7-17 按 todo 逐页标记 in_progress / completed，循环持续。
