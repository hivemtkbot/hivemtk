package router

import (
	"context"

	geoctrl "hivemtk-user/internal/geo/controller"
	georepo "hivemtk-user/internal/geo/repository"
	geoservice "hivemtk-user/internal/geo/service"
	mainrepo "hivemtk-user/internal/repository"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupGeoRoutes GEO 生成式引擎优化功能路由。
//
// 权限分级：config 写入/优化（PUT /geo/config、POST /geo/config/optimize）
// 仅管理员可操作，防止 staff 误改品牌配置导致全链路 GEO 内容偏移。
func SetupGeoRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	// 初始化 repositories（显式注入 gormDB，避免隐藏全局依赖，测试可替换）
	keywordRepo := georepo.NewGeoKeywordRepositoryWithDB(gormDB)
	articleRepo := georepo.NewGeoArticleRepositoryWithDB(gormDB)
	optimizationRepo := georepo.NewGeoOptimizationRepositoryWithDB(gormDB)
	verifyRepo := georepo.NewGeoVerifyResultRepositoryWithDB(gormDB)
	apiCallRepo := georepo.NewGeoAPICallRepositoryWithDB(gormDB)
	configRepo := georepo.NewGeoConfigRepositoryWithDB(gormDB)
	accountRepo := georepo.NewGeoPlatformAccountRepositoryWithDB(gormDB)
	publishRecordRepo := georepo.NewGeoPublishRecordRepositoryWithDB(gormDB)
	kbDocRepo := georepo.NewGeoKnowledgeDocumentRepositoryWithDB(gormDB)
	wfRepo := georepo.NewGeoWorkflowRepositoryWithDB(gormDB)
	execRepo := georepo.NewGeoWorkflowExecutionRepositoryWithDB(gormDB)
	tplRepo := georepo.NewGeoWorkflowTemplateRepositoryWithDB(gormDB)

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
	// v3 决策链化: 捕获线索写主域线索库（ClueTypeLeadMining=8）
	mainClueRepo := mainrepo.NewClueRepositoryWithDB(gormDB)

	// v3 决策链报表: L4 线索捕获数（clue SourceID=chain 归因键）
	decisionCtrl := geoctrl.NewGeoDecisionController(
		georepo.NewGeoQueryChainRepository(gormDB),
		georepo.NewGeoContentTaskRepository(gormDB),
		newGeoLeadReporter(gormDB),
	)
	auth.GET("/geo/decision/report", decisionCtrl.GetDecisionReport)
	auth.GET("/geo/decision/tasks", decisionCtrl.GetTasks)
	auth.POST("/geo/decision/tasks/:id/done", decisionCtrl.MarkTaskDone)

	// v3 竞品对齐分析（A1 SOV / A6 爬虫 / A7 不准确检测）
	analyticsSvc := geoservice.NewGeoDecisionAnalyticsService(
		georepo.NewGeoVerifyResultRepositoryWithDB(gormDB),
		georepo.NewGeoContentTaskRepository(gormDB),
		georepo.NewGeoQueryChainRepository(gormDB),
		georepo.NewGeoCrawlerVisitRepository(gormDB),
		llmAdapter,
		georepo.NewGeoAPICallRepositoryWithDB(gormDB))
	auth.GET("/geo/sov", func(c *gin.Context) {
		result, err := analyticsSvc.GetShareOfVoice(c.Request.Context(), c.Query("intent"))
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "data": result})
	})
	auth.GET("/geo/crawler-stats", func(c *gin.Context) {
		result, err := analyticsSvc.GetCrawlerStats(c.Request.Context())
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "data": result})
	})
	auth.POST("/geo/inaccurate-claims", func(c *gin.Context) {
		var body struct{ BrandName string `json:"brand_name"` }
		if err := c.BindJSON(&body); err != nil || body.BrandName == "" {
			c.JSON(400, gin.H{"error": "brand_name required"})
			return
		}
		result, err := analyticsSvc.DetectInaccurateClaims(c.Request.Context(), body.BrandName)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"code": 0, "data": result})
	})
	// v3 GEO 决策链化 Phase3：注入线索捕获端口（capture_lead 执行器 → 主域 clue）
	geoChainRepo := georepo.NewGeoQueryChainRepository(gormDB)
	wfSvc.RegisterCaptureLeadExecutor(geoservice.CaptureLeadFunc(func(ctx context.Context, contact, contactType, chainID, intent string) (string, error) {
		clue := &model.Clue{
			Account:  contact,
			Name:     contact,
			SourceID: chainID, // v3 修正：保留思维链归因键（原固定值丢失归因链）
			Type:     int64(9), // ClueTypeGeoCapture：GEO 决策链捕获专用类型
		}
		if err := mainClueRepo.Create(ctx, clue); err != nil {
			return "", err
		}
		// v3 决策链化 Phase3 收口：捕获即绑定 OneID，inbox 回填据此定位链
		if clue.OneID != "" && geoChainRepo != nil {
			utils.WarnErrKV("router.geo.bindOneIDToChain", gormDB.WithContext(ctx).
				Table("geo_query_chains").
				Where("chain_id = ? AND (one_id = '' OR one_id IS NULL)", chainID).
				Update("one_id", clue.OneID).Error, "chain_id", chainID, "one_id", clue.OneID, "clue_id", clue.ID)
		}
		return clue.ID, nil
	}))
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
	geo.POST("/techconfig/llms-txt", resCtrl.GenerateLLMsTxt)

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


// geoLeadReporter L4 捕获线索精确统计：
// 决策链捕获线索特征 = Type=8(ClueTypeLeadMining) 且 SourceID 为思维链键前缀
type geoLeadReporter struct{ db *gorm.DB }

func newGeoLeadReporter(db *gorm.DB) *geoLeadReporter { return &geoLeadReporter{db: db} }

func (r *geoLeadReporter) CountCapturedLeads(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.Clue{}).
		Where("type = ?", int64(9)).
		Where("(source_id LIKE 'probe:%' OR source_id LIKE 'verify:%' OR source_id LIKE 'inbox:%')").
		Count(&n).Error
	return n, err
}
