package router

import (
	contentctrl "hivemtk-user/internal/content/controller"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	opsctrl "hivemtk-user/internal/ops/controller"

	"github.com/gin-gonic/gin"
)

// setupBatchRoutes 批量操作路由
func setupBatchRoutes(auth *gin.RouterGroup) {
	batchImportCtrl := contentctrl.NewBatchImportController()
	auth.POST("/batch/import", batchImportCtrl.ImportFile)
	auth.GET("/batch/template", batchImportCtrl.DownloadTemplate)

	batchExportCtrl := contentctrl.NewBatchExportController()
	auth.POST("/batch/export", batchExportCtrl.ExportData)

	batchOpCtrl := contentctrl.NewBatchOperationController()
	auth.POST("/batch/delete", batchOpCtrl.BatchDelete)
	auth.POST("/batch/update", batchOpCtrl.BatchUpdate)
	auth.GET("/batch/tools", batchOpCtrl.GetTools)
	auth.GET("/batch/histories", batchOpCtrl.GetHistories)
	auth.GET("/batch/histories/:id", batchOpCtrl.GetHistoryByID)
	auth.POST("/batch/histories/:id/cancel", batchOpCtrl.CancelHistory)
	auth.POST("/batch/preview", batchOpCtrl.Preview)
}

// setupAIContentRoutes AI 内容创作路由
//
// 权限分级（2026-08-18 三轮发现）：GenerateContent / SaveRecord admin only
// （AI 生成调 LLM 烧 token；SaveRecord 写入正式文案库 → 客户可见，防 staff 投毒）。
// history 列表 / 模板列表 / 收藏评分等读写操作任意登录。
func setupAIContentRoutes(auth *gin.RouterGroup) {
	aiContentCtrl := contentctrl.NewAIContentController()
	auth.GET("/ai/history", aiContentCtrl.GetGenerationHistory)
	auth.GET("/ai/history/:id", aiContentCtrl.GetRecordByID)
	auth.POST("/ai/history", aiContentCtrl.CreateHistory)
	auth.POST("/ai/history/:id/favorite", aiContentCtrl.FavoriteRecord)
	auth.POST("/ai/history/:id/rate", aiContentCtrl.RateRecord)
	auth.GET("/ai/templates", aiContentCtrl.GetTemplates)
	auth.GET("/ai/templates/:id", aiContentCtrl.GetTemplateByID)
	auth.GET("/ai/template-types", aiContentCtrl.GetTemplateTypes)
	admin := auth.Group("/ai", middleware.AdminAuthMiddleware())
	{
		admin.POST("/generate", aiContentCtrl.GenerateContent)
		admin.POST("/history/:id/save", aiContentCtrl.SaveRecord)
		admin.DELETE("/history/:id", aiContentCtrl.DeleteRecord)
		admin.POST("/templates", aiContentCtrl.CreateTemplate)
		admin.PUT("/templates/:id", aiContentCtrl.UpdateTemplate)
		admin.DELETE("/templates/:id", aiContentCtrl.DeleteTemplate)
	}
}

// setupUserSegmentRoutes 用户分层 RFM 路由
//
// 权限分级（2026-08-18 三轮发现）：RFM 规则 / 批量计算 admin only
// （RFM 规则是分组营销的判定标准，staff 改规则会污染分群结果）。
func setupUserSegmentRoutes(auth *gin.RouterGroup) {
	userSegmentCtrl := controller.NewUserSegmentController()
	auth.GET("/user-segment/rfm/rule", userSegmentCtrl.GetRFMRule)
	auth.GET("/user-segment/rfm/rules", userSegmentCtrl.ListRFMRules)
	auth.GET("/user-segment/rfm/list", userSegmentCtrl.GetRFMList)
	auth.GET("/user-segment/rfm/user", userSegmentCtrl.GetUserRFM)
	auth.GET("/user-segment/rfm/stats", userSegmentCtrl.GetRFMStats)
	auth.GET("/user-segment/layers", userSegmentCtrl.GetLayerDescription)
	admin := auth.Group("/user-segment/rfm", middleware.AdminAuthMiddleware())
	{
		admin.POST("/rule", userSegmentCtrl.SaveRFMRule)
		admin.PUT("/rule/:id", userSegmentCtrl.UpdateRFMRule)
		admin.DELETE("/rule/:id", userSegmentCtrl.DeleteRFMRule)
		admin.POST("/calculate", userSegmentCtrl.CalculateRFM)
	}
}

