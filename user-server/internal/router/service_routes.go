package router

import (
	"context"
	"os"
	"time"

	"hivemtk-user/internal/app"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	opsctrl "hivemtk-user/internal/ops/controller"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/translation"
	"hivemtk-user/internal/websocket"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupCustomerServiceRoutes 客服会话管理路由
//
// 传入 aiAgentSvc 以满足 agent_status controller 装配（控制器零 db 引用）。
// 传入 langResolver 注入到坐席 WebSocket handler。
func setupCustomerServiceRoutes(auth *gin.RouterGroup, aiAgentSvc *service.AIAgentService, langResolver *translation.LangConfigResolver) {
	customerSessionCtrl := controller.NewCustomerSessionController()
	auth.GET("/customer-sessions", customerSessionCtrl.GetSessions)
	auth.GET("/customer-sessions/pending", customerSessionCtrl.GetPendingSessions)
	auth.POST("/customer-sessions", customerSessionCtrl.CreateSession)
	auth.POST("/customer-sessions/assign", customerSessionCtrl.AssignSession)
	auth.GET("/customer-sessions/:id/messages", customerSessionCtrl.GetMessages)
	auth.POST("/customer-sessions/:id/messages", customerSessionCtrl.SendMessage)
	auth.POST("/customer-sessions/:id/auto-assign", customerSessionCtrl.AutoAssignSession)
	auth.POST("/customer-sessions/:id/close", customerSessionCtrl.CloseSession)
	auth.POST("/customer-sessions/:id/rate", customerSessionCtrl.RateSession)
	auth.POST("/customer-sessions/:id/transfer", customerSessionCtrl.TransferSession)
	auth.POST("/customer-sessions/:id/tags", customerSessionCtrl.TagSession)
	auth.POST("/customer-sessions/:id/takeover", customerSessionCtrl.Takeover)
	auth.POST("/customer-sessions/:id/release", customerSessionCtrl.Release)
	auth.POST("/customer-sessions/:id/switch-handler", customerSessionCtrl.SwitchHandler)
	auth.GET("/customer-sessions/blacklist", customerSessionCtrl.ListActiveBlacklist)
	auth.GET("/customer-sessions/blacklist/check", customerSessionCtrl.IsUserBlacklisted)
	auth.POST("/customer-sessions/blacklist/remove", customerSessionCtrl.Unblacklist)
	auth.POST("/customer-sessions/:id/blacklist", customerSessionCtrl.Blacklist)
	auth.PUT("/customer-sessions/:id/status", customerSessionCtrl.UpdateSessionStatus)
	auth.GET("/customer-sessions/:id", customerSessionCtrl.GetSessionByID)

	agentStatusCtrl := controller.NewAgentStatusController(aiAgentSvc)
	auth.POST("/agents", agentStatusCtrl.CreateAgent)
	auth.GET("/agents/me", agentStatusCtrl.GetMyAgent)
	auth.GET("/agents/all", agentStatusCtrl.ListAllAgents)
	auth.GET("/agents/online", agentStatusCtrl.GetOnlineAgents)
	auth.GET("/agents/:id", agentStatusCtrl.GetAgentStatus)
	auth.PUT("/agents/:id/status", agentStatusCtrl.UpdateAgentStatus)
	auth.POST("/agents/:id/online", agentStatusCtrl.GoOnline)
	auth.POST("/agents/:id/offline", agentStatusCtrl.GoOffline)
	auth.GET("/agents/:id/sessions", agentStatusCtrl.GetAgentSessions)

	quickReplyCtrl := controller.NewQuickReplyController()
	auth.GET("/quick-replies", quickReplyCtrl.GetReplies)
	auth.GET("/quick-replies/categories", quickReplyCtrl.GetReplyCategories)
	auth.POST("/quick-replies", quickReplyCtrl.CreateReply)
	auth.PUT("/quick-replies/:id", quickReplyCtrl.UpdateReply)
	auth.DELETE("/quick-replies/:id", quickReplyCtrl.DeleteReply)

	sessionTagCtrl := controller.NewSessionTagController()
	auth.GET("/session-tags", sessionTagCtrl.GetTags)
	auth.POST("/session-tags", sessionTagCtrl.CreateTag)
	auth.PUT("/session-tags/:id", sessionTagCtrl.UpdateTag)
	auth.DELETE("/session-tags/:id", sessionTagCtrl.DeleteTag)

	aiSuggestionCtrl := controller.NewAISuggestionController()
	auth.GET("/ai-suggestions/:session_id", aiSuggestionCtrl.GetSuggestions)
	auth.POST("/ai-suggestions/:id/use", aiSuggestionCtrl.UseSuggestion)

	wsHandler := websocket.NewWSHandler()
	wsHandler.SetLangResolver(langResolver)
	auth.GET("/ws/agent", wsHandler.HandleWebSocket)


	customer360Ctrl := controller.NewCustomer360Controller()
	auth.GET("/customer-360", customer360Ctrl.GetCustomer360)
	auth.GET("/customer-360/list", customer360Ctrl.GetCustomerList)
	auth.GET("/customer-360/basic", customer360Ctrl.GetCustomerBasicInfo)
	auth.GET("/customer-360/stats", customer360Ctrl.GetCustomerStats)
	auth.GET("/customer-360/sessions", customer360Ctrl.GetCustomerSessions)
	auth.GET("/customer-360/messages", customer360Ctrl.GetCustomerMessages)
	auth.GET("/customer-360/events", customer360Ctrl.GetCustomerEvents)
	auth.GET("/customer-360/orders", customer360Ctrl.GetCustomerOrders)
	auth.PUT("/customer-360/tags", customer360Ctrl.UpdateCustomerTags)
	auth.GET("/customer-360/tags", customer360Ctrl.GetCustomerTags)

	customerCtrl := controller.NewCustomerController()
	auth.POST("/customer", customerCtrl.CreateCustomer)
	auth.GET("/customer/list", customer360Ctrl.GetCustomerList)
	auth.GET("/customer/360/:id", customer360Ctrl.GetCustomer360ByID)
	auth.GET("/customer/:id", customer360Ctrl.GetCustomerDetail)
	auth.GET("/customer/:id/behaviors", customer360Ctrl.GetCustomerBehaviors)
	auth.GET("/customer/:id/communications", customer360Ctrl.GetCustomerCommunications)
	auth.PUT("/customer/:id", customer360Ctrl.UpdateCustomer)
	auth.POST("/customer/:id/tags", customer360Ctrl.AddCustomerTag)
	auth.DELETE("/customer/:id/tags/:tag", customer360Ctrl.RemoveCustomerTag)

	auth.GET("/customer-360/tag-rules", customer360Ctrl.ListTagRules)
	auth.POST("/customer-360/tag-rules", customer360Ctrl.SaveTagRule)
	auth.PUT("/customer-360/tag-rules/:id", customer360Ctrl.UpdateTagRule)
	auth.DELETE("/customer-360/tag-rules/:id", customer360Ctrl.DeleteTagRule)
	auth.GET("/customer-360/tag-stats", customer360Ctrl.GetTagStats)

	oneIDCtrl := controller.NewCustomerOneIDController()
	auth.GET("/customer/oneid/list", oneIDCtrl.ListOneID)
	auth.GET("/customer/oneid/stats", oneIDCtrl.OneIDStats)
	auth.GET("/customer/oneid/conflicts", oneIDCtrl.ListConflicts)
	auth.POST("/customer/oneid/merge", oneIDCtrl.MergeIdentity)
	auth.POST("/customer/oneid/resolve", oneIDCtrl.ResolveIdentity)
	auth.POST("/customer/oneid/conflicts/:id/resolve", oneIDCtrl.ResolveConflict)
	auth.GET("/customer-oneid/:customer_id/identities", oneIDCtrl.GetIdentityMappings)
	auth.POST("/customer-oneid/:customer_id/identities", oneIDCtrl.LinkIdentity)

	auth.GET("/customer/session/list", customerSessionCtrl.GetSessions)
	auth.POST("/customer/session", customerSessionCtrl.CreateSession)
	auth.POST("/customer/session/messages", customerSessionCtrl.SendMessage)
	auth.GET("/customer/session/:id/messages", customerSessionCtrl.GetMessages)
	auth.POST("/customer/session/:id/close", customerSessionCtrl.CloseSession)
	auth.POST("/customer/session/:id/transfer", customerSessionCtrl.TransferSession)

	appConfigCtrl := controller.NewAppConfigController()
	auth.GET("/app-config", appConfigCtrl.GetAppConfig)
	auth.PUT("/app-config", appConfigCtrl.UpdateAppConfig)
	auth.POST("/app-config/sync", appConfigCtrl.SyncWithPlatform)
	auth.GET("/app-config/health", appConfigCtrl.HealthCheck)
}

// setupMessageRoutes 统一消息管理路由
func setupMessageRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	unifiedMsgCtrl := controller.NewUnifiedMessageController()
	auth.GET("/messages", unifiedMsgCtrl.GetMessages)
	auth.GET("/messages/:id", unifiedMsgCtrl.GetMessageByID)

	messageHubCtrl := controller.NewMessageHubController(service.NewMessageHubService())
	auth.POST("/message-hub/push", messageHubCtrl.Push)
	auth.POST("/message-hub/push-batch", messageHubCtrl.PushBatch)
	auth.POST("/message-hub/push-from-channel", messageHubCtrl.PushFromChannel)
	auth.GET("/message-hub/list", messageHubCtrl.List)
	auth.GET("/message-hub/stats", messageHubCtrl.Stats)
	auth.GET("/message-hub/platforms", messageHubCtrl.Platforms)
	auth.GET("/message-hub/:id", messageHubCtrl.GetByID)
	auth.POST("/message-hub/:id/read", messageHubCtrl.MarkRead)

	inboxCtrl := controller.NewInboxController(service.NewInboxService())
	auth.GET("/inbox", inboxCtrl.List)
	auth.GET("/inbox/stats", inboxCtrl.Stats)
	auth.POST("/inbox/reconcile", inboxCtrl.Reconcile)
	auth.GET("/inbox/assignments", inboxCtrl.ListAssignments)
	auth.POST("/inbox/assign", inboxCtrl.Assign)
	auth.POST("/inbox/auto-assign", inboxCtrl.AutoAssign)
	auth.GET("/inbox/staff/:staff/load", inboxCtrl.StaffLoad)
	auth.GET("/inbox/:id", inboxCtrl.GetByID)
	auth.POST("/inbox/:id/read", inboxCtrl.MarkRead)
	auth.POST("/inbox/:id/pin", inboxCtrl.Pin)
	auth.POST("/inbox/:id/star", inboxCtrl.Star)
	auth.POST("/inbox/:id/mute", inboxCtrl.Mute)
	auth.POST("/inbox/:id/tags", inboxCtrl.AddTag)
	auth.DELETE("/inbox/:id/tags/:tag", inboxCtrl.RemoveTag)
	auth.GET("/inbox/:id/messages", inboxCtrl.GetMessages)
	auth.DELETE("/inbox/:id/messages/:mid", inboxCtrl.DeleteMessage)

	inboxIngressCtrl := controller.NewInboxIngressController(service.NewInboxIngressService())
	auth.POST("/inbox/lock-human", inboxIngressCtrl.LockHuman)
	auth.POST("/inbox/unlock-human/:session_id", inboxIngressCtrl.UnlockHuman)
}

