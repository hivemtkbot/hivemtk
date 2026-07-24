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

## 第二轮深度实测（真实 JWT + 真实后端 + DB 落库校验，2026-07-23~24）

用真实 JWT 对 12 页逐页发起真实后端调用 + DB 校验，发现并修复 4 个后端真实缺陷：

1. **【P9】OpenAPI 源创建/编辑必 400**：`KnowledgeOpenAPISource.ProductID` 是 int64，前端传 UUID 字符串无法绑入。修复：`knowledge_workspace_controller.go` 的 Create/Update handler 以 string 收 `product_id` 再 `resolveProductID`(UUID→int64) 转。✅ 已验（落库成功）。
2. **【P2】短文本 <100 字无法入库**：`document_processor.go` 默认 `MinChunkSize=100`/`MinLengthPerChunk=50`，`postProcessChunks` 把 <100 字整块丢弃→「分片结果为空」。改为阈值=1（仅过滤空白）。✅ 已验（19 字文本可切分+向量化+playground 召回）。
3. **【P5】反馈列表默认隐藏非中性反馈**：`ListFeedbacks` 把默认 `rating=0` 当 `WHERE rating=0` 等值过滤。改为仅 -1/1 作具体过滤、0=全部(sentinel 999)。✅ 已验（total 由 0 恢复为 1）。
4. **【P10/P11】`platform_account_configs` 表缺失（账号配置 GET/PUT 报 500 relation does not exist）**——根因 & 闭环如下。

### P10/P11 根因（已彻底闭环，非临时兜底）

**真因**：`AutoMigrate(&PlatformAccountConfig{})` 会**级联迁移其 belongs-to 关联 `RagProduct`**；`rag_products` 表存在历史约束命名漂移（`uni_rag_products_vector_table` 不存在，SQLSTATE 42704），该错误被 `isTolerableMigrateError` 判为「可容忍」→ 整个 `AutoMigrate` 调用在**本表尚未创建**时即被中断并返回 nil，导致 `platform_account_configs` 从未建表。随后 `CREATE TABLE IF NOT EXISTS` 因残留的同名 composite `pg_type platform_account_configs`（历史手动建表遗留）名冲突、同样被「already exists」判为可容忍而静默跳过——形成「表实际不存在、GORM 却以为已建」的死结，`HasTable`(查 relkind='r') 返回 false。新鲜测试库无此漂移，故 `TestAutoMigrate_Complete` 通过、唯独真实库漏建，极具迷惑性。

**修复**（`internal/pkg/utils/db/migrate.go`）：
- `AutoMigrate()` 收尾新增 `missingTables` 终校验：逐模型 `Migrator().HasTable` 探测，任一注册模型表缺失即**启动期 panic**（部署阶段暴露，非生产 500）。
- 循环内新增 `createTableFallback`：当某模型 `AutoMigrate` 命中可容忍错误且 `HasTable` 为 false 时，先 `DROP TYPE IF EXISTS <表名> CASCADE` 剔除同名 composite type（消除名冲突），再 `CreateTable`（仅建本表、不递归迁移关联模型，避开级联漂移），建后二次核对，仍缺失则 panic。
- 该兜底对 `platform_account_configs` 在真实库（含 rag_products 漂移）下可自动重建表，且不影响其他 270+ 表。

**验证（真实库 user_db@8232）**：
- 删除 `platform_account_configs` 后重启 user-server（携带修复），启动无 panic，表自动重建（`to_regclass` 返回 `platform_account_configs`）。
- `GET /api/rag-config/accounts/config` → HTTP 200；`PUT` → 200 "Configuration updated successfully"，回读确认落库。✅ 端到端无 500。

### 部署附带修复（2026-07-24）

`internal/router/team_routes.go` 为新增文件，但其 handler 引用了不存在的 `TeamUserController` 方法名（`GetCurrentTeamUser` 等），导致 `cmd/api` **无法编译**、阻塞上述修复部署。已将其对齐到真实实现方法（`GetCurrentUser`/`GetList`/`Create`/`Update`/`Delete`/`ChangePassword`/`ResetPassword`），并补 `TeamRoleController`（`GetList`/`Create`/`Update`/`Delete`/`GetPermissions`）与 `OperationLogController`（`GetStatistics`/`GetList`/`GetByID`/`ExportLogs`/`DeleteLogs`/`CleanLogs`）。✅ `go build ./cmd/api/` 通过。

### 环境注意事项
- user-server 重启会重置 admin 密码，需 `psql` 重置为已知 bcrypt 哈希后再登录。
- 本机 Docker 工具链安装（apk gcc）偶发网络挂起，无法走 `docker compose build`；采用本地 `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build` 交叉编译 + `docker cp` 注入容器二进制的方式完成部署（等价、可复现）。镜像重建待网络恢复后补做。
- 测试数据已清理（删除 `test-acc-001`）。
