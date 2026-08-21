package router

import (
	geoctrl "hivemtk-user/internal/geo/controller"
	georepo "hivemtk-user/internal/geo/repository"
	geoservice "hivemtk-user/internal/geo/service"
	"hivemtk-user/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupGeoRoutes GEO 生成式引擎优化功能路由。
//
// 权限分级：config 写入/优化（PUT /geo/config、POST /geo/config/optimize）
// 仅管理员可操作，防止 staff 误改品牌配置导致全链路 GEO 内容偏移。
func setupGeoRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	// 初始化 repositories
	keywordRepo := georepo.NewGeoKeywordRepository()
	articleRepo := georepo.NewGeoArticleRepository()
	optimizationRepo := georepo.NewGeoOptimizationRepository()
	verifyRepo := georepo.NewGeoVerifyResultRepository()
	apiCallRepo := georepo.NewGeoAPICallRepository()
	configRepo := georepo.NewGeoConfigRepository()
	accountRepo := georepo.NewGeoPlatformAccountRepository()
	publishRecordRepo := georepo.NewGeoPublishRecordRepository()
	kbDocRepo := georepo.NewGeoKnowledgeDocumentRepository()
	wfRepo := georepo.NewGeoWorkflowRepository()
	execRepo := georepo.NewGeoWorkflowExecutionRepository()
	tplRepo := georepo.NewGeoWorkflowTemplateRepository()

	// 初始化 LLM 适配器（复用 hivemtk 全局 Dispatcher）
	llmAdapter := geoservice.NewLLMAdapter()

	// 初始化 services
	keywordSvc := geoservice.NewKeywordService(keywordRepo, apiCallRepo, llmAdapter)
	contentSvc := geoservice.NewContentService(articleRepo, optimizationRepo, apiCallRepo, llmAdapter)
	verifySvc := geoservice.NewVerificationService(verifyRepo, apiCallRepo, llmAdapter)
	reportSvc := geoservice.NewReportService(articleRepo, keywordRepo, optimizationRepo, verifyRepo, apiCallRepo)
	configSvc := geoservice.NewConfigService(configRepo, llmAdapter)
	platformSvc := geoservice.NewPlatformService(accountRepo, publishRecordRepo, articleRepo)
	kbSvc := geoservice.NewKBService(kbDocRepo, llmAdapter)
	wfSvc := geoservice.NewWorkflowService(wfRepo, execRepo, tplRepo, llmAdapter)
	keSvc := geoservice.NewKeywordEnhanceService(keywordRepo, verifyRepo, llmAdapter)

	// 初始化 controllers
	keywordCtrl := geoctrl.NewKeywordController(keywordSvc)
	contentCtrl := geoctrl.NewContentController(contentSvc)
	verifyCtrl := geoctrl.NewVerificationController(verifySvc)
	reportCtrl := geoctrl.NewReportController(reportSvc)
	configCtrl := geoctrl.NewConfigController(configSvc)
	platformCtrl := geoctrl.NewPlatformController(platformSvc)
	kbCtrl := geoctrl.NewKBController(kbSvc)
	wfCtrl := geoctrl.NewWorkflowController(wfSvc)
	resCtrl := geoctrl.NewResourceController()
	keCtrl := geoctrl.NewKeywordEnhanceController(keSvc)

	// 注册路由（统一挂在 /geo 下）
	geo := auth.Group("/geo")

	// 关键词路由
	geo.POST("/keywords/mine", keywordCtrl.MineKeywords)
	geo.POST("/keywords/expand", keywordCtrl.SemanticExpand)
	geo.POST("/keywords/cluster", keywordCtrl.TopicCluster)
	geo.GET("/keywords/list", keywordCtrl.GetKeywordList)
	geo.DELETE("/keywords/:id", keywordCtrl.DeleteKeyword)

	// 内容路由
	geo.POST("/content/generate", contentCtrl.GenerateContent)
	geo.POST("/content/optimize", contentCtrl.OptimizeContent)
	geo.POST("/content/score", contentCtrl.ScoreContent)
	geo.POST("/content/eeat", contentCtrl.EnhanceEEAT)
	geo.POST("/content/schema", contentCtrl.GenerateSchema)
	geo.POST("/content/uniqueness", contentCtrl.CheckUniqueness)
	geo.GET("/content/list", contentCtrl.GetArticleList)
	geo.GET("/content/:id", contentCtrl.GetArticleByID)

	// 验证路由
	geo.POST("/verification/verify", verifyCtrl.VerifyArticle)
	geo.POST("/verification/negative", verifyCtrl.MonitorNegative)
	geo.GET("/verification/results/:article_id", verifyCtrl.GetVerifyResults)

	// 报表路由
	geo.GET("/reports/summary", reportCtrl.GetReport)
	geo.GET("/reports/roi", reportCtrl.GetROI)
	geo.GET("/reports/api-costs", reportCtrl.GetAPICosts)

	// 配置路由（读取：所有登录用户）
	geo.GET("/config", configCtrl.GetConfig)

	// 平台同步发布路由
	geo.GET("/platform/platforms", platformCtrl.ListPlatforms)
	geo.GET("/platform/accounts", platformCtrl.ListAccounts)
	geo.POST("/platform/accounts", platformCtrl.SaveAccount)
	geo.DELETE("/platform/accounts/:id", platformCtrl.DeleteAccount)
	geo.POST("/platform/publish", platformCtrl.Publish)
	geo.GET("/platform/records", platformCtrl.ListPublishRecords)

	// 知识库路由
	geo.GET("/kb/documents", kbCtrl.List)
	geo.POST("/kb/documents", kbCtrl.Save)
	geo.GET("/kb/documents/:id", kbCtrl.Get)
	geo.DELETE("/kb/documents/:id", kbCtrl.Delete)
	geo.GET("/kb/search", kbCtrl.Search)
	geo.POST("/kb/ask", kbCtrl.Ask)

	// 工作流自动化路由
	geo.GET("/workflow/workflows", wfCtrl.List)
	geo.POST("/workflow/workflows", wfCtrl.Create)
	geo.GET("/workflow/workflows/:id", wfCtrl.Get)
	geo.PUT("/workflow/workflows/:id", wfCtrl.Update)
	geo.DELETE("/workflow/workflows/:id", wfCtrl.Delete)
	geo.POST("/workflow/workflows/:id/run", wfCtrl.Run)
	geo.GET("/workflow/workflows/:id/executions", wfCtrl.ListExecutions)
	geo.GET("/workflow/templates", wfCtrl.ListTemplates)
	geo.POST("/workflow/templates", wfCtrl.CreateTemplate)

	// 资源推荐路由
	geo.GET("/resources/agents", resCtrl.GetAgents)
	geo.GET("/resources/tools", resCtrl.GetTools)
	geo.GET("/resources/papers", resCtrl.GetPapers)
	geo.GET("/resources/communities", resCtrl.GetCommunities)
	geo.GET("/resources/summary", resCtrl.GetResourceSummary)
	geo.GET("/resources/search", resCtrl.SearchResources)

	// 技术配置路由
	geo.POST("/techconfig/robots", resCtrl.GenerateRobots)
	geo.POST("/techconfig/sitemap", resCtrl.GenerateSitemap)

	// 质量指标路由
	geo.POST("/metrics/analyze", resCtrl.AnalyzeMetrics)

	// 关键词数据增强路由
	geo.GET("/keyword-enhance/analyze", keCtrl.Analyze)
	geo.POST("/keyword-enhance/enhance", keCtrl.Enhance)

	// 配置路由（写入/优化：仅管理员）
	geoAdmin := geo.Group("")
	geoAdmin.Use(middleware.AdminAuthMiddleware())
	geoAdmin.PUT("/config", configCtrl.UpdateConfig)
	geoAdmin.POST("/config/optimize", configCtrl.OptimizeConfig)
}
