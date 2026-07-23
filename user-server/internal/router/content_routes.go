package router

import (
	contentctrl "marketing/internal/content/controller"
	contentservice "marketing/internal/content/service"
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupDomainPoolRoutes 域名池管理路由
func setupDomainPoolRoutes(auth *gin.RouterGroup) {
	database := db.GetDB()
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
	// 前端兼容路由
	auth.DELETE("/domainpool/delete/:id", domainPoolCtrl.Delete)
	auth.POST("/domainpool/check/:id", func(c *gin.Context) {
		c.Request.URL.Path = "/api/domainpool/check-domain"
		domainPoolCtrl.CheckDomain(c)
	})
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
	// 前端兼容路由
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
	// 前端兼容路由
	auth.GET("/clues/list", clueCtrl.GetClueList)
	auth.DELETE("/clues/delete/:id", clueCtrl.DeleteClue)
	auth.GET("/clues/statistics", clueCtrl.GetClueStatistics)
	auth.POST("/clues/import", clueCtrl.ImportClues)
	auth.GET("/clues/type", clueCtrl.GetClueTypes)

	// H 域 P1: 线索评分 + 互动事件
	clueScoreCtrl := controller.NewClueScoreController()
	auth.POST("/clue/score", clueScoreCtrl.ScoreClue)
	auth.POST("/clue/score-all", clueScoreCtrl.ScoreAll)
	auth.GET("/clue/score/:clue_id", clueScoreCtrl.GetByClueID)
	auth.GET("/clue/score/list", clueScoreCtrl.ListByGrade)
	auth.POST("/clue/engagement", clueScoreCtrl.RecordEngagement)
}

// setupCustomerRFMRoutes 客户 RFM 路由
func setupCustomerRFMRoutes(auth *gin.RouterGroup) {
	rfmCtrl := controller.NewCustomerRFMController()
	auth.POST("/customer-rfm/compute", rfmCtrl.ComputeForCustomer)
	auth.POST("/customer-rfm/compute-all", rfmCtrl.ComputeAll)
	auth.GET("/customer-rfm/:customer_id", rfmCtrl.GetByCustomerID)
	auth.GET("/customer-rfm/list", rfmCtrl.ListBySegment)
	auth.GET("/customer-rfm/distribution", rfmCtrl.Distribution)
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
