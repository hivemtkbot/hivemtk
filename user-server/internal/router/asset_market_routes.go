package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/platform"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupAssetMarketRoutes 资产市场 + 本地资产
func setupAssetMarketRoutes(auth *gin.RouterGroup) {
	gdb := db.GetDB()
	ar := repository.NewLocalAssetRepository(gdb)
	dr := repository.NewLocalAssetDataRepository(gdb)
	sr := repository.NewLocalAssetSyncLogRepository(gdb)
	pc := platform.NewPlatformAPIClient()
	h := controller.NewAssetMarketController(
		service.NewLocalAssetService(ar, dr, sr, pc, gdb),
		service.NewAssetMarketService(pc),
	)

	// 兼容 /api/v1 与 /api
	groups := []*gin.RouterGroup{
		auth.Group("/v1/asset-market"),
		auth.Group("/asset-market"),
	}
	for _, market := range groups {
		// 前端实际调用路径（兼容）
		market.GET("", h.ListMarket)            // GET /api/v1/asset-market  → listAssets
		market.GET("/:asset_id", h.MarketDetail) // GET /api/v1/asset-market/:id → getAssetDetail
		market.GET("/my", h.MyPurchases)         // GET /api/v1/asset-market/my → getMyAssets
		// 历史/备用路径
		market.GET("/list", h.ListMarket)
		market.GET("/detail/:asset_id", h.MarketDetail)
		market.POST("/purchase", h.Purchase)
		market.POST("/sync", h.Sync)
		market.POST("/report-usage", h.ReportUsage)
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