// setupPlatformAccountRoutes 平台账号管理路由
func setupPlatformAccountRoutes(auth *gin.RouterGroup) {
	platformAccountCtrl := controller.NewPlatformAccountController()
	auth.GET("/platform-accounts", platformAccountCtrl.GetAccounts)
	auth.GET("/platform-accounts/platforms", platformAccountCtrl.GetSupportedPlatforms)
	auth.GET("/platform-accounts/:id", platformAccountCtrl.GetAccountByID)
	auth.POST("/platform-accounts", platformAccountCtrl.CreateAccount)
	auth.PUT("/platform-accounts/:id", platformAccountCtrl.UpdateAccount)
	auth.DELETE("/platform-accounts/:id", platformAccountCtrl.DeleteAccount)
	auth.POST("/platform-accounts/:id/login", platformAccountCtrl.LoginAccount)
	auth.GET("/platform-accounts/:id/status", platformAccountCtrl.CheckLoginStatus)
}

// setupWeComHealthRoutes 企微账号健康度路由
func setupWeComHealthRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	wcHCtrl := controller.NewWeComHealthController(service.NewWeComAccountHealthService(db), service.NewWeComIntegrationService(db))
	auth.GET("/wecom/health/accounts", wcHCtrl.ListAccountsWithHealth)
	auth.GET("/wecom/health/accounts/risks", wcHCtrl.GetRiskAccounts)
	auth.GET("/wecom/health/accounts/select", wcHCtrl.SelectHealthyAccount)
	auth.GET("/wecom/health/accounts/summary", wcHCtrl.GetHealthSummary)
	auth.GET("/wecom/health/accounts/:id", wcHCtrl.GetLatestHealth)
	auth.GET("/wecom/health/accounts/:id/history", wcHCtrl.ListHealthHistory)
	auth.POST("/wecom/health/accounts/:id", wcHCtrl.ReportHealth)
	auth.POST("/wecom/health/accounts/:id/status", wcHCtrl.UpdateAccountStatus)
	auth.POST("/wecom/health/accounts/:id/quota/consume", wcHCtrl.ConsumeQuota)
	auth.POST("/wecom/health/accounts/quota/reset", wcHCtrl.ResetDailyQuota)
	auth.POST("/wecom/messages/ingest", wcHCtrl.IngestMessage)
	auth.POST("/wecom/messages/send", wcHCtrl.SendMessage)
}

