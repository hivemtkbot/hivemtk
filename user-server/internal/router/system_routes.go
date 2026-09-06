package router

import (
	knowledgectrl "hivemtk-user/internal/aiagent/knowledge/controller"
	knowledgerepo "hivemtk-user/internal/aiagent/knowledge/repository"
	knowledgesvc "hivemtk-user/internal/aiagent/knowledge/service"
	"hivemtk-user/internal/aiagent/llm"
	rag_core "hivemtk-user/internal/aiagent/rag/core"
	rag_service "hivemtk-user/internal/aiagent/rag/service"
	"hivemtk-user/internal/aiagent/vector"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/etl"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/migration"
	"hivemtk-user/internal/migration/migrations"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/translation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupSystemRoutes(auth *gin.RouterGroup) {
	systemConfigCtrl := controller.NewSystemConfigController()
	auth.GET("/system/config", systemConfigCtrl.GetConfig)
	auth.POST("/system/config", systemConfigCtrl.SaveConfig)

	toolIntegrationCtrl := controller.NewToolIntegrationConfigController()
	auth.GET("/agent/tool-integrations", toolIntegrationCtrl.GetConfig)
	auth.PUT("/agent/tool-integrations", toolIntegrationCtrl.SaveConfig)

	agentSettingsCtrl := controller.NewAgentSettingsController()
	auth.GET("/agent/settings", agentSettingsCtrl.GetConfig)
	auth.PUT("/agent/settings", agentSettingsCtrl.SaveConfig)

	systemOpsCtrl := controller.NewSystemOpsController()
	auth.POST("/system/restart", systemOpsCtrl.RestartServer)
	auth.GET("/system/logs", systemOpsCtrl.GetSystemLogs)
	auth.GET("/system/stats", systemOpsCtrl.GetSystemStats)
	auth.GET("/stats/system", systemOpsCtrl.GetSystemStats)
	auth.GET("/system/backup", systemOpsCtrl.GetBackupList)
	auth.POST("/system/backup", systemOpsCtrl.CreateBackup)
	auth.POST("/system/restore", systemOpsCtrl.RestoreBackup)

	adminConfigCtrl := controller.NewAdminConfigController()
	auth.GET("/admin/config", adminConfigCtrl.GetAdminConfig)

	obsConfigCtrl := controller.NewObsConfigController()
	auth.GET("/obs/config", obsConfigCtrl.GetConfigList)
	auth.GET("/obs/config/:id", obsConfigCtrl.GetConfig)
	auth.GET("/obs/config/default", obsConfigCtrl.GetDefaultConfig)

	obsAdmin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		obsAdmin.POST("/obs/config", obsConfigCtrl.CreateConfig)
		obsAdmin.PUT("/obs/config/:id", obsConfigCtrl.UpdateConfig)
		obsAdmin.DELETE("/obs/config/:id", obsConfigCtrl.DeleteConfig)
		obsAdmin.POST("/obs/config/:id/test", obsConfigCtrl.TestConnection)
		obsAdmin.POST("/obs/config/:id/default", obsConfigCtrl.SetDefault)
	}
}

func setupRagRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	ragRepo := knowledgerepo.NewRagConfigRepository(gormDB)
	llmService := llm.NewLLMService()
	rageEngine := rag_core.NewRAGEngine(nil)
	ragCoreService := rag_service.NewRAGService(llmService, rageEngine)
	docProcessor := etl.NewDocumentProcessor(nil)
	remoteVectorizer := vector.NewRemoteVectorizer(1024)
	vectorStore := vector.NewInMemoryVectorStore(1024)
	vecProcessor := vector.NewVectorProcessor(remoteVectorizer, vectorStore)
	ragService := knowledgesvc.NewRagConfigService(ragRepo, ragCoreService, docProcessor, vecProcessor)
	ragConfigCtrl := knowledgectrl.NewRagConfigController(ragService)
	ragConfigCtrl.RegisterRoutes(auth)

	ragMetricsSvc := service.NewRagMetricsService(gormDB)
	ragHealthSvc := service.NewRagHealthService(gormDB, ragMetricsSvc)
	ragHealthCtrl := controller.NewRagHealthController(ragHealthSvc)
	ragHealthCtrl.RegisterRoutes(auth)

	ragRecallCtrl := controller.NewRagRecallMonitorController(service.NewRagRecallMonitorService(gormDB, 0, 0), ragMetricsSvc)
	ragRecallCtrl.RegisterRoutes(auth)
}

func setupKnowledgeBaseRoutes(auth *gin.RouterGroup) {
	knowledgeBaseCtrl := knowledgectrl.NewKnowledgeBaseController()
	knowledgeBaseCtrl.RegisterRoutes(auth)

	knowledgeWorkspaceCtrl := knowledgectrl.NewKnowledgeWorkspaceController(&openAPISourceAdapter{svc: service.NewOpenAPIService()})
	knowledgeWorkspaceCtrl.RegisterRoutes(auth)

	knowledgeWorkspaceCtrl.RegisterConnectors(auth, controller.NewKBConnectorController())

	knowledgeMerchantCtrl := knowledgectrl.NewKnowledgeMerchantController()
	knowledgeMerchantCtrl.RegisterRoutes(auth)
}

