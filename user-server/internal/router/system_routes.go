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
	"marketing/internal/migration"
	"marketing/internal/migration/migrations"
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

	// 工具集成配置（物流/售后回写，凭证存数据库 agent.tool_integrations，非环境变量）
	toolIntegrationCtrl := controller.NewToolIntegrationConfigController()
	auth.GET("/agent/tool-integrations", toolIntegrationCtrl.GetConfig)
	auth.PUT("/agent/tool-integrations", toolIntegrationCtrl.SaveConfig)

	// Agent Loop 运行期调参（max_tools / max_loop_iterations，存数据库 agent.settings，非环境变量）
	agentSettingsCtrl := controller.NewAgentSettingsController()
	auth.GET("/agent/settings", agentSettingsCtrl.GetConfig)
	auth.PUT("/agent/settings", agentSettingsCtrl.SaveConfig)

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

	// 管理员配置（仅 UI 行为；不暴露密码）
	// 超管密码唯一来源是 DB（InitAdmin 写入），API 不返回明文。
	adminConfigCtrl := controller.NewAdminConfigController()
	auth.GET("/admin/config", adminConfigCtrl.GetAdminConfig)

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

	// C 域 缺口修复 - 召回率监控 / 内容风控 / 健康度评估
	// （统一在此注册，避免在 auth_routes.go 引入对 rag 子包的耦合）
	// 私域部署: RagAlertService 已删除, 健康度评分不再纳入 alert 维度
	ragMetricsSvc := service.NewRagMetricsService(db.GetDB())
	ragHealthSvc := service.NewRagHealthService(db.GetDB(), ragMetricsSvc)
	ragHealthCtrl := controller.NewRagHealthController(ragHealthSvc)
	ragHealthCtrl.RegisterRoutes(auth)

	// RagSafetyGuardService 已整体移除（本地部署不需要内容安全护栏）

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

// setupMigrationRoutes 数据库迁移管理路由
// 路径由原 /upgrade/* 改为 /migration/*（M3 重命名以避免与"OTA 升级"概念混淆）。
// controller 结构体已重命名为 MigrationController（兼容别名 NewUpgradeController）。
func setupMigrationRoutes(auth *gin.RouterGroup) {
	registry := migration.NewMigrationRegistry()
	migrationSvc := migration.NewMigrationService(registry, db.GetDB(), migrations.RegisterMigrations)
	migrationCtrl := controller.NewMigrationController(migrationSvc)
	auth.GET("/migration/task/:id", migrationCtrl.GetUpgradeTask)
	auth.GET("/migration/history", migrationCtrl.GetUpgradeHistory)
	auth.GET("/migration/records", migrationCtrl.GetMigrationRecords)
	auth.GET("/migration/current-version", migrationCtrl.GetCurrentVersion)
	auth.POST("/migration/task", migrationCtrl.CreateUpgradeTask)
	auth.POST("/migration/rollback", migrationCtrl.Rollback)
	auth.GET("/migration/available", migrationCtrl.GetAvailableUpgrades)
}