// setupIntentRoutes 意图识别路由
//
// 使用全局 IntentRecognizer（main.go 中 InitIntentRecognizer 初始化），
// 复用 dispatcher/db,避免每个请求重建对象。
func setupIntentRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	rec := service.GetIntentRecognizer()
	if rec == nil {
		rec = service.NewIntentRecognizer(db, app.GetGlobalDispatcher(), nil)
	}
	intentCtrl := controller.NewIntentController(rec)
	auth.POST("/intent/recognize", intentCtrl.Recognize)
	auth.POST("/intent/recognize/batch", intentCtrl.BatchRecognize)
	auth.GET("/intent/stats", intentCtrl.Stats)
	auth.GET("/intent/recent", intentCtrl.RecentIntents)
	auth.GET("/intent/dict", intentCtrl.Intents)
	auth.POST("/intent/recognize/fine", intentCtrl.RecognizeFine)
	auth.GET("/intent/logs", intentCtrl.IntentLogs)
	auth.GET("/intent/stats/fine", intentCtrl.IntentStatsFine)
	auth.GET("/intent/config", intentCtrl.GetConfig)
	auth.PUT("/intent/config", intentCtrl.UpdateConfig)
}

// setupLLMProviderRoutes LLM Provider 降级管理路由（）
//
// failover 为 nil 时使用占位 controller（所有端点返回 503）。
// 生产环境 main.go 应通过 NewSetupLLMProviderRoutes 注入真实 failover。
//
// 权限分级（2026-08-18）：写操作（reset circuit / update policy）必须 admin
// 防止 staff 误操作 / 恶意绕过熔断器造成服务不可用。
func setupLLMProviderRoutes(auth *gin.RouterGroup) {
	failoverSvc := service.NewLLMFailoverService(app.GetGlobalProviderFailover())
	llmProvCtrl := controller.NewLLMProviderController(failoverSvc)
	// 写操作：admin only
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	admin.POST("/llm/providers/circuit/reset", llmProvCtrl.ResetCircuit)
	admin.POST("/llm/providers/circuit/reset/:provider", llmProvCtrl.ResetCircuit)
	admin.PUT("/llm/providers/policy", llmProvCtrl.UpdatePolicy)
	admin.POST("/llm-routings/providers/circuit/reset", llmProvCtrl.ResetCircuit)
	admin.PUT("/llm-routings/policy", llmProvCtrl.UpdatePolicy)
	// 读操作：任意登录用户（监控面板需要）
	auth.GET("/llm/providers/health", llmProvCtrl.GetHealth)
	auth.GET("/llm/providers/health/:provider", llmProvCtrl.GetProviderHealth)
	auth.GET("/llm/providers/policy", llmProvCtrl.GetPolicy)
	auth.GET("/llm-routings/providers/health", llmProvCtrl.GetHealth)
	auth.GET("/llm-routings/providers/:provider/health", llmProvCtrl.GetProviderHealth)
	auth.GET("/llm-routings/policy", llmProvCtrl.GetPolicy)
	// resolve 是只读路径解析（业务侧 chat/sop 会调），不需要 admin
	auth.POST("/llm-routings/resolve", llmProvCtrl.ResolveRoute)
}

