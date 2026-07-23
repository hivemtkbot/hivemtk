# 资产包（Asset Bundle）三端业务闭环说明

> 适用范围：hivemtk 用户端（开源客服系统）资产包体系。
> 三端：平台端 `platform-server`、商户后端 `user-server`、商户前端 `user-web`。
> 目标：开发者生产 → 平台集市分发 → 商户消费使用 → 使用数据回传平台，形成**可实际运行的业务闭环**。

---

## 1. 三端职责

| 端 | 代码位置 | 角色 | 关键职责 |
| --- | --- | --- | --- |
| 平台端 | `hivemtk-platform/platform-server` | 资产包商城 / 中台（唯一数据源 + 分发中心） | 存储、审核上架、购买分发、贡献者账户、使用统计 |
| 开发者端 | （ISV / 商户内 Playground） | 生产者 | 在 Playground 调教 messages → 提交平台审核上架 |
| 商户端 | `hivemtk/user-server` + `hivemtk/user-web` | 消费者 / 运行者 | 从平台浏览、购买/试用、拉取到本地、运行织入自身 RAG，**不生产只消费** |

数据流铁律：`user-web → user-server → platform-server`，**禁止 user-web 直连平台**；user-server 访问平台统一走 `config.PlatformCfg.APIURL`（源 `config/platform.yaml` 的 `api_url`，`PLATFORM_API_HOST` 环境变量最高优先级覆盖）。

---

## 2. 端到端业务链（8 步闭环）

```
[开发者 Playground]
   │ 调教 messages / 配置行业·范围·价格·署名
   ▼
① 发布到本地 asset_bundles（状态 published）
   │ 前端: Playground.vue "上架到官方蜂巢商城"
   ▼
② 提交平台审核（user-server SubmitToPlatform）
   │ POST /api/asset-bundle/:id/submit-platform
   │ user-server: AssetBundleService.SubmitToPlatform
   │   → platform contributor-api: CreateAsset + SubmitAudit
   ▼
[平台端]
③ 平台存资产 + 进入待审核（status=pending）
   │ model.Asset / AssetVersion / Contributor
   ▼
④ 平台运营审核通过（admin 审核）
   │ POST /api/asset-market/approve
   │ 资产 status → approved，进入公开资产市场
   ▼
[商户 user-web 资产市场]
⑤ 商户浏览市场 → 试用/购买（记录成本，不实际接入支付）
   │ Market.vue → POST /api/v1/asset-market/purchase
   │ user-server: AssetMarketController.Purchase
   │   → platform merchant-api: POST /merchant-api/asset-market/purchase
   │   → Purchase.Amount=a.Price（免费试用 IsFreeTrial=true，仅记录成本与分润）
   ▼
⑥ 商户"同步"拉取到本地库存
   │ MyAssets.vue 同步 → POST /api/v1/asset-market/sync
   │ user-server: AssetMarketService.SyncFromPlatform
   │   → platform merchant-api: POST /merchant-api/asset-market/sync
   │   → 落地 local_assets + local_asset_datas
   ▼
[商户运行时]
⑦ 商户客服系统实际使用资产（Loader 加载）
   │ LocalAssetService.LoadByType 被运行时调用
   │   → local_assets.use_count += 1（本地使用计数）
   │   → 后台 best-effort 回传平台（见⑧）
   ▼
⑧ 使用次数回传平台（闭环遥测）—— 本次补全
   │ 触发方式 1（自动）：LoadByType 加载 purchased 资产时 go reportUsageAsync
   │ 触发方式 2（手动）：MyAssets.vue "上报使用" 按钮
   │ user-server: LocalAssetService.ReportUsage
   │   → platform merchant-api: POST /merchant-api/asset-market/report-usage
   │   → Purchase.UsageCount += delta
   ▼
[平台端统计]
   贡献者可在平台侧看到其资产被商户使用的累计次数（Purchase.UsageCount）
```

---

## 3. 功能链 / 代码落点

### 3.1 平台端 platform-server
| 能力 | 路由 / 方法 | 关键文件 |
| --- | --- | --- |
| 贡献者注册/登录 | `/contributor-api/v1/auth/register\|login` | `internal/controller/contributor_controller.go` |
| 创建资产 | `POST /contributor-api/v1/assets` | `internal/controller/asset_market_controller.go` `CreateAsset` |
| 提交审核 | `POST /contributor-api/v1/assets/:id/submit` | `SubmitAudit` |
| 市场列表 | `GET /merchant-api/asset-market/list` | `ListAssets` |
| 资产详情 | `GET /merchant-api/asset-market/detail/:asset_id` | `GetAsset` |
| 购买/试用 | `POST /merchant-api/asset-market/purchase` | `PurchaseByBody` |
| 拉取数据 | `POST /merchant-api/asset-market/sync` | `SyncPullByBody` |
| **使用上报**（新增） | `POST /merchant-api/asset-market/report-usage` | `ReportUsage`（controller/service/repository） |
| 运营审核 | `POST /api/asset-market/approve` | `AdminApprove` |
| 已购列表 | `GET /merchant-api/asset-market/my-purchases` | `MyPurchases`（含 `usage_count`） |

数据模型：`model.Asset`、`model.AssetVersion`、`model.Purchase`（`UsageCount int64`，新增）、`model.Contributor`。

