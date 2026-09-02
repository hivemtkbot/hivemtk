package router

import (
	contentctrl "hivemtk-user/internal/content/controller"
	contentservice "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupDomainPoolRoutes 域名池管理路由
//
// 权限分级（2026-08-31 P0-23 四轮加固）：
//   - 域名池是公司级基础设施，staff 不能新增/修改/删除域名（钓鱼+SSRF 风险）。
//   - 域名探测 check-domain / check-all 涉及外部网络请求，也收敛为 admin。
//   - 只读查询（list / :id）保留任意登录。
func setupDomainPoolRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	database := gormDB
	domainPoolRepo := repository.NewDomainPoolRepository(database)
	domainPoolSvc := service.NewDomainPoolService(database)
	healthSvc := service.NewDomainHealthService(database, domainPoolRepo)
	domainPoolCtrl := controller.NewDomainPoolController(domainPoolSvc, healthSvc)
	auth.GET("/domainpool/list", domainPoolCtrl.List)
	auth.GET("/domainpool/:id", domainPoolCtrl.GetByID)

	admin := auth.Group("/domainpool", middleware.AdminAuthMiddleware())
	{
		admin.POST("", domainPoolCtrl.Create)
		admin.PUT("/:id", domainPoolCtrl.Update)
		admin.DELETE("/:id", domainPoolCtrl.Delete)
		admin.POST("/check-domain", domainPoolCtrl.CheckDomain)
		admin.POST("/check-all", domainPoolCtrl.CheckAllDomains)
		admin.POST("/create", domainPoolCtrl.Create)
		admin.POST("/checkall", domainPoolCtrl.CheckAllDomains)
		admin.PUT("/update", domainPoolCtrl.Update)
	}
}

// setupMaterialRoutes 素材管理路由
//
// P0-25 权限分级（2026-08-31 六轮加固）：
//   - 读（List/Get/Stats/Categories）+ 上传：任意登录用户（销售日场上架素材）
//   - DELETE 素材 + 分类 CRUD：admin only（防 staff 误删/恶意删除生产素材库）
func setupMaterialRoutes(auth *gin.RouterGroup) {
	materialCtrl := contentctrl.NewMaterialController(contentservice.NewMaterialService())
	// 读 + 上传：任意登录
	auth.GET("/material/list", materialCtrl.GetMaterialList)
	auth.POST("/material", materialCtrl.UploadMaterial)
	auth.POST("/material/upload", materialCtrl.UploadMaterial)
	auth.PUT("/material/:id/usage", materialCtrl.UpdateMaterialUsage)
	auth.POST("/material/:id/usage", materialCtrl.UpdateMaterialUsage)
	auth.GET("/material/stats", materialCtrl.GetMaterialStats)
	auth.GET("/material/categories", materialCtrl.GetMaterialCategories)
	auth.GET("/material/categories/:id", materialCtrl.GetMaterialCategoryByID)
	auth.GET("/material/selector", materialCtrl.GetMaterialSelector)
	auth.GET("/material/:id", materialCtrl.GetMaterialByID)

	// 写（DELETE material + categories CRUD）：admin only
	materialAdmin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		materialAdmin.DELETE("/material/:id", materialCtrl.DeleteMaterial)
		materialAdmin.POST("/material/categories", materialCtrl.CreateMaterialCategory)
		materialAdmin.PUT("/material/categories/:id", materialCtrl.UpdateMaterialCategory)
		materialAdmin.DELETE("/material/categories/:id", materialCtrl.DeleteMaterialCategory)
	}
}