// setupTraceRoutes 全链路追踪路由（）
//
// 路由顺序重要：/traces/recent 必须在 /trace/:traceId 之前注册，
// 否则 gin 路由树会把 "/traces/recent" 匹配为 /trace/:traceId（traceId="s/recent"）。
func setupTraceRoutes(auth *gin.RouterGroup) {
	traceCtrl := controller.NewTraceController()
	auth.GET("/traces/recent", traceCtrl.ListRecent)
	auth.GET("/trace/:traceId", traceCtrl.GetTrace)
}

// setupSSEDashboardRoutes SSE 实时驾驶舱路由（）
func setupSSEDashboardRoutes(auth *gin.RouterGroup) {
	sseCtrl := controller.NewSSEDashboardController()
	auth.GET("/dashboard/sse", sseCtrl.Stream)
	auth.GET("/dashboard/clients", sseCtrl.ListClients)
	auth.GET("/dashboard/topics", sseCtrl.ListTopics)
	auth.GET("/dashboard/stats", sseCtrl.Stats)
	auth.POST("/dashboard/broadcast", sseCtrl.Broadcast)
}

// setupDialogueMemoryRoutes 对话记忆路由
func setupDialogueMemoryRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	memCtrl := controller.NewDialogueMemoryController(service.NewDialogueMemoryService(db, nil))
	auth.POST("/memory/messages", memCtrl.AppendMessage)
	auth.GET("/memory/short", memCtrl.ShortTerm)
	auth.GET("/memory/long", memCtrl.LongTerm)
	auth.POST("/memory/facts", memCtrl.UpdateKeyFacts)
	auth.POST("/memory/objections", memCtrl.RecordObjection)
	auth.POST("/memory/purchase-intent", memCtrl.UpdatePurchaseIntent)
	auth.POST("/memory/intent-trail", memCtrl.RecordIntent)
	auth.POST("/memory/sop-history", memCtrl.RecordSOP)
	auth.GET("/memory/context", memCtrl.BuildContext)
	auth.GET("/memory/list", memCtrl.Stats)
}