// setupMarketingFlowRoutes 营销自动化流程路由
//
// 权限分级（2026-08-18 三轮发现）：写操作（Create/Update/Delete/Activate/Pause/Stop）admin only
// 防 staff 误触发批量营销活动 / 删 SOP / 暂停客服流程。
func setupMarketingFlowRoutes(auth *gin.RouterGroup) {
	marketingFlowCtrl := contentctrl.NewMarketingFlowController()
	// R39 零散补齐
	extrasCtrl := controller.NewCustomerEventBatchController()
	auth.POST("/customer-events/batch", extrasCtrl.TrackBatch)
	// R39: AI 销冠驾驶舱聚合
	auth.GET("/ai/sales-cockpit", controller.NewSalesCockpitController().GetCockpit)
	wvCtrl := controller.NewWebVitalsController()
	auth.POST("/monitor/web-vitals", wvCtrl.Report)
	mfSyncCtrl := controller.NewMarketingFlowSyncController()
	auth.POST("/marketing-flows/:id/sync-ab-results", mfSyncCtrl.SyncABResults)

	auth.GET("/marketing-flows", marketingFlowCtrl.GetFlowList)
	auth.GET("/marketing-flows/:id", marketingFlowCtrl.GetFlowByID)
	auth.GET("/marketing-flows/:id/executions", marketingFlowCtrl.GetExecutionList)
	auth.GET("/marketing-flows/:id/stats", marketingFlowCtrl.GetExecutionStats)
	admin := auth.Group("/marketing-flows", middleware.AdminAuthMiddleware())
	{
		admin.POST("", marketingFlowCtrl.CreateFlow)
		admin.PUT("/:id", marketingFlowCtrl.UpdateFlow)
		admin.DELETE("/:id", marketingFlowCtrl.DeleteFlow)
		admin.POST("/:id/activate", marketingFlowCtrl.ActivateFlow)
		admin.POST("/:id/pause", marketingFlowCtrl.PauseFlow)
		admin.POST("/:id/stop", marketingFlowCtrl.StopFlow)
	}
}

// setupCustomReportRoutes 自定义报表路由
func setupCustomReportRoutes(auth *gin.RouterGroup) {
	customReportCtrl := opsctrl.NewCustomReportController()
	auth.GET("/custom-reports", customReportCtrl.GetReportList)
	auth.GET("/custom-reports/:id", customReportCtrl.GetReport)
	auth.POST("/custom-reports", customReportCtrl.CreateReport)
	auth.PUT("/custom-reports/:id", customReportCtrl.UpdateReport)
	auth.DELETE("/custom-reports/:id", customReportCtrl.DeleteReport)
	auth.GET("/custom-reports/templates", customReportCtrl.GetPublicTemplates)
	auth.POST("/custom-reports/templates/:id/use", customReportCtrl.UseTemplate)
	auth.GET("/custom-reports/:id/data", customReportCtrl.QueryReportData)
	auth.GET("/custom-reports/:id/export", customReportCtrl.ExportCSV)
}

