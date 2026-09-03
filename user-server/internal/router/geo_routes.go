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

	"hivemtk-user/internal/aiagent/llm"

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
	keywordGroupRepo := georepo.NewGeoKeywordGroupRepositoryWithDB(gormDB)
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
	chainRepo := georepo.NewGeoQueryChainRepository(gormDB)
	taskRepo := georepo.NewGeoContentTaskRepository(gormDB)

	// 初始化 LLM 适配器（复用 hivemtk 全局 Dispatcher）
	llmAdapter := geoservice.NewLLMAdapter()

	// 初始化 services
	keywordSvc := geoservice.NewKeywordService(keywordGroupRepo, keywordRepo, apiCallRepo, llmAdapter)
	contentSvc := geoservice.NewContentService(articleRepo, optimizationRepo, apiCallRepo, kbDocRepo, llmAdapter)
	verifySvc := geoservice.NewVerificationService(verifyRepo, apiCallRepo, chainRepo, taskRepo, llmAdapter, geoservice.NewDefaultSearchProbe())
	reportSvc := geoservice.NewReportService(articleRepo, keywordRepo, optimizationRepo, verifyRepo, apiCallRepo)
	configSvc := geoservice.NewConfigService(configRepo, llmAdapter)
	platformSvc := geoservice.NewPlatformService(accountRepo, publishRecordRepo, articleRepo)
	kbSvc := geoservice.NewKBService(kbDocRepo, llmAdapter)
	wfSvc := geoservice.NewWorkflowService(wfRepo, execRepo, tplRepo, chainRepo, taskRepo, llmAdapter)
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
		georepo.NewGeoProbeRunRepositoryWithDB(gormDB),
		georepo.NewGeoConfigRepositoryWithDB(gormDB),
		georepo.NewGeoContentTaskRepository(gormDB),
		georepo.NewGeoQueryChainRepository(gormDB),
		georepo.NewGeoCrawlerVisitRepository(gormDB),
		llmAdapter,
		georepo.NewGeoAPICallRepositoryWithDB(gormDB))
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
	reportCtrl := geoctrl.NewReportController(reportSvc, analyticsSvc)
	configCtrl := geoctrl.NewConfigController(configSvc)
	platformCtrl := geoctrl.NewPlatformController(platformSvc)
	kbCtrl := geoctrl.NewKBController(kbSvc)
	wfCtrl := geoctrl.NewWorkflowController(wfSvc)
	tmCtrl := geoctrl.NewTechMetricsController()
	keCtrl := geoctrl.NewKeywordEnhanceController(keSvc)

	// 多引擎探针聚合：ProbeService + 爬虫/实体辅助 controller
	probeRepo := georepo.NewGeoProbeRunRepositoryWithDB(gormDB)
	probes := geoservice.NewEngineProbesFromDB(gormDB)
	probeSvc := geoservice.NewProbeService(probes, probeRepo)
	probeCtrl := geoctrl.NewProbeController(probeSvc)

	// 定时任务管理（互斥/历史/调度配置统一由 JobManager 处理）
	jobCtrl := geoctrl.NewJobController(geoservice.GetGeoJobManager())

	sourceRepo := georepo.NewGeoSourceCatalogRepositoryWithDB(gormDB)
	crawlerSvc := geoservice.NewCrawlerService(sourceRepo)
	sourceCtrl := geoctrl.NewSourceCatalogController(crawlerSvc)

	entityRepo := georepo.NewGeoEntityRepositoryWithDB(gormDB)
	extractSvc := geoservice.NewEntityExtractorService(
		entityRepo,
		kbDocRepo,
		llm.GetGlobalDispatcher(),
	)
	entityCtrl := geoctrl.NewEntityController(entityRepo, extractSvc)

	// 竞品管理
	competitorRepo := georepo.NewGeoCompetitorRepository()
	competitorSvc := geoservice.NewGeoCompetitorService(competitorRepo)
	competitorCtrl := geoctrl.NewCompetitorController(competitorSvc)

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

	// v3 竞品对齐分析（A1 SOV / A6 爬虫 / A7 不准确检测）
	geo.GET("/sov", reportCtrl.ShareOfVoice)
	geo.GET("/crawler-stats", reportCtrl.CrawlerStats)
	geo.POST("/crawler/run", reportCtrl.RunCrawler)
	geo.POST("/inaccurate-claims", reportCtrl.InaccurateClaims)

	// 竞品管理（admin 权限，所有 geo 登录用户可增删改）
	geo.GET("/competitors", competitorCtrl.List)
	geo.GET("/competitors/:id", competitorCtrl.Get)
	geo.POST("/competitors", competitorCtrl.Create)
	geo.PUT("/competitors/:id", competitorCtrl.Update)
	geo.DELETE("/competitors/:id", competitorCtrl.Delete)

	// 配置路由（读取：所有登录用户）
	geo.GET("/config", configCtrl.GetConfig)

	// 平台同步发布路由
	geo.GET("/platform/platforms", platformCtrl.ListPlatforms)
	geo.GET("/platform/accounts", platformCtrl.ListAccounts)
	geo.GET("/platform/records", platformCtrl.ListPublishRecords)

	// 知识库路由
	geo.GET("/kb/documents", kbCtrl.List)
	geo.GET("/kb/documents/:id", kbCtrl.Get)
	geo.GET("/kb/search", kbCtrl.Search)
	geo.POST("/kb/ask", kbCtrl.Ask)

	// 工作流自动化路由
	geo.GET("/workflow/workflows", wfCtrl.List)
	geo.GET("/workflow/workflows/:id", wfCtrl.Get)
	geo.GET("/workflow/workflows/:id/executions", wfCtrl.ListExecutions)
	geo.GET("/workflow/templates", wfCtrl.ListTemplates)

	// 技术配置路由（真实功能：robots.txt / sitemap.xml / llms.txt 生成器）
	geo.POST("/techconfig/robots", tmCtrl.GenerateRobots)
	geo.POST("/techconfig/sitemap", tmCtrl.GenerateSitemap)
	geo.POST("/techconfig/llms-txt", tmCtrl.GenerateLLMsTxt)

	// 质量指标路由（真实功能：regex 解析内容结构 / 信任信号分析）
	geo.POST("/metrics/analyze", tmCtrl.AnalyzeMetrics)

	// 关键词数据增强路由
	geo.GET("/keyword-enhance/analyze", keCtrl.Analyze)
	geo.POST("/keyword-enhance/enhance", keCtrl.Enhance)

	// === 多引擎探针路由 ===
	geo.GET("/probe/engines", probeCtrl.ListAvailableEngines)
	geo.POST("/probe/test", probeCtrl.TestSingle)
	geo.POST("/probe/all", probeCtrl.ProbeAll)
	geo.POST("/probe/run-negative", probeCtrl.RunNegativeMonitor)
	geo.POST("/probe/run-source-sync", probeCtrl.RunSourceSync)
	geo.GET("/probe/runs", probeCtrl.ListRuns)

	// === 信源目录路由 ===
	geo.GET("/source-catalog/levels", sourceCtrl.LookupLevels)

	// === 实体图谱路由 ===
	geo.GET("/entity/list", entityCtrl.ListEntities)
	geo.GET("/entity/:id/graph", entityCtrl.GetGraph)
	geo.POST("/entities/extract", entityCtrl.Extract)

	// P0-13 配置/发布/删除/调度类写操作 admin only（2026-08-31 四轮加固）
	geoAdmin := geo.Group("")
	geoAdmin.Use(middleware.AdminAuthMiddleware())
	geoAdmin.PUT("/config", configCtrl.UpdateConfig)
	geoAdmin.POST("/config/optimize", configCtrl.OptimizeConfig)
	// 平台账号 CRUD + 发布
	geoAdmin.POST("/platform/accounts", platformCtrl.SaveAccount)
	geoAdmin.DELETE("/platform/accounts/:id", platformCtrl.DeleteAccount)
	geoAdmin.POST("/platform/publish", platformCtrl.Publish)
	// 工作流写操作
	geoAdmin.POST("/workflow/workflows", wfCtrl.Create)
	geoAdmin.PUT("/workflow/workflows/:id", wfCtrl.Update)
	geoAdmin.DELETE("/workflow/workflows/:id", wfCtrl.Delete)
	geoAdmin.POST("/workflow/workflows/:id/run", wfCtrl.Run)
	geoAdmin.POST("/workflow/templates", wfCtrl.CreateTemplate)
	// kb 文档写操作
	geoAdmin.POST("/kb/documents", kbCtrl.Save)
	geoAdmin.DELETE("/kb/documents/:id", kbCtrl.Delete)
	// Cron 触发端点（防 staff 手工跑 SOV 排名扫描 / 爬虫调度）
	// SOV 刷新改由 /geo/jobs/:name/trigger 触发
	// === 定时任务管理（admin）===
	geoAdmin.GET("/jobs", jobCtrl.List)
	geoAdmin.GET("/jobs/runs", jobCtrl.Runs)
	geoAdmin.POST("/jobs/:name/trigger", jobCtrl.Trigger)
	geoAdmin.PUT("/jobs/:name/schedule", jobCtrl.UpdateSchedule)
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