// setupSOPRoutes SOP 智能体路由
//
// 权限分级（2026-08-18 三轮发现）：写操作（Create/Update/Delete/Activate/Pause/Resume/Cancel/Execute/Step/ABTest）admin only
// SOP 是一线客服自动应答流程，被 staff 误操作会立即影响客户对话体验。
func setupSOPRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	sopCtrl := controller.NewSOPController(service.NewSOPService(db, nil))
	// 读操作
	auth.GET("/sop", sopCtrl.List)
	auth.GET("/sop/stats", sopCtrl.Stats)
	auth.GET("/sop/match", sopCtrl.MatchByIntent)
	auth.GET("/sop/executions", sopCtrl.ListExecutions)
	auth.GET("/sop/executions/:id", sopCtrl.GetExecution)
	auth.GET("/sop/:id", sopCtrl.Get)
	auth.GET("/sop/:id/abtest/stats", sopCtrl.GetABTestStats)
	// 写操作：admin only
	admin := auth.Group("/sop", middleware.AdminAuthMiddleware())
	{
		admin.POST("", sopCtrl.Create)
		admin.PUT("/:id", sopCtrl.Update)
		admin.DELETE("/:id", sopCtrl.Delete)
		admin.POST("/:id/activate", sopCtrl.Activate)
		admin.POST("/:id/deactivate", sopCtrl.Deactivate)
		admin.POST("/executions/:id/pause", sopCtrl.Pause)
		admin.POST("/executions/:id/resume", sopCtrl.Resume)
		admin.POST("/executions/:id/cancel", sopCtrl.Cancel)
		admin.PUT("/:id/abtest/config", sopCtrl.UpdateABTestConfig)
		admin.POST("/execute", sopCtrl.Execute)
		admin.POST("/step", sopCtrl.Step)
	}

	faqCtrl := controller.NewFAQController()
	faqCtrl.RegisterRoutes(auth)

	sopTplCtrl := controller.NewSOPTemplateController()
	sopTplCtrl.RegisterRoutes(auth)

	kbCtrl := controller.NewKnowledgeBaseController()
	kbCtrl.RegisterRoutes(auth)

	bindCtrl := controller.NewAgentKBBindingController()
	bindCtrl.RegisterRoutes(auth)
}

