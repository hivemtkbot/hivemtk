package router

import (
	contentctrl "hivemtk-user/internal/content/controller"
	"hivemtk-user/internal/controller"
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
func setupAIContentRoutes(auth *gin.RouterGroup) {
	aiContentCtrl := contentctrl.NewAIContentController()
	auth.POST("/ai/generate", aiContentCtrl.GenerateContent)
	auth.POST("/ai/history", aiContentCtrl.CreateHistory)
	auth.GET("/ai/history", aiContentCtrl.GetGenerationHistory)
	auth.GET("/ai/history/:id", aiContentCtrl.GetRecordByID)
	auth.POST("/ai/history/:id/save", aiContentCtrl.SaveRecord)
	auth.POST("/ai/history/:id/favorite", aiContentCtrl.FavoriteRecord)
	auth.POST("/ai/history/:id/rate", aiContentCtrl.RateRecord)
	auth.DELETE("/ai/history/:id", aiContentCtrl.DeleteRecord)
	auth.GET("/ai/templates", aiContentCtrl.GetTemplates)
	auth.GET("/ai/templates/:id", aiContentCtrl.GetTemplateByID)
	auth.POST("/ai/templates", aiContentCtrl.CreateTemplate)
	auth.PUT("/ai/templates/:id", aiContentCtrl.UpdateTemplate)
	auth.DELETE("/ai/templates/:id", aiContentCtrl.DeleteTemplate)
	auth.GET("/ai/template-types", aiContentCtrl.GetTemplateTypes)
}

// setupUserSegmentRoutes 用户分层 RFM 路由
func setupUserSegmentRoutes(auth *gin.RouterGroup) {
	userSegmentCtrl := controller.NewUserSegmentController()
	auth.GET("/user-segment/rfm/rule", userSegmentCtrl.GetRFMRule)
	auth.GET("/user-segment/rfm/rules", userSegmentCtrl.ListRFMRules)
	auth.POST("/user-segment/rfm/rule", userSegmentCtrl.SaveRFMRule)
	auth.PUT("/user-segment/rfm/rule/:id", userSegmentCtrl.UpdateRFMRule)
	auth.DELETE("/user-segment/rfm/rule/:id", userSegmentCtrl.DeleteRFMRule)
	auth.GET("/user-segment/rfm/list", userSegmentCtrl.GetRFMList)
	auth.GET("/user-segment/rfm/user", userSegmentCtrl.GetUserRFM)
	auth.GET("/user-segment/rfm/stats", userSegmentCtrl.GetRFMStats)
	auth.POST("/user-segment/rfm/calculate", userSegmentCtrl.CalculateRFM)
	auth.GET("/user-segment/layers", userSegmentCtrl.GetLayerDescription)
}

// setupMarketingFlowRoutes 营销自动化流程路由
func setupMarketingFlowRoutes(auth *gin.RouterGroup) {
	marketingFlowCtrl := contentctrl.NewMarketingFlowController()
	auth.GET("/marketing-flows", marketingFlowCtrl.GetFlowList)
	auth.GET("/marketing-flows/:id", marketingFlowCtrl.GetFlowByID)
	auth.POST("/marketing-flows", marketingFlowCtrl.CreateFlow)
	auth.PUT("/marketing-flows/:id", marketingFlowCtrl.UpdateFlow)
	auth.DELETE("/marketing-flows/:id", marketingFlowCtrl.DeleteFlow)
	auth.POST("/marketing-flows/:id/activate", marketingFlowCtrl.ActivateFlow)
	auth.POST("/marketing-flows/:id/pause", marketingFlowCtrl.PauseFlow)
	auth.POST("/marketing-flows/:id/stop", marketingFlowCtrl.StopFlow)
	auth.GET("/marketing-flows/:id/executions", marketingFlowCtrl.GetExecutionList)
	auth.GET("/marketing-flows/:id/stats", marketingFlowCtrl.GetExecutionStats)
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
}

// setupABTestRoutes A/B 测试路由
func setupABTestRoutes(auth *gin.RouterGroup) {
	abCtrl := opsctrl.NewABExperimentController()
	auth.GET("/ab-experiments", abCtrl.GetExperimentList)
	auth.GET("/ab-experiments/:id", abCtrl.GetExperiment)
	auth.POST("/ab-experiments", abCtrl.CreateExperiment)
	auth.PUT("/ab-experiments/:id", abCtrl.UpdateExperiment)
	auth.DELETE("/ab-experiments/:id", abCtrl.DeleteExperiment)
	auth.POST("/ab-experiments/:id/start", abCtrl.StartExperiment)
	auth.POST("/ab-experiments/:id/pause", abCtrl.PauseExperiment)
	auth.POST("/ab-experiments/:id/stop", abCtrl.StopExperiment)
	auth.GET("/ab-experiments/:id/results", abCtrl.GetExperimentResults)
	auth.GET("/ab-experiments/:id/conversion-events", abCtrl.GetConversionEvents)
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
func setupIntegrationRoutes(auth *gin.RouterGroup) {
	integrationCtrl := controller.NewIntegrationController()
	auth.GET("/integrations", integrationCtrl.GetAccountList)
	auth.GET("/integrations/:id", integrationCtrl.GetAccountByID)
	auth.POST("/integrations", integrationCtrl.CreateAccount)
	auth.PUT("/integrations/:id", integrationCtrl.UpdateAccount)
	auth.DELETE("/integrations/:id", integrationCtrl.DeleteAccount)
	auth.POST("/integrations/:id/test", integrationCtrl.TestIntegration)
	auth.POST("/integrations/:id/sync-customers", integrationCtrl.SyncCustomers)
	auth.POST("/integrations/:id/sync-products", integrationCtrl.SyncProducts)
	auth.GET("/integration/sync-logs", integrationCtrl.GetSyncLogs)
	auth.GET("/integration/external-customers", integrationCtrl.GetExternalCustomers)
	auth.GET("/integration/external-products", integrationCtrl.GetExternalProducts)
	auth.GET("/integration/external-orders", integrationCtrl.GetExternalOrders)
	auth.GET("/integration/external-orders-by-customer", integrationCtrl.GetExternalOrdersByCustomer)
	auth.POST("/integration/order-webhook/:platform", integrationCtrl.ReceiveOrderWebhook)
}

// setupCommunityRoutes 社群管理路由
func setupCommunityRoutes(auth *gin.RouterGroup) {
	communityCtrl := controller.NewCommunityController()
	auth.GET("/community/groups", communityCtrl.GetGroups)
	auth.GET("/community/groups/:id", communityCtrl.GetGroupByID)
	auth.POST("/community/groups", communityCtrl.CreateGroup)
	auth.PUT("/community/groups/:id", communityCtrl.UpdateGroup)
	auth.DELETE("/community/groups/:id", communityCtrl.DeleteGroup)
	auth.GET("/community/members", communityCtrl.GetMembers)
	auth.POST("/community/members", communityCtrl.AddMember)
	auth.GET("/community/members/:id", communityCtrl.GetMemberByID)
	auth.PUT("/community/members/:id", communityCtrl.UpdateMember)
	auth.DELETE("/community/members/:id", communityCtrl.RemoveMember)
	auth.GET("/community/messages", communityCtrl.GetMessages)
	auth.GET("/community/stats", communityCtrl.GetStatistics)
	auth.POST("/community/import", communityCtrl.ImportData)
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

