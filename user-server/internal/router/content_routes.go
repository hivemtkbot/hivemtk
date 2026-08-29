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
func setupDomainPoolRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	database := gormDB
	domainPoolRepo := repository.NewDomainPoolRepository(database)
	domainPoolSvc := service.NewDomainPoolService(database)
	healthSvc := service.NewDomainHealthService(database, domainPoolRepo)
	domainPoolCtrl := controller.NewDomainPoolController(domainPoolSvc, healthSvc)
	auth.GET("/domainpool/list", domainPoolCtrl.List)
	auth.POST("/domainpool", domainPoolCtrl.Create)
	auth.PUT("/domainpool/:id", domainPoolCtrl.Update)
	auth.DELETE("/domainpool/:id", domainPoolCtrl.Delete)
	auth.GET("/domainpool/:id", domainPoolCtrl.GetByID)
	auth.POST("/domainpool/check-domain", domainPoolCtrl.CheckDomain)
	auth.POST("/domainpool/check-all", domainPoolCtrl.CheckAllDomains)
	// R40: 旧蛇形别名 /domainpool/delete|check 已删除（前端全量走 /api/domain-pool/*，ZOMBIE_API_TRIAGE ③ 处置）
	// R40 ZOMBIE_API_TRIAGE ③ 处置：删除旧蛇形别名路由
	// （/domainpool/delete|check|create|update|checkall —— 前端全量走 /api/domain-pool/*，grep 全仓无脚本依赖）
	auth.POST("/domainpool/create", domainPoolCtrl.Create)
	auth.PUT("/domainpool/update", func(c *gin.Context) {
		domainPoolCtrl.Update(c)
	})
	auth.POST("/domainpool/checkall", domainPoolCtrl.CheckAllDomains)
}

// setupMaterialRoutes 素材管理路由
func setupMaterialRoutes(auth *gin.RouterGroup) {
	materialCtrl := contentctrl.NewMaterialController(contentservice.NewMaterialService())
	auth.GET("/material/list", materialCtrl.GetMaterialList)
	auth.POST("/material", materialCtrl.UploadMaterial)
	auth.PUT("/material/:id/usage", materialCtrl.UpdateMaterialUsage)
	auth.DELETE("/material/:id", materialCtrl.DeleteMaterial)
	auth.GET("/material/stats", materialCtrl.GetMaterialStats)
	auth.GET("/material/categories", materialCtrl.GetMaterialCategories)
	auth.POST("/material/categories", materialCtrl.CreateMaterialCategory)
	auth.PUT("/material/categories/:id", materialCtrl.UpdateMaterialCategory)
	auth.DELETE("/material/categories/:id", materialCtrl.DeleteMaterialCategory)
	auth.GET("/material/categories/:id", materialCtrl.GetMaterialCategoryByID)
	auth.GET("/material/selector", materialCtrl.GetMaterialSelector)
	auth.GET("/material/:id", materialCtrl.GetMaterialByID)
	auth.POST("/material/upload", materialCtrl.UploadMaterial)
	auth.POST("/material/:id/usage", materialCtrl.UpdateMaterialUsage)
}

// setupClueRoutes 线索管理路由
func setupClueRoutes(auth *gin.RouterGroup) {
	clueCtrl := controller.NewClueController()
	auth.GET("/clue/list", clueCtrl.GetClueList)
	auth.DELETE("/clue/:id", clueCtrl.DeleteClue)
	auth.GET("/clue/statistics", clueCtrl.GetClueStatistics)
	auth.POST("/clue/import", clueCtrl.ImportClues)
	auth.GET("/clues/list", clueCtrl.GetClueList)
	auth.DELETE("/clues/delete/:id", clueCtrl.DeleteClue)
	auth.GET("/clues/statistics", clueCtrl.GetClueStatistics)
	auth.POST("/clues/import", clueCtrl.ImportClues)
	auth.GET("/clues/type", clueCtrl.GetClueTypes)

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
func setupRecoveryQueueRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewRecoveryQueueController()
	auth.POST("/recovery-queue/enqueue", ctrl.Enqueue)
	auth.POST("/recovery-queue/:id/attempt", ctrl.MarkAttempt)
	auth.POST("/recovery-queue/:id/recovered", ctrl.MarkRecovered)
	auth.POST("/recovery-queue/:id/cancel", ctrl.Cancel)
	auth.GET("/recovery-queue/list", ctrl.ListByStage)
	auth.GET("/recovery-queue/distribution", ctrl.Distribution)
	auth.GET("/recovery-queue/ready", ctrl.ListReadyForAttempt)
}

