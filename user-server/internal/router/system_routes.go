package router

import (
	knowledgectrl "marketing/internal/aiagent/knowledge/controller"
	knowledgerepo "marketing/internal/aiagent/knowledge/repository"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/aiagent/llm"
	rag_core "marketing/internal/aiagent/rag/core"
	rag_service "marketing/internal/aiagent/rag/service"
	"marketing/internal/aiagent/vector"
	"marketing/internal/controller"
	"marketing/internal/etl"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupSystemRoutes 系统管理路由
func setupSystemRoutes(auth *gin.RouterGroup) {
	// 系统配置
	systemConfigCtrl := controller.NewSystemConfigController()
	auth.GET("/system/config", systemConfigCtrl.GetConfig)
	auth.POST("/system/config", systemConfigCtrl.SaveConfig)

	// 系统运维
	systemOpsCtrl := controller.NewSystemOpsController()
	auth.POST("/system/restart", systemOpsCtrl.RestartServer)
	auth.GET("/system/logs", systemOpsCtrl.GetSystemLogs)
	auth.GET("/system/stats", systemOpsCtrl.GetSystemStats)
	auth.GET("/stats/system", systemOpsCtrl.GetSystemStats) // 前端兼容路由
	auth.GET("/system/backup", systemOpsCtrl.GetBackupList)
	auth.POST("/system/backup", systemOpsCtrl.CreateBackup)
	auth.POST("/system/restore", systemOpsCtrl.RestoreBackup)

	// 系统信息（已在 admin_routes.go 的公开路由中注册）
	// systemInfoCtrl := controller.NewSystemInfoController()
	// auth.GET("/system/info", systemInfoCtrl.GetSystemInfo)

	// 系统用户（已在 admin_routes.go 的公开路由中注册，用于系统初始化）
	// systemUserCtrl := controller.NewSystemUserController()
	// auth.POST("/system/create-default-admin", systemUserCtrl.CreateDefaultAdmin)

	// 管理员配置
	adminConfigCtrl := controller.NewAdminConfigController()
	auth.GET("/admin/config", adminConfigCtrl.GetAdminConfig)
	auth.GET("/admin/default-credentials", adminConfigCtrl.GetDefaultCredentials)

	// OBS 配置
	obsConfigCtrl := controller.NewObsConfigController()
	auth.GET("/obs/config", obsConfigCtrl.GetConfigList)
	auth.POST("/obs/config", obsConfigCtrl.CreateConfig)
	auth.GET("/obs/config/:id", obsConfigCtrl.GetConfig)
	auth.PUT("/obs/config/:id", obsConfigCtrl.UpdateConfig)
	auth.DELETE("/obs/config/:id", obsConfigCtrl.DeleteConfig)
	auth.POST("/obs/config/:id/test", obsConfigCtrl.TestConnection)
	auth.POST("/obs/config/:id/default", obsConfigCtrl.SetDefault)
	auth.GET("/obs/config/default", obsConfigCtrl.GetDefaultConfig)
}

// setupRagRoutes RAG 知识库管理路由
func setupRagRoutes(auth *gin.RouterGroup) {
	ragRepo := knowledgerepo.NewRagConfigRepository(db.GetDB())
	llmService := llm.NewLLMService()
	rageEngine := rag_core.NewRAGEngine(nil)
	ragCoreService := rag_service.NewRAGService(llmService, rageEngine)
	docProcessor := etl.NewDocumentProcessor(nil)
	// remoteVectorizer 调用本地 embedding-server（OpenAI 兼容 /v1/embeddings，
	// 私域基线：纯 Go 本地服务，数据不出域），并非 mock。
	remoteVectorizer := vector.NewRemoteVectorizer(1024)
	vectorStore := vector.NewInMemoryVectorStore(1024)
	vecProcessor := vector.NewVectorProcessor(remoteVectorizer, vectorStore)
	ragService := knowledgesvc.NewRagConfigService(ragRepo, ragCoreService, docProcessor, vecProcessor)
	ragConfigCtrl := knowledgectrl.NewRagConfigController(ragService)
	ragConfigCtrl.RegisterRoutes(auth)

	// C 域 P1 缺口修复 - 召回率监控 / 内容风控 / 健康度评估
	// （统一在此注册，避免在 auth_routes.go 引入对 rag 子包的耦合）
	ragMetricsSvc := service.NewRagMetricsService(db.GetDB())
	ragAlertSvc := service.NewRagAlertService(db.GetDB(), ragMetricsSvc)
	ragHealthSvc := service.NewRagHealthService(db.GetDB(), ragMetricsSvc, ragAlertSvc)
	ragHealthCtrl := controller.NewRagHealthController(ragHealthSvc)
	ragHealthCtrl.RegisterRoutes(auth)

	ragSafetyCtrl := controller.NewRagSafetyGuardController(service.NewRagSafetyGuardService(db.GetDB()))
	ragSafetyCtrl.RegisterRoutes(auth)

	ragRecallCtrl := controller.NewRagRecallMonitorController(service.NewRagRecallMonitorService(db.GetDB(), 0, 0))
	ragRecallCtrl.RegisterRoutes(auth)
}

// setupKnowledgeBaseRoutes 知识库管理路由(基础 + 工作台 + 商户视角增强)
func setupKnowledgeBaseRoutes(auth *gin.RouterGroup) {
	// 基础知识库路由
	knowledgeBaseCtrl := knowledgectrl.NewKnowledgeBaseController()
	knowledgeBaseCtrl.RegisterRoutes(auth)

	// 知识库工作台路由(UI 可视化导入 + OpenAPI + 统计)
	knowledgeWorkspaceCtrl := knowledgectrl.NewKnowledgeWorkspaceController()
	knowledgeWorkspaceCtrl.RegisterRoutes(auth)

	// 商户视角增强路由(批量导入/Playground/分段编辑/反馈/Token/外部接入)
	knowledgeMerchantCtrl := knowledgectrl.NewKnowledgeMerchantController()
	knowledgeMerchantCtrl.RegisterRoutes(auth)
}

// setupBackupRoutes 备份恢复路由
func setupBackupRoutes(auth *gin.RouterGroup) {
	backupCtrl := controller.NewBackupController()
	auth.GET("/backups", backupCtrl.GetBackupList)
	auth.GET("/backups/:id", backupCtrl.GetBackupByID)
	auth.POST("/backups", backupCtrl.CreateBackup)
	auth.DELETE("/backups/:id", backupCtrl.DeleteBackup)

	restoreCtrl := controller.NewRestoreController()
	auth.POST("/restore", restoreCtrl.RestoreBackup)
	auth.GET("/restore/list", restoreCtrl.GetRestoreList)
	auth.GET("/restore/last", restoreCtrl.GetLastRestore)
}

// setupUpgradeRoutes 版本升级管理路由
func setupUpgradeRoutes(auth *gin.RouterGroup) {
	upgradeCtrl := controller.NewUpgradeController()
	auth.GET("/upgrade/task/:id", upgradeCtrl.GetUpgradeTask)
	auth.GET("/upgrade/history", upgradeCtrl.GetUpgradeHistory)
	auth.GET("/upgrade/migration-records", upgradeCtrl.GetMigrationRecords)
	auth.GET("/upgrade/current-version", upgradeCtrl.GetCurrentVersion)
	auth.POST("/upgrade/task", upgradeCtrl.CreateUpgradeTask)
	auth.POST("/upgrade/rollback", upgradeCtrl.Rollback)
	auth.GET("/upgrade/available", upgradeCtrl.GetAvailableUpgrades)
}
