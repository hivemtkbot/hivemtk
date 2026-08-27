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

// setupSystemRoutes 系统管理路由
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

	// 最高标准审计 P1-6 修复：OBS 凭据配置（AccessKey/SecretKey + TestConnection
	// SSRF 探测面）写操作收敛为 admin only；只读查询保留任意登录用户。
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

// setupRagRoutes RAG 知识库管理路由
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

// setupKnowledgeBaseRoutes 知识库管理路由(基础 + 工作台 + 商户视角增强)
func setupKnowledgeBaseRoutes(auth *gin.RouterGroup) {
	knowledgeBaseCtrl := knowledgectrl.NewKnowledgeBaseController()
	knowledgeBaseCtrl.RegisterRoutes(auth)

	knowledgeWorkspaceCtrl := knowledgectrl.NewKnowledgeWorkspaceController(&openAPISourceAdapter{svc: service.NewOpenAPIService()})
	knowledgeWorkspaceCtrl.RegisterRoutes(auth)

	knowledgeMerchantCtrl := knowledgectrl.NewKnowledgeMerchantController()
	knowledgeMerchantCtrl.RegisterRoutes(auth)
}

// setupBackupRoutes 备份恢复路由
//
// 权限分级（2026-08-18 三轮发现）：CreateBackup / DeleteBackup / RestoreBackup admin only
// （备份含全库客户数据，恢复可覆盖全库；GetBackupList 保留任意登录查看）。
func setupBackupRoutes(auth *gin.RouterGroup) {
	backupCtrl := controller.NewBackupController()
	restoreCtrl := controller.NewRestoreController()
	auth.GET("/backups", backupCtrl.GetBackupList)
	auth.GET("/backups/:id", backupCtrl.GetBackupByID)
	auth.GET("/restore/list", restoreCtrl.GetRestoreList)
	auth.GET("/restore/last", restoreCtrl.GetLastRestore)
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	admin.POST("/backups", backupCtrl.CreateBackup)
	admin.DELETE("/backups/:id", backupCtrl.DeleteBackup)
	admin.POST("/restore", restoreCtrl.RestoreBackup)
}

// setupMigrationRoutes 数据库迁移管理路由
// 路径由原 /upgrade/* 改为 /migration/*（M3 重命名以避免与"OTA 升级"概念混淆）。
// controller 结构体已重命名为 MigrationController。
//
// 安全（2026-08-26 审计 P0-1）：
//   - POST /migration/task（执行升级）与 POST /migration/rollback（回滚）直接操作
//     数据库 schema 与数据，原挂载在普通 JWT 组，任意低权限登录用户均可触发，
//     构成数据丢失风险 → 写操作收敛为 admin only。
//   - 只读查询端点（task/history/records/current-version/available）保留任意登录可访问。
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



// setupI18nRoutes 注册多语言方案相关路由（术语表 CRUD + 校验预览 + 监控看板）
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



// setupTuningRoutes 注册 置信度/拟人度/反馈学习 统一管理 API
//
// 路由前缀: /api/admin/tuning/
// 中间件: 继承 auth group(InitGuard + JWTAuthMiddleware + LicenseGuard)
//
// 涵盖:
//   - 置信度信号/校准/阈值策略
//   - 拟人度评分/销冠基线
//   - 反馈事件/销冠对话/Prompt 候选/Bandit 臂
//   - 低质样本
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


// setupAIToolConfigRoutes 注册AI工具配置路由
func setupAIToolConfigRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	toolRepo := repository.NewAIToolConfigRepository(db)
	bindingRepo := repository.NewAIToolAccountBindingRepository(db)
	svc := service.NewAIToolConfigService(toolRepo, bindingRepo)
	ctrl := controller.NewAIToolConfigController(svc)

	g := auth.Group("/ai-tools")
	{
		g.GET("", ctrl.ListTools)
		g.GET("/:name", ctrl.GetTool)
		g.PUT("/:name/status", ctrl.UpdateToolStatus)
		g.POST("/batch-status", ctrl.BatchUpdateStatus)

		g.GET("/:name/accounts", ctrl.GetToolAccounts)
		g.POST("/:name/accounts", ctrl.BindAccount)
		g.DELETE("/:name/accounts/:account_type/:account_id", ctrl.UnbindAccount)
	}
}