// setupDashboardRoutes 数据大屏路由
func setupDashboardRoutes(auth *gin.RouterGroup, public *gin.RouterGroup) {
	dashboardCtrl := opsctrl.NewDashboardScreenController()
	auth.GET("/dashboards", dashboardCtrl.GetScreenList)
	auth.GET("/dashboards/data", dashboardCtrl.GetDashboardData)
	auth.GET("/dashboards/activities", dashboardCtrl.GetRealtimeActivities)
	auth.GET("/dashboards/:id", dashboardCtrl.GetScreenByID)
	auth.POST("/dashboards", dashboardCtrl.CreateScreen)
	auth.PUT("/dashboards/:id", dashboardCtrl.UpdateScreen)
	auth.DELETE("/dashboards/:id", dashboardCtrl.DeleteScreen)

	public.GET("/dashboards/public/:code", dashboardCtrl.PublicViewScreen)
}

// setupTemplateRoutes 模板市场路由
func setupTemplateRoutes(auth *gin.RouterGroup) {
	templateCtrl := contentctrl.NewTemplateMarketController()
	auth.GET("/templates", templateCtrl.GetTemplateList)
	auth.POST("/templates", templateCtrl.CreateTemplate)
	auth.GET("/templates/:id", templateCtrl.GetTemplateByID)
	auth.POST("/templates/:id/download", templateCtrl.DownloadTemplate)
	auth.POST("/templates/:id/rate", templateCtrl.RateTemplate)
	auth.GET("/templates/official", templateCtrl.GetOfficialTemplates)
	auth.GET("/templates/search", templateCtrl.SearchTemplates)
	auth.GET("/templates/my-downloads", templateCtrl.GetMyDownloads)
}

// setupScriptRoutes 话术库路由
func setupScriptRoutes(auth *gin.RouterGroup) {
	scriptCtrl := contentctrl.NewScriptTemplateController()
	auth.GET("/scripts", scriptCtrl.GetTemplateList)
	auth.GET("/scripts/:id", scriptCtrl.GetTemplateByID)
	auth.POST("/scripts", scriptCtrl.CreateTemplate)
	auth.PUT("/scripts/:id", scriptCtrl.UpdateTemplate)
	auth.DELETE("/scripts/:id", scriptCtrl.DeleteTemplate)
	auth.GET("/scripts/categories", scriptCtrl.GetCategories)
	auth.GET("/scripts/search", scriptCtrl.SearchTemplates)
	auth.GET("/scripts/public", scriptCtrl.GetPublicTemplates)
	auth.POST("/scripts/recommend", scriptCtrl.RecommendScript)
	auth.POST("/scripts/sync-to-library", scriptCtrl.SyncToLibrary)

	// T-6/T-7 话术版本管理 + AB 曝光统计（script-library 域）
	scriptLibCtrl := controller.NewScriptLibraryController()
	auth.GET("/script-library/:id/versions", scriptLibCtrl.ListVersions)
	auth.POST("/script-library/:id/versions", scriptLibCtrl.CreateVersion)
	auth.PUT("/script-library/:id/versions/:vid/activate", scriptLibCtrl.ActivateVersion)
	auth.POST("/script-library/:id/expire", scriptLibCtrl.ExpireScript)
	auth.GET("/script-library/:id/ab-stats", scriptLibCtrl.GetABStats)
	auth.PUT("/script-library/:id/ab-config", scriptLibCtrl.UpdateABConfig)
	auth.POST("/script-ab/conversion", scriptLibCtrl.RecordConversion)

	// K2 Feature Flags（Unleash/GrowthBook 管理端最小完备集）
	flagCtrl := controller.NewFeatureFlagController()
	auth.GET("/feature-flags", flagCtrl.List)
	auth.POST("/feature-flags", flagCtrl.Create)
	auth.GET("/feature-flags/stale", flagCtrl.Stale)
	auth.POST("/feature-flags/evaluate", flagCtrl.Evaluate)
	auth.POST("/feature-flags/evaluate-batch", flagCtrl.EvaluateBatch)
	auth.GET("/feature-flags/:id", flagCtrl.Get)
	auth.PUT("/feature-flags/:id", flagCtrl.Update)
	auth.DELETE("/feature-flags/:id", flagCtrl.Delete)
	auth.POST("/feature-flags/:id/enable", flagCtrl.Enable)
	auth.POST("/feature-flags/:id/disable", flagCtrl.Disable)
	auth.POST("/feature-flags/:id/rollout", flagCtrl.Rollout)
	auth.GET("/feature-flags/:id/audit", flagCtrl.Audit)
	auth.GET("/feature-flags/:id/eval-log", flagCtrl.EvalLogs)
	auth.GET("/feature-flags/:id/code-references", flagCtrl.CodeReferences)
	auth.POST("/feature-flags/:id/code-references", flagCtrl.RegisterCodeReference)
}