func setupBackupRoutes(auth *gin.RouterGroup) {
	backupCtrl := controller.NewBackupController()
	restoreCtrl := controller.NewRestoreController()
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	admin.GET("/backups", backupCtrl.GetBackupList)
	admin.GET("/backups/:id", backupCtrl.GetBackupByID)
	admin.GET("/restore/list", restoreCtrl.GetRestoreList)
	admin.GET("/restore/last", restoreCtrl.GetLastRestore)
	admin.POST("/backups", backupCtrl.CreateBackup)
	admin.DELETE("/backups/:id", backupCtrl.DeleteBackup)
	admin.POST("/restore", restoreCtrl.RestoreBackup)
}

func setupMigrationRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	registry := migration.NewMigrationRegistry()
	migrationSvc := migration.NewMigrationService(registry, gormDB, migrations.RegisterMigrations)
	migrationCtrl := controller.NewMigrationController(migrationSvc)
	auth.GET("/migration/task/:id", migrationCtrl.GetUpgradeTask)
	auth.GET("/migration/history", migrationCtrl.GetUpgradeHistory)
	auth.GET("/migration/records", migrationCtrl.GetMigrationRecords)
	auth.GET("/migration/current-version", migrationCtrl.GetCurrentVersion)
	auth.GET("/migration/available", migrationCtrl.GetAvailableUpgrades)

	admin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		admin.POST("/migration/task", migrationCtrl.CreateUpgradeTask)
		admin.POST("/migration/rollback", migrationCtrl.Rollback)
	}
}

func setupI18nRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	glossaryRepo := repository.NewGlossaryRepositoryWithDB(db)
	glossarySvc := translation.NewGlossaryService(glossaryRepo, nil)
	glossaryCtrl := controller.NewGlossaryController(glossarySvc)
	glossaryCtrl.RegisterRoutes(auth)

	statsRepo := repository.NewI18nStatsRepositoryWithDB(db)
	statsSvc := translation.NewI18nStatsService(statsRepo)
	statsCtrl := controller.NewI18nStatsController(statsSvc)
	statsCtrl.RegisterRoutes(auth)
}

func setupTuningRoutes(auth *gin.RouterGroup) {
	tuning := auth.Group("/admin/tuning")
	ctrl := controller.NewTuningController(service.NewTuningService())

	tuning.GET("/confidence/signals", ctrl.ListConfidenceSignals)
	tuning.GET("/confidence/signals/:id", ctrl.GetConfidenceSignal)
	tuning.GET("/confidence/signals/stats", ctrl.StatsConfidenceSignals)

	tuning.GET("/confidence/calibrations", ctrl.ListCalibrations)

	tuning.GET("/confidence/policies", ctrl.ListThresholdPolicies)
	tuning.PUT("/confidence/policies", ctrl.UpsertThresholdPolicy)

	tuning.GET("/humanize/scores", ctrl.ListHumanizeScores)
	tuning.GET("/humanize/scores/stats", ctrl.StatsHumanizeScores)

	tuning.GET("/humanize/baselines", ctrl.ListChampionBaselines)

	tuning.GET("/feedback/events", ctrl.ListFeedbackEvents)
	tuning.GET("/feedback/events/stats", ctrl.StatsFeedbackEvents)

	tuning.GET("/feedback/dialogues", ctrl.ListChampionDialogues)

	tuning.GET("/prompt/candidates", ctrl.ListPromptCandidates)
	tuning.PUT("/prompt/candidates/:id/status", ctrl.UpdatePromptCandidateStatus)

	tuning.GET("/bandit/arms", ctrl.ListBanditArms)

	tuning.GET("/humanize/low-quality", ctrl.ListLowQualitySamples)
}

func setupAIToolConfigRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	toolRepo := repository.NewAIToolConfigRepository(db)
	bindingRepo := repository.NewAIToolAccountBindingRepository(db)
	svc := service.NewAIToolConfigService(toolRepo, bindingRepo)
	ctrl := controller.NewAIToolConfigController(svc)

	g := auth.Group("/ai-tools")
	{
		g.GET("", ctrl.ListTools)
		g.GET("/:name", ctrl.GetTool)
		g.GET("/:name/accounts", ctrl.GetToolAccounts)
	}

	admin := auth.Group("/ai-tools", middleware.AdminAuthMiddleware())
	{
		admin.PUT("/:name/status", ctrl.UpdateToolStatus)
		admin.POST("/batch-status", ctrl.BatchUpdateStatus)
		admin.POST("/:name/accounts", ctrl.BindAccount)
		admin.DELETE("/:name/accounts/:account_type/:account_id", ctrl.UnbindAccount)
	}
}