// setupReachPipelineRoutes 触达 Pipeline 路由
func setupReachPipelineRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	reachSvc := service.NewReachPipelineService(db)
	if hook := service.NewHTTPAlertHook(os.Getenv("ALERT_WEBHOOK_URL")); hook != nil {
		reachSvc.SetAlertHook(hook)
	}
	if sender := app.NewPipelineReachSender(db); sender != nil {
		reachSvc.SetReachSender(sender)
	}
	// 2026-08-16 严肃化：把全渠道 service 注册到 tooluse GlobalServiceRegistry，
	// 否则 AI Agent 触发 reach.*.send 全部走 NoOp 路径。
	app.RegisterAllReachServices(db)
	reachSvc.StartDispatcher(context.Background(), 15*time.Second)
	reachCtrl := controller.NewReachPipelineController(reachSvc)
	auth.GET("/reach/pipelines", reachCtrl.ListPipelines)
	auth.POST("/reach/pipelines", reachCtrl.CreatePipeline)
	auth.GET("/reach/stats", reachCtrl.Stats)
	auth.GET("/reach/jobs", reachCtrl.ListJobs)
	auth.POST("/reach/jobs", reachCtrl.EnqueueJob)
	auth.POST("/reach/rate-limit/reset", reachCtrl.ResetRateLimit)
	auth.GET("/reach/pipelines/:id", reachCtrl.GetPipeline)
	auth.PUT("/reach/pipelines/:id", reachCtrl.UpdatePipeline)
	auth.DELETE("/reach/pipelines/:id", reachCtrl.DeletePipeline)
	auth.POST("/reach/pipelines/:id/pause", reachCtrl.PausePipeline)
	auth.POST("/reach/pipelines/:id/resume", reachCtrl.ResumePipeline)
	auth.POST("/reach/pipelines/:id/archive", reachCtrl.ArchivePipeline)
	auth.GET("/reach/jobs/:id", reachCtrl.GetJob)
	auth.POST("/reach/jobs/:id/cancel", reachCtrl.CancelJob)
	auth.POST("/reach/jobs/:id/retry", reachCtrl.RetryJob)
	auth.POST("/reach/jobs/:id/execute", reachCtrl.ExecuteJob)
}

// setupProactiveReachRoutes 主动触达路由
func setupProactiveReachRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	reachSvc := service.NewReachPipelineService(db)
	proactiveSvc := service.NewProactiveReachService(db, nil)
	service.BindProactiveReachSenders(proactiveSvc, db)
	proactiveCtrl := controller.NewProactiveReachController(proactiveSvc)

	// 主动触达核心 API
	auth.POST("/reach/proactive/send", proactiveCtrl.ProactiveSend)
	auth.POST("/reach/proactive/quick", proactiveCtrl.QuickSend)
	auth.POST("/reach/proactive/validate", proactiveCtrl.ValidateReach)

	// 客户维度触达（按 OneID 智能选渠道）
	auth.POST("/reach/proactive/customer/:customer_id", proactiveCtrl.ProactiveSendFromCustomer)
	auth.GET("/reach/proactive/customer/:customer_id/channels", proactiveCtrl.ListChannels)

	// 批量触达
	auth.POST("/reach/proactive/batch", proactiveCtrl.BatchProactiveSend)

	_ = reachSvc
}