// setupABTestRoutes A/B 测试路由
//
// 权限分级（2026-08-18 多角度审计修复）：
//   - 写操作（Create/Update/Delete/Start/Pause/Stop）必须 admin
//   - 读操作（List/Get/Results/ConversionEvents）任意登录用户
func setupABTestRoutes(auth *gin.RouterGroup) {
	abCtrl := opsctrl.NewABExperimentController()
	auth.GET("/ab-experiments", abCtrl.GetExperimentList)
	auth.GET("/ab-experiments/:id", abCtrl.GetExperiment)
	auth.GET("/ab-experiments/:id/results", abCtrl.GetExperimentResults)
	auth.GET("/ab-experiments/:id/conversion-events", abCtrl.GetConversionEvents)
	// K5 AB 高级统计（GrowthBook 轻量版）
	auth.GET("/ab-experiments/:id/stats", abCtrl.GetAdvancedStats)
	auth.GET("/ab-experiments/:id/diagnostics", abCtrl.GetExperimentDiagnostics)
	auth.GET("/ab-experiments/:id/cuped", abCtrl.GetExperimentCUPED)
	auth.POST("/ab-experiments/:id/sequential-test", abCtrl.PostSequentialTest)
	auth.POST("/ab-experiments/:id/bayesian-test", abCtrl.PostBayesianTest)
	auth.GET("/ab-experiments/:id/results-with-reach", abCtrl.GetResultsWithReach)
	admin := auth.Group("/ab-experiments", middleware.AdminAuthMiddleware())
	{
		admin.POST("", abCtrl.CreateExperiment)
		admin.PUT("/:id", abCtrl.UpdateExperiment)
		admin.DELETE("/:id", abCtrl.DeleteExperiment)
		admin.POST("/:id/start", abCtrl.StartExperiment)
		admin.POST("/:id/pause", abCtrl.PauseExperiment)
		admin.POST("/:id/stop", abCtrl.StopExperiment)
	}
}

// setupChurnRoutes 流失预警路由
func setupChurnRoutes(auth *gin.RouterGroup) {
	churnCtrl := opsctrl.NewChurnPredictionController()
	auth.GET("/churn/prediction", churnCtrl.GetChurnPrediction)
	auth.GET("/churn/predictions", churnCtrl.GetChurnPredictions)
	auth.GET("/churn/high-risk-users", churnCtrl.GetHighRiskUsers)
	auth.GET("/churn/warnings", churnCtrl.GetChurnWarnings)
	auth.GET("/churn/unhandled-warnings", churnCtrl.GetUnhandledWarnings)
	auth.POST("/churn/warnings/:id/handle", churnCtrl.MarkWarningHandled)
	auth.POST("/churn/warnings/intervene", churnCtrl.InterveneUser)
	auth.GET("/churn/model-config", churnCtrl.GetModelConfig)
	auth.POST("/churn/model-config", churnCtrl.SaveModelConfig)
	auth.GET("/churn/statistics", churnCtrl.GetChurnStatistics)
	auth.GET("/churn/risk-distribution", churnCtrl.GetRiskDistribution)
}

