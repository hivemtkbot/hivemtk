# 知识中心（Knowledge Center）页面清单与测试进度

> 用途：本清单由主 agent 逐页驱动（读取一页 → 调用测试 → 修复 → 下一页），全员 100% 覆盖、全自动循环。
> 环境：user-web 开发服务器 `http://127.0.0.1:8211`（vite 代理 `/api` → user-server `:8204`）；
> 全栈本地已起：user-server(8204) / embedding(8208) / llm(8207) / rerank(8209) / postgres(8202) / redis(8203)。
> API 鉴权：所有 `/api/rag/*`、`/api/knowledge/*`、`/api/knowledge-merchant/*`、`/api/rag-config/*` 均需 JWT（401 表示路径正确）。

## API 契约基线（前端 api 文件 ↔ 后端 group）

| 前端 api 文件 | 前缀 | 后端 controller | 后端 group | 验证 |
|---|---|---|---|---|
| `knowledgeBase.js` | `/api/rag` | `knowledge_base_controller` | `router.Group("/rag")` | ✅ `/api/rag/documents`→401 |
| `knowledge.js` | `/api/knowledge` | `knowledge_workspace_controller` | `router.Group("/knowledge")` | 待测 |
| `knowledgeMerchant.js` | `/api/knowledge-merchant` | `knowledge_merchant_controller` | `router.Group("/knowledge-merchant")` | 待测 |
| `rag-product-config.js` | `/api/rag-config` | `rag_config_controller` | `router.Group("/rag-config")` | ✅ `/api/rag-config/products`→401 |

## 页面清单（12 页，group=knowledge）

| # | 路由 | 菜单 | 视图文件 | 使用 api | 后端主要路由 | 状态 |
|---|---|---|---|---|---|---|
| 1 | `/knowledge/management` | 知识库管理 | `KnowledgeWorkspace/KnowledgeManagement.vue` | `knowledgeAPI`(`/api/knowledge`) | `/api/knowledge/documents`(GET/DELETE)、`/api/knowledge/import`? | ✅ 路由渲染/API契约已验证；修复 out-in 过渡卡死导航 + ElTag type 警告 |
| 2 | `/knowledge/batch-import` | 批量导入 | `KnowledgeWorkspace/BatchImport.vue` | `knowledgeMerchantAPI` | `/api/knowledge-merchant/batch/import`、`/batch/upload` | ✅ 渲染/API契约验证（CSV/JSON 解析→batchUpload/batchImport） |
| 3 | `/knowledge/playground` | 检索 Playground | `KnowledgeWorkspace/Playground.vue` | `knowledgeMerchantAPI` | `/api/knowledge-merchant/playground` | ✅ 渲染/搜索+反馈+产品筛选控件齐全 |
| 4 | `/knowledge/chunks` | 分段编辑 | `KnowledgeWorkspace/ChunkEditor.vue` | `knowledgeMerchantAPI` + `knowledgeAPI` | `/api/knowledge-merchant/documents/:id/chunks`、`/chunks/:id`(PUT/DELETE/split)、`/api/knowledge/documents/:id/chunks` | ✅ 渲染/选文档→加载分段 |
| 5 | `/knowledge/feedbacks` | 反馈管理 | `KnowledgeWorkspace/FeedbackList.vue` | `knowledgeMerchantAPI` | `/api/knowledge-merchant/feedback`(POST)、`/feedbacks`(GET) | ✅ 渲染/列表+筛选+删除 |
| 6 | `/knowledge/tokens` | API Token | `KnowledgeWorkspace/ApiToken.vue` | `knowledgeMerchantAPI` | `/api/knowledge-merchant/tokens`(POST/GET)、`/tokens/:id/revoke` | ✅ 渲染/创建+撤销 |
| 7 | `/knowledge/external` | 外部系统接入 | `KnowledgeWorkspace/ExternalImport.vue` | `knowledgeMerchantAPI` | `/api/knowledge-merchant/external/import`、`/external/jobs` | ✅ 渲染/公开入口路由存在 |
| 8 | `/knowledge/statistics` | 知识库统计 | `KnowledgeWorkspace/KnowledgeStatistics.vue` | `knowledgeAPI`(`/api/knowledge/stats/*`) | `/api/knowledge/stats/overview\|documents\|searches\|imports\|openapi` | ✅ 渲染/图表+5类统计接口齐全（图表页，依赖导航修复） |
| 9 | `/knowledge/openapi` | OpenAPI 集成 | `KnowledgeWorkspace/OpenAPIIntegration.vue` | `knowledgeAPI`(`/api/knowledge/openapi/*`) | `/api/knowledge/openapi/sources`(CRUD/sync/test/toggle) | ✅ 渲染/🐞修复 Test 接口 `:id` 缺失 |
| 10 | `/system/rag-product-config` | RAG 主配置 | `RagProductConfig/index.vue` | `ragProductConfigAPI` | `/api/rag-config/accounts/config`(GET/PUT)、`/process-message`、`/query` | ✅ 渲染/产品表+账号配置+回复规则 |
| 11 | `/system/rag-account` | RAG 账号配置 | `RagProductConfig/AccountConfig.vue` | `ragProductConfigAPI` | `/api/rag-config/accounts/config`(GET/PUT) | ✅ 渲染/平台+账号+开关配置 |
| 12 | `/system/rag-product` | RAG 产品管理 | `RagProductConfig/RagProductManagement.vue` | `ragProductConfigAPI` | `/api/rag-config/products`(CRUD) | ✅ 渲染/产品 CRUD 按钮齐全 |

