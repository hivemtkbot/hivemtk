package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAssetBundleRoutes 资产包（AssetBundle）路由
//
// 方向9：资产包模式 - OpenAI messages 资产包 CRUD + Weave 织布算法
// 方向 D1：资产包热插拔/动态启用禁用（立即生效，无需重启）
// 文档依据：docs/企业级架构优化/资产包模式.md
//
// 路由：
//
//	POST   /api/asset-bundle              创建
//	PUT    /api/asset-bundle/:id          更新
//	GET    /api/asset-bundle/:id          查询
//	GET    /api/asset-bundle/by-aid/:aid  按 AssetID 查询
//	POST   /api/asset-bundle/list         分页
//	POST   /api/asset-bundle/:id/publish  启用
//	POST   /api/asset-bundle/:id/archive  归档
//	DELETE /api/asset-bundle/:id          软删
//	POST   /api/asset-bundle/weave        Weave 织布
//	POST   /api/asset-bundle/merchant-save 商户表单保存
//	POST   /api/asset-bundle/merchant-parse/:aid 商户表单解析
//	POST   /api/asset-bundle/:id/enable   热启用（D1，立即生效）
//	POST   /api/asset-bundle/:id/disable  热禁用（D1，立即生效）
//	POST   /api/asset-bundle/enabled/list 查询已热启用的资产包列表
func setupAssetBundleRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	// 装配：repo → service → controller
	assetBundleRepo := repository.NewAssetBundleRepository(gormDB)
	versionLogRepo := repository.NewAssetBundleVersionLogRepository(gormDB)
	assetBundleSvc := service.NewAssetBundleService(assetBundleRepo, versionLogRepo)
	// 注入平台同步资产加载器：资产包解析器在 asset_bundles 未命中时回退解析商户从
	// 平台同步下来的本地资产（local_assets），闭合「平台下发资产包→运行时消费」闭环。
	localAssetRepo := repository.NewLocalAssetRepository(gormDB)
	localAssetDataRepo := repository.NewLocalAssetDataRepository(gormDB)
	localAssetSyncLogRepo := repository.NewLocalAssetSyncLogRepository(gormDB)
	localAssetSvc := service.NewLocalAssetService(localAssetRepo, localAssetDataRepo, localAssetSyncLogRepo, platform.NewPlatformAPIClient(), gormDB)
	assetBundleSvc.SetLocalLoader(localAssetSvc)
	// 注入全局资产包解析器：让 SalesEngine 在执行智能体时按 asset_bundle_id
	// 织入资产包人设/话术（渠道→智能体→资产包 三层接线点）
	service.SetAssetBundleResolver(assetBundleSvc)
	ctrl := controller.NewAssetBundleController(assetBundleSvc)

	// 兼容 /api/v1 与 /api 两种前缀。
	// 注意：controller.AssetBundleController.Register 内部已对传入 group 追加
	// "/asset-bundle" 子组，因此此处只传前缀组（/v1 或空），避免路径重复成
	// /api/v1/asset-bundle/asset-bundle/list 导致 404。
	for _, g := range []*gin.RouterGroup{
		auth.Group("/v1"),
		auth,
	} {
		ctrl.Register(g)
	}
}

// ============================================================================
// 以下内容合并自 asset_market_routes.go（P1-2 router 文件数收敛）
// ============================================================================

// setupAssetMarketRoutes 资产市场 + 本地资产
func setupAssetMarketRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	gdb := gormDB
	ar := repository.NewLocalAssetRepository(gdb)
	dr := repository.NewLocalAssetDataRepository(gdb)
	sr := repository.NewLocalAssetSyncLogRepository(gdb)
	pc := platform.NewPlatformAPIClient()
	h := controller.NewAssetMarketController(
		service.NewLocalAssetService(ar, dr, sr, pc, gdb),
		service.NewAssetMarketService(pc),
		service.NewAIAgentService(),
	)

	// 兼容 /api/v1 与 /api
	groups := []*gin.RouterGroup{
		auth.Group("/v1/asset-market"),
		auth.Group("/asset-market"),
	}
	for _, market := range groups {
		market.GET("/list", h.ListMarket)
		market.GET("/detail/:asset_id", h.MarketDetail)
		market.POST("/purchase", h.Purchase)
		market.POST("/sync", h.Sync)
		market.POST("/report-usage", h.ReportUsage)
		market.GET("/my-purchases", h.MyPurchases)
		market.POST("/bind", h.BindToAgent)
	}

	localGroups := []*gin.RouterGroup{
		auth.Group("/v1/local-assets"),
		auth.Group("/local-assets"),
	}
	for _, local := range localGroups {
		local.GET("", h.ListLocal)
		local.GET("/sync-log", h.SyncLog)
		local.GET("/:id", h.GetLocal)
		local.POST("", h.CreateLocal)
		local.PUT("/:id", h.UpdateLocal)
		local.DELETE("/:id", h.DeleteLocal)
		local.PUT("/:id/toggle-active", h.ToggleActive)
	}
}