### 3.2 商户后端 user-server
| 能力 | 入口 | 关键文件 |
| --- | --- | --- |
| 上架提交平台 | `POST /api/asset-bundle/:id/submit-platform` | `internal/service/asset_bundle_submit.go` `SubmitToPlatform` |
| 平台客户端（签名调用） | — | `internal/platform/contributor_client.go`、`asset_market_client.go`、`asset_market_adapter.go` |
| 市场列表/详情 | `GET /api/asset-market/list`、`/detail/:asset_id` | `internal/controller/asset_market.go` `ListMarket`/`MarketDetail` |
| 购买 | `POST /api/asset-market/purchase` | `Purchase` |
| 同步拉取 | `POST /api/asset-market/sync` | `Sync` → `LocalAssetService.SyncFromPlatform` |
| **使用上报**（新增） | `POST /api/asset-market/report-usage` | `ReportUsage` → `LocalAssetService.ReportUsage` |
| 本地资产增删改查/启停/同步日志 | `/api/v1/local-assets/*` | `internal/service/local_asset_service.go` |
| 运行时加载并计数（新增） | — | `LocalAssetService.LoadByType`（累加 `use_count` + 自动上报） |

跨端接口契约：`internal/repository/platform_api_client.go` 的 `PlatformAPIClient`
（`ListAssets` / `GetAssetDetail` / `Purchase` / `PullData` / `MyPurchases` / `ReportUsage`）。

本地模型：`model.LocalAsset`（新增 `UseCount`、`ReportedUseCount`）、`model.LocalAssetData`、`model.LocalAssetSyncLog`。
仓储：`internal/repository/local_asset_repo.go`（`IncrementUseCount`、`SetReportedUseCount`）。

### 3.3 商户前端 user-web
| 能力 | 页面 / API |
| --- | --- |
| 浏览市场 + 试用购买 | `src/views/assetMarket/Market.vue` + `src/api/assetMarket.js` `listMarketAssets`/`purchaseAsset` |
| 我的资产（列表/同步/详情/启停/删除） | `src/views/assetMarket/MyAssets.vue` |
| **使用次数列 + 上报使用按钮**（新增） | `MyAssets.vue` + `reportUsage()` |
| 同步日志 | `src/views/assetMarket/SyncLog.vue` |
| 开发者 Playground（调教 + 上架） | `src/views/assetBundle/Playground.vue` + `src/api/assetBundle.js` `submitToPlatform` |

---

## 4. 闭环要点说明

1. **购买即记录成本、不实际接入支付**：`platform-server` 的 `Purchase.Amount = a.Price`，按 `PlatformShare / ContributorShare` 比例登记分润，`IsFreeTrial = true`。前端为"免费试用"流程，平台端确认即可，无真实支付网关。
2. **本地使用计数**：运行时 `LoadByType` 每加载一次 purchased 资产，`local_assets.use_count += 1`（开发者自有 `asset_bundles.use_count` 由 `WeaveForRequest` 单独维护，互不影响）。
3. **增量回传、避免重复计数**：
   - 本地用 `ReportedUseCount` 记录已上报基准；上报增量 `delta = UseCount - ReportedUseCount`。
   - 自动上报用 `sync.Map`（`reportingInFlight`）做同资产并发去重，上报成功后才推进 `ReportedUseCount`。
   - 平台侧 `UPDATE purchases SET usage_count = usage_count + ?` 按增量累加，天然幂等。
4. **best-effort、不阻塞主流程**：运行时自动上报走 goroutine + 5s 超时，失败仅记日志不影响客服响应；商户也可在"我的资产"手动点击"上报使用"强制回传。

---

## 5. 如何验证闭环

### 5.1 编译校验（三端均通过）
```bash
# 平台端
cd hivemtk-platform/platform-server && GOCACHE=/tmp/gs_plat go build ./...
# 商户后端
cd hivemtk/user-server && GOCACHE=/tmp/gs_user go build ./...
# 商户前端
cd hivemtk/user-web && PATH=/usr/local/n/versions/node/22.12.0/bin:$PATH npm run build
```
> 注：user-server 构建缓存易损坏，若遇随机 `undefined`/EOF 报错，先 `go clean -cache` 再编。

### 5.2 端到端最小验证（curl 思路）
1. 开发者 `Playground.vue`：调教话术 → "上架到官方蜂巢商城"（触发②）。
2. 平台运营 `POST /api/asset-market/approve`（触发④）。
3. 商户 `Market.vue`：点击"免费试用"（触发⑤）。
4. 商户 `MyAssets.vue`：点击"同步"（触发⑥，资产进入本地库存）。
5. 触发商户客服运行时加载该资产（⑦，`use_count` 递增），或点击"上报使用"（⑧）。
6. 平台侧查询 `GET /merchant-api/asset-market/my-purchases`，可见 `usage_count > 0`，闭环完成。

---

## 6. 已知边界
- 本地使用计数仅在 `LoadByType` 命中 purchased 资产时累加；若某资产类型未被运行时加载，则不会触发自动上报（可手动"上报使用"）。
- 平台侧使用统计以 `Purchase` 维度聚合（同一商户对同一资产一次购买对应一条购买记录）。
- 支付未接入：购买仅作成本/分润登记，符合企业内网非产品销售定位。
