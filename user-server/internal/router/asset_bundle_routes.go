package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
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
func setupAssetBundleRoutes(auth *gin.RouterGroup) {
	// 装配：repo → service → controller
	assetBundleRepo := repository.NewAssetBundleRepository(db.GetDB())
	versionLogRepo := repository.NewAssetBundleVersionLogRepository(db.GetDB())
	assetBundleSvc := service.NewAssetBundleService(assetBundleRepo, versionLogRepo)
	ctrl := controller.NewAssetBundleController(assetBundleSvc)

	// 兼容 /api/v1 与 /api 两种前缀
	for _, g := range []*gin.RouterGroup{
		auth.Group("/v1/asset-bundle"),
		auth.Group("/asset-bundle"),
	} {
		ctrl.Register(g)
	}
}