// setupLLMRoutingRoutes LLM 多模型路由
//
// 权限分级（2026-08-18 多角度审计修复）：
//   - 写操作（Create/Update/Delete/Test/Strategies）必须 admin（防 staff 注入恶意 base_url
//     把全公司 LLM 流量重定向到 evil.com，窃取全部对话数据）
//   - 读操作（ListModels/GetModel/ListStrategies/Stats/Health 等）任意登录用户可访问（业务需要）
func setupLLMRoutingRoutes(auth *gin.RouterGroup) {
	dispatcher := app.GetGlobalDispatcher()
	routingService := service.NewLLMRoutingService(dispatcher)
	llmCtrl := controller.NewLLMRoutingController(routingService)
	if f := app.GetGlobalProviderFailover(); f != nil {
		llmCtrl.SetFailover(service.NewLLMFailoverService(f))
	}
	// 写操作：admin only（防 LLM 流量重定向 / 注入 / 熔断绕过）
	admin := auth.Group("/llm", middleware.AdminAuthMiddleware())
	{
		admin.POST("/models", llmCtrl.CreateModel)
		admin.PUT("/models/:name", llmCtrl.UpdateModel)
		admin.DELETE("/models/:name", llmCtrl.DeleteModel)
		admin.POST("/models/:name/test", llmCtrl.TestModel)
		admin.PUT("/strategies", llmCtrl.UpdateStrategies)
		admin.POST("/strategies", llmCtrl.UpdateStrategies)
		admin.PUT("/scene-routing", llmCtrl.UpdateSceneRouting)
		admin.POST("/scene-routing", llmCtrl.UpdateSceneRouting)
		admin.PUT("/fallback", llmCtrl.UpdateSceneRouting)
		admin.POST("/fallback", llmCtrl.UpdateSceneRouting)
	}
	// 读操作：任意登录用户（业务展示需要）
	auth.GET("/llm/models", llmCtrl.ListModels)
	auth.GET("/llm/models/:name", llmCtrl.GetModel)
	auth.GET("/llm/strategies", llmCtrl.ListStrategies)
	auth.GET("/llm/audit", llmCtrl.ListAuditHistory)
	auth.GET("/llm/stats", llmCtrl.Stats)
	auth.GET("/llm/usage", llmCtrl.Usage)
	auth.GET("/llm/cost-stats", llmCtrl.CostStats)
	auth.GET("/llm/fallback", llmCtrl.FallbackConfig)
	auth.GET("/llm/scene-routing", llmCtrl.ListStrategies)
	auth.GET("/llm/scenarios", llmCtrl.ListScenarios)
	auth.GET("/llm/health", llmCtrl.Health)
	auth.GET("/llm/scenario-stats", llmCtrl.ScenarioStats)
	auth.GET("/llm/model-type-stats", llmCtrl.ModelTypeStats)
	auth.GET("/llm/egress-alerts", llmCtrl.EgressAlerts)
	auth.GET("/llm/egress-audit", llmCtrl.EgressAudit)
}

// setupAnalyticsRoutes 数据分析 (转化漏斗 + AI 产能 + 销冠画像)
func setupAnalyticsRoutes(auth *gin.RouterGroup) {
	funnelCtrl := opsctrl.NewConversionFunnelController()
	auth.GET("/analytics/funnel", funnelCtrl.GetFunnel)
	auth.GET("/analytics/funnel/stage", funnelCtrl.GetStageDetails)

	aiProdCtrl := opsctrl.NewAIProductivityController()
	auth.GET("/analytics/ai-productivity", aiProdCtrl.GetReport)
	auth.GET("/analytics/ai-productivity/trend", aiProdCtrl.GetDailyTrend)

	personaCtrl := controller.NewSalesPersonaController()
	auth.GET("/analytics/persona/staffs", personaCtrl.ListStaffs)
	auth.GET("/analytics/persona/staffs/:id", personaCtrl.GetReport)
}

// setupObjectionHandlerRoutes 异议处理
func setupObjectionHandlerRoutes(auth *gin.RouterGroup) {
	objCtrl := controller.NewObjectionHandlerController()
	auth.POST("/objection/handle", objCtrl.Handle)
	auth.POST("/objection/classify", objCtrl.Classify)
	auth.GET("/objection/categories", objCtrl.ListCategories)
	auth.POST("/objection/usage", objCtrl.RecordUsage)
}

// setupCustomerJourneyRoutes 客户旅程大屏 (G10)
func setupCustomerJourneyRoutes(auth *gin.RouterGroup) {
	journeyCtrl := controller.NewCustomerJourneyController()
	auth.GET("/customer-journey/overview", journeyCtrl.GetOverview)
	auth.GET("/customer-journey/stages", journeyCtrl.ListStages)
	auth.GET("/customer-journey/by-stage", journeyCtrl.ListByStage)
	auth.POST("/customer-journey/transition", journeyCtrl.TransitionStage)
	auth.POST("/customer-journey/touch", journeyCtrl.TouchCustomer)
}

// setupQualityRoutes 性能压测 + 安全审计
func setupQualityRoutes(auth *gin.RouterGroup) {
	perfCtrl := opsctrl.NewPerformanceTestController()
	auth.POST("/perf/run", perfCtrl.RunTest)
	auth.GET("/perf/list", perfCtrl.ListResults)
	auth.GET("/perf/:id", perfCtrl.GetResult)
}

// setupSecurityAuditRoutes 安全审计：列表 / 立即审计 / 明细。
func setupSecurityAuditRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	ctrl := controller.NewSecurityAuditController(service.NewSecurityAuditService(gormDB))
	auth.GET("/security/audit/list", ctrl.ListSecurityAudits)
	auth.POST("/security/audit", ctrl.RunSecurityAudit)
	auth.GET("/security/audit/:id", ctrl.GetSecurityAudit)
}

