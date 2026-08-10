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
func setupRagRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	ragRepo := knowledgerepo.NewRagConfigRepository(gormDB)
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
	ragMetricsSvc := service.NewRagMetricsService(gormDB)
	ragHealthSvc := service.NewRagHealthService(gormDB, ragMetricsSvc)
	ragHealthCtrl := controller.NewRagHealthController(ragHealthSvc)
	ragHealthCtrl.RegisterRoutes(auth)

	// RagSafetyGuardService 已整体移除（本地部署不需要内容安全护栏）

	ragRecallCtrl := controller.NewRagRecallMonitorController(service.NewRagRecallMonitorService(gormDB, 0, 0))
	ragRecallCtrl.RegisterRoutes(auth)
}

// setupKnowledgeBaseRoutes 知识库管理路由(基础 + 工作台 + 商户视角增强)
func setupKnowledgeBaseRoutes(auth *gin.RouterGroup) {
	// 基础知识库路由
	knowledgeBaseCtrl := knowledgectrl.NewKnowledgeBaseController()
	knowledgeBaseCtrl.RegisterRoutes(auth)

	// 知识库工作台路由(UI 可视化导入 + OpenAPI + 统计)
	// P2-3：OpenAPI 数据源能力经窄接口适配器注入，aiagent 不再 import service
	knowledgeWorkspaceCtrl := knowledgectrl.NewKnowledgeWorkspaceController(&openAPISourceAdapter{svc: service.NewOpenAPIService()})
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
func setupMigrationRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	registry := migration.NewMigrationRegistry()
	migrationSvc := migration.NewMigrationService(registry, gormDB, migrations.RegisterMigrations)
	migrationCtrl := controller.NewMigrationController(migrationSvc)
	auth.GET("/migration/task/:id", migrationCtrl.GetUpgradeTask)
	auth.GET("/migration/history", migrationCtrl.GetUpgradeHistory)
	auth.GET("/migration/records", migrationCtrl.GetMigrationRecords)
	auth.GET("/migration/current-version", migrationCtrl.GetCurrentVersion)
	auth.POST("/migration/task", migrationCtrl.CreateUpgradeTask)
	auth.POST("/migration/rollback", migrationCtrl.Rollback)
	auth.GET("/migration/available", migrationCtrl.GetAvailableUpgrades)
}

// ============================================================================
// 以下内容合并自 i18n_routes.go（P1-2 router 文件数收敛）
// ============================================================================

// ============================================================================
// 多语言方案路由（v1.2 出海多语言方案）
// ----------------------------------------------------------------------------
// 注册：
//   1. 术语表管理路由（/api/glossaries/*）
//   2. 监控看板路由（/api/i18n/stats/*）
//
// 私域独立部署：无 merchant_id，B 端 JWT 鉴权
// ============================================================================

// setupI18nRoutes 注册多语言方案相关路由（术语表 CRUD + 校验预览 + 监控看板）
func setupI18nRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 1. 术语表管理（/api/glossaries/*）
	glossaryRepo := repository.NewGlossaryRepositoryWithDB(db)
	glossarySvc := translation.NewGlossaryService(glossaryRepo, nil)
	glossaryCtrl := controller.NewGlossaryController(glossarySvc)
	glossaryCtrl.RegisterRoutes(auth)

	// 2. 监控看板（/api/i18n/stats/*）
	statsRepo := repository.NewI18nStatsRepositoryWithDB(db)
	statsSvc := translation.NewI18nStatsService(statsRepo)
	statsCtrl := controller.NewI18nStatsController(statsSvc)
	statsCtrl.RegisterRoutes(auth)
}

// ============================================================================
// 以下内容合并自 tuning_routes.go（P1-2 router 文件数收敛）
// ============================================================================

// tuning_routes.go 注册 置信度/拟人度/反馈学习 统一管理 API
//
// 五层架构归属: L2 网关层
// 设计依据: docs/核心链路优化.md 第十五/十六/十七章

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

	// 1. 置信度信号
	tuning.GET("/confidence/signals", ctrl.ListConfidenceSignals)
	tuning.GET("/confidence/signals/:id", ctrl.GetConfidenceSignal)
	tuning.GET("/confidence/signals/stats", ctrl.StatsConfidenceSignals)

	// 2. 置信度校准
	tuning.GET("/confidence/calibrations", ctrl.ListCalibrations)

	// 3. 阈值策略
	tuning.GET("/confidence/policies", ctrl.ListThresholdPolicies)
	tuning.PUT("/confidence/policies", ctrl.UpsertThresholdPolicy)

	// 4. 拟人度评分
	tuning.GET("/humanize/scores", ctrl.ListHumanizeScores)
	tuning.GET("/humanize/scores/stats", ctrl.StatsHumanizeScores)

	// 5. 销冠基线
	tuning.GET("/humanize/baselines", ctrl.ListChampionBaselines)

	// 6. 反馈事件
	tuning.GET("/feedback/events", ctrl.ListFeedbackEvents)
	tuning.GET("/feedback/events/stats", ctrl.StatsFeedbackEvents)

	// 7. 销冠对话
	tuning.GET("/feedback/dialogues", ctrl.ListChampionDialogues)

	// 8. Prompt 候选
	tuning.GET("/prompt/candidates", ctrl.ListPromptCandidates)
	tuning.PUT("/prompt/candidates/:id/status", ctrl.UpdatePromptCandidateStatus)

	// 9. Bandit 臂
	tuning.GET("/bandit/arms", ctrl.ListBanditArms)

	// 10. 低质样本
	tuning.GET("/humanize/low-quality", ctrl.ListLowQualitySamples)
}

// ============================================================================
// 以下内容合并自 ai_tool_config_routes.go（P1-2 router 文件数收敛）
// ============================================================================

// setupAIToolConfigRoutes 注册AI工具配置路由
func setupAIToolConfigRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 创建依赖
	toolRepo := repository.NewAIToolConfigRepository(db)
	bindingRepo := repository.NewAIToolAccountBindingRepository(db)
	svc := service.NewAIToolConfigService(toolRepo, bindingRepo)
	ctrl := controller.NewAIToolConfigController(svc)

	// 注册路由
	g := auth.Group("/ai-tools")
	{
		// 工具配置
		g.GET("", ctrl.ListTools)
		g.GET("/:name", ctrl.GetTool)
		g.PUT("/:name/status", ctrl.UpdateToolStatus)
		g.POST("/batch-status", ctrl.BatchUpdateStatus)

		// 工具-账号绑定
		g.GET("/:name/accounts", ctrl.GetToolAccounts)
		g.POST("/:name/accounts", ctrl.BindAccount)
		g.DELETE("/:name/accounts/:account_type/:account_id", ctrl.UnbindAccount)
	}
}