// setupIntegrationRoutes 第三方对接路由
//
// 权限分级（2026-08-18 多角度审计修复）：
//   - 写操作（Create/Update/Delete/Test/Sync）必须 admin（含 corp_secret 等敏感凭据）
//   - 读操作（List/Get/SyncLogs/ExternalXxx）任意登录用户（业务查询需要）
func setupIntegrationRoutes(auth *gin.RouterGroup) {
	integrationCtrl := controller.NewIntegrationController()
	auth.GET("/integrations", integrationCtrl.GetAccountList)
	auth.GET("/integrations/:id", integrationCtrl.GetAccountByID)
	auth.GET("/integration/sync-logs", integrationCtrl.GetSyncLogs)
	auth.GET("/integration/external-customers", integrationCtrl.GetExternalCustomers)
	auth.GET("/integration/external-products", integrationCtrl.GetExternalProducts)
	auth.GET("/integration/external-orders", integrationCtrl.GetExternalOrders)
	auth.GET("/integration/external-orders-by-customer", integrationCtrl.GetExternalOrdersByCustomer)
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		admin.POST("/integrations", integrationCtrl.CreateAccount)
		admin.PUT("/integrations/:id", integrationCtrl.UpdateAccount)
		admin.DELETE("/integrations/:id", integrationCtrl.DeleteAccount)
		admin.POST("/integrations/:id/test", integrationCtrl.TestIntegration)
		admin.POST("/integrations/:id/sync-customers", integrationCtrl.SyncCustomers)
		admin.POST("/integrations/:id/sync-products", integrationCtrl.SyncProducts)
	}
	auth.POST("/integration/order-webhook/:platform", integrationCtrl.ReceiveOrderWebhook)
}

// setupCommunityRoutes 社群管理路由
//
// 权限分级（2026-08-18 三轮发现）：写操作（Create/Update/Delete/AddMember/RemoveMember/Import）admin only
// 防 staff 误删社群 / 拉人入群。
func setupCommunityRoutes(auth *gin.RouterGroup) {
	communityCtrl := controller.NewCommunityController()
	auth.GET("/community/groups", communityCtrl.GetGroups)
	auth.GET("/community/groups/:id", communityCtrl.GetGroupByID)
	auth.GET("/community/members", communityCtrl.GetMembers)
	auth.GET("/community/members/:id", communityCtrl.GetMemberByID)
	auth.PUT("/community/members/:id", communityCtrl.UpdateMember)
	auth.GET("/community/messages", communityCtrl.GetMessages)
	auth.GET("/community/stats", communityCtrl.GetStatistics)
	admin := auth.Group("/community", middleware.AdminAuthMiddleware())
	{
		admin.POST("/groups", communityCtrl.CreateGroup)
		admin.PUT("/groups/:id", communityCtrl.UpdateGroup)
		admin.DELETE("/groups/:id", communityCtrl.DeleteGroup)
		admin.POST("/members", communityCtrl.AddMember)
		admin.DELETE("/members/:id", communityCtrl.RemoveMember)
		admin.POST("/import", communityCtrl.ImportData)
	}
	auth.POST("/community/export", communityCtrl.ExportData)
}


// setupEventRoutes 客户事件追踪(CDP)路由
// 提供 8 个事件端点 + 历史查询/统计
func setupEventRoutes(auth *gin.RouterGroup) {
	eventCtrl := controller.NewCustomerEventController()
	auth.POST("/events/track", eventCtrl.TrackEvent)
	auth.GET("/events/customer/:id", eventCtrl.GetEventHistory)
	auth.DELETE("/events/customer/:id", eventCtrl.DeleteEvent)
	auth.GET("/events/stats", eventCtrl.GetEventStats)
	auth.POST("/events/pageview", eventCtrl.TrackPageView)
	auth.POST("/events/click", eventCtrl.TrackClick)
	auth.POST("/events/purchase", eventCtrl.TrackPurchase)
	auth.POST("/events/signup", eventCtrl.TrackSignup)
	auth.POST("/events/login", eventCtrl.TrackLogin)
	auth.POST("/events/add-to-cart", eventCtrl.TrackAddToCart)
}

