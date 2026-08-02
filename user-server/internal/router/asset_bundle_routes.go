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