// setupClueRoutes 线索管理路由
//
// 权限分级（2026-08-31 P0-24 四轮加固）：
//   - 线索删除（DELETE /clue/:id /clues/delete/:id）admin only（防 staff 误删客户线索）。
//   - 线索导入（POST /clue/import /clues/import）涉及批量写入，也收敛为 admin。
//   - 评分/打标（score / engagement）保留任意登录（销售日常操作需要）。
func setupClueRoutes(auth *gin.RouterGroup) {
	clueCtrl := controller.NewClueController()
	auth.GET("/clue/list", clueCtrl.GetClueList)
	auth.GET("/clue/statistics", clueCtrl.GetClueStatistics)
	auth.GET("/clues/list", clueCtrl.GetClueList)
	auth.GET("/clues/statistics", clueCtrl.GetClueStatistics)
	auth.GET("/clues/type", clueCtrl.GetClueTypes)

	clueAdmin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		clueAdmin.DELETE("/clue/:id", clueCtrl.DeleteClue)
		clueAdmin.POST("/clue/import", clueCtrl.ImportClues)
		clueAdmin.DELETE("/clues/delete/:id", clueCtrl.DeleteClue)
		clueAdmin.POST("/clues/import", clueCtrl.ImportClues)
	}

	clueScoreCtrl := controller.NewClueScoreController()
	auth.POST("/clue/score", clueScoreCtrl.ScoreClue)
	auth.POST("/clue/score-all", clueScoreCtrl.ScoreAll)
	auth.GET("/clue/score/:clue_id", clueScoreCtrl.GetByClueID)
	auth.GET("/clue/score/list", clueScoreCtrl.ListByGrade)
	auth.POST("/clue/engagement", clueScoreCtrl.RecordEngagement)
}

// setupLeadMiningRoutes 线索发掘路由
//
// 权限分级（2026-08-18 三轮发现）：SaveConfig admin only
// 防 staff 误开关 lead-mining → 全公司线索流程停摆 / 误启用烧 token。
func setupLeadMiningRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewLeadMiningController()
	auth.GET("/lead-mining/config", ctrl.GetConfig)
	auth.GET("/lead-mining/status", ctrl.GetStatus)
	admin := auth.Group("/lead-mining", middleware.AdminAuthMiddleware())
	admin.POST("/config", ctrl.SaveConfig)
}

// setupCustomerRFMRoutes 客户 RFM 路由
//
// 权限分级（2026-08-18 三轮发现）：compute-all admin only（资源密集）
// compute 单个仍允许 staff（按需单点计算）。
func setupCustomerRFMRoutes(auth *gin.RouterGroup) {
	rfmCtrl := controller.NewCustomerRFMController()
	auth.POST("/customer-rfm/compute", rfmCtrl.ComputeForCustomer)
	auth.GET("/customer-rfm/:customer_id", rfmCtrl.GetByCustomerID)
	auth.GET("/customer-rfm/list", rfmCtrl.ListBySegment)
	auth.GET("/customer-rfm/distribution", rfmCtrl.Distribution)
	admin := auth.Group("/customer-rfm", middleware.AdminAuthMiddleware())
	admin.POST("/compute-all", rfmCtrl.ComputeAll)
}

// setupRecoveryQueueRoutes 流失挽回队列路由
//
// P0-24 权限分级（2026-08-31 四轮加固）：
//   - 读（ListByStage/Distribution/ListReadyForAttempt）：任意登录用户
//   - 写（Enqueue/MarkAttempt/MarkRecovered/Cancel）：admin only
// 防 staff 恶意 enqueue 大量客户触发批量挽回短信轰炸。
func setupRecoveryQueueRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewRecoveryQueueController()
	auth.GET("/recovery-queue/list", ctrl.ListByStage)
	auth.GET("/recovery-queue/distribution", ctrl.Distribution)
	auth.GET("/recovery-queue/ready", ctrl.ListReadyForAttempt)
	admin := auth.Group("/recovery-queue", middleware.AdminAuthMiddleware())
	{
		admin.POST("/enqueue", ctrl.Enqueue)
		admin.POST("/:id/attempt", ctrl.MarkAttempt)
		admin.POST("/:id/recovered", ctrl.MarkRecovered)
		admin.POST("/:id/cancel", ctrl.Cancel)
	}
}