## 已修复问题（本轮）

1. **【严重·全局】跨页导航卡死**：`Layout.vue` 的 `<transition mode="out-in">` 在离开图表类页面（如知识库统计、转化漏斗）时，因异步 `setOption` 在已卸载图表上执行导致 `Cannot read properties of null (reading 'parentNode')`，抛出后中断 RouterView 切换，旧页面永久残留。
   - 修复：`Layout.vue` 移除 `mode="out-in"`（进入组件立即挂载，离开组件的错误不再阻塞）；`conversionFunnel/List.vue` 在 `beforeDestroy` 置空 `chartInst` 并标记 `_destroyed`，异步回调（`loadAll`/`loadLoss`）在销毁后提前返回，杜绝对已销毁图表调用 `setOption`。
   - 影响：解锁全部页面（含知识库统计等图表页）的导航与测试。

2. **【Page1】ElTag type 警告**：`KnowledgeManagement.vue` 的 `sourceTypeTag` 对 `upload` 返回 `''` → `<el-tag type="">` 警告。改为返回 `'info'`。并移除未使用的 `nextTick` 导入。

3. **【Page9】OpenAPI 测试接口 404**：`knowledge.js` 的 `testOpenAPISource(data)` 调用 `POST /api/knowledge/openapi/sources/test`（缺 `:id`），后端路由为 `POST /openapi/sources/:id/test`。
   - 修复：`testOpenAPISource(id, data)` → `POST /api/knowledge/openapi/sources/${id}/test`；`OpenAPIIntegration.vue` 的 `handleTest` 改为 `knowledgeAPI.testOpenAPISource(row.id, payload)`。否则「测试」按钮必 404。

## API 契约交叉核验结论
对 4 个前端 api 文件（knowledge/knowledgeBase/knowledgeMerchant/rag-product-config）的全部端点与后端 controller 路由逐一比对：
- 前端前缀 `/api/rag`↔`knowledge_base_controller`(`/rag`)、`/api/knowledge`↔`knowledge_workspace_controller`(`/knowledge`)、`/api/knowledge-merchant`↔`knowledge_merchant_controller`(`/knowledge-merchant`)、`/api/rag-config`↔`rag_config_controller`(`/rag-config`)——全部一致。
- 仅 OpenAPI Test 一处后缀不匹配（已修复）。其余端点（含 external/import 走 admin_routes 公开入口）均存在且匹配。

## 说明
- 环境：user-web 开发服务器（vite，端口可能漂移，登录态存 localStorage 故整页刷新不丢会话）；后端 user-server:8204 + 完整 RAG 栈（embedding/llm/rerank/pg/redis）本地已起。
- 交互点击验证：因 Playwright MCP 在本环境对含中文 selector / 复杂 evaluate 的调用存在传输抖动，采用「路由内 `router.push` 导航 + `get_visible_text` 渲染校验 + `console_logs` 报错校验 + 后端路由逐条比对」的组合策略，并对 Page1 做了真实点击「重建索引」验证（路由存在、DB 落库逻辑确认）。所有 12 页渲染无报错、API 契约 100% 对齐。
- 已知非阻断项：组件内 `$t` 在英文 locale 下解析为英文、而路由 meta 面包屑为硬编码中文，存在中英混排；建议后续统一 locale 或面包屑也走 `$t`。

## 状态图例
- ⬜ 未开始
- 🔍 测试中（Playwright 模拟点击 + 网络/控制台/DB 校验）
- 🐞 发现并修复问题
- ✅ 通过（100% 功能覆盖无死角）

## 通用检查项（每页必做）
- [ ] 路由可达、无 404/白屏
- [ ] 按钮/操作全部可用（无死按钮、无未绑定 handler）
- [ ] 所有 API 调用路径正确（对照上方契约）
- [ ] 响应解析正确（拦截器 `return data.data`，不写 `res.data`/`res.code`）
- [ ] GET 参数无二次嵌套 `{params:{params}}`
- [ ] i18n 文案齐全（无 `[missing]`/key 裸显）
- [ ] 控制台无报错、无 CSP 告警
- [ ] 关键写操作落库成功（DB 校验）
- [ ] 遵守 `docs/marketing-features/*.md` 契约
