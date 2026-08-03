package router

import (
	"context"
	"os"
	"time"

	"marketing/internal/bridge"
	"marketing/internal/controller"
	opsctrl "marketing/internal/ops/controller"
	"marketing/internal/service"
	i18nservice "marketing/internal/service/i18n"
	"marketing/internal/websocket"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// bridgeIngressSvc 网页桥接消息入站服务（包级变量，供 Setup 在 webhookSvc 构造后注入 AITrigger）
var bridgeIngressSvc *service.InboxIngressService

// setupCustomerServiceRoutes 客服会话管理路由
//
// 传入 aiAgentSvc 以满足 agent_status controller 装配（控制器零 db 引用）。
// 传入 langResolver 注入到坐席 WebSocket handler。
func setupCustomerServiceRoutes(auth *gin.RouterGroup, aiAgentSvc *service.AIAgentService, langResolver *i18nservice.LangConfigResolver) {
	// 客服会话管理
	customerSessionCtrl := controller.NewCustomerSessionController()
	auth.GET("/customer-sessions", customerSessionCtrl.GetSessions)
	auth.GET("/customer-sessions/pending", customerSessionCtrl.GetPendingSessions)
	auth.POST("/customer-sessions", customerSessionCtrl.CreateSession)
	auth.POST("/customer-sessions/assign", customerSessionCtrl.AssignSession)
	// 注意：具体路径必须在通配符路径之前注册，统一使用:id 参数名
	auth.GET("/customer-sessions/:id/messages", customerSessionCtrl.GetMessages)
	auth.POST("/customer-sessions/:id/messages", customerSessionCtrl.SendMessage)
	auth.POST("/customer-sessions/:id/auto-assign", customerSessionCtrl.AutoAssignSession)
	auth.POST("/customer-sessions/:id/close", customerSessionCtrl.CloseSession)
	auth.POST("/customer-sessions/:id/rate", customerSessionCtrl.RateSession)
	auth.POST("/customer-sessions/:id/transfer", customerSessionCtrl.TransferSession)
	auth.POST("/customer-sessions/:id/tags", customerSessionCtrl.TagSession)
	// 方向10：坐席实时聊天看板 - AI/人工接管与切换
	auth.POST("/customer-sessions/:id/takeover", customerSessionCtrl.Takeover)
	auth.POST("/customer-sessions/:id/release", customerSessionCtrl.Release)
	auth.POST("/customer-sessions/:id/switch-handler", customerSessionCtrl.SwitchHandler)
	// 方向10：拉黑 / 解除拉黑
	// 注意：黑名单相关路由放在 :id 通配符之前，避免被通配捕获
	auth.GET("/customer-sessions/blacklist", customerSessionCtrl.ListActiveBlacklist)
	auth.GET("/customer-sessions/blacklist/check", customerSessionCtrl.IsUserBlacklisted)
	auth.POST("/customer-sessions/blacklist/remove", customerSessionCtrl.Unblacklist)
	auth.POST("/customer-sessions/:id/blacklist", customerSessionCtrl.Blacklist)
	auth.PUT("/customer-sessions/:id/status", customerSessionCtrl.UpdateSessionStatus)
	auth.GET("/customer-sessions/:id", customerSessionCtrl.GetSessionByID)

	// 客服状态管理
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

	// 快捷回复管理
	quickReplyCtrl := controller.NewQuickReplyController()
	auth.GET("/quick-replies", quickReplyCtrl.GetReplies)
	auth.GET("/quick-replies/categories", quickReplyCtrl.GetReplyCategories)
	auth.POST("/quick-replies", quickReplyCtrl.CreateReply)
	auth.PUT("/quick-replies/:id", quickReplyCtrl.UpdateReply)
	auth.DELETE("/quick-replies/:id", quickReplyCtrl.DeleteReply)

	// 会话标签管理
	sessionTagCtrl := controller.NewSessionTagController()
	auth.GET("/session-tags", sessionTagCtrl.GetTags)
	auth.POST("/session-tags", sessionTagCtrl.CreateTag)
	auth.PUT("/session-tags/:id", sessionTagCtrl.UpdateTag)
	auth.DELETE("/session-tags/:id", sessionTagCtrl.DeleteTag)

	// AI 建议管理
	aiSuggestionCtrl := controller.NewAISuggestionController()
	auth.GET("/ai-suggestions/:session_id", aiSuggestionCtrl.GetSuggestions)
	auth.POST("/ai-suggestions/:id/use", aiSuggestionCtrl.UseSuggestion)

	// WebSocket 连接
	wsHandler := websocket.NewWSHandler()
	wsHandler.SetLangResolver(langResolver)
	auth.GET("/ws/agent", wsHandler.HandleWebSocket)

	// 网页桥接（抖音/小红书/TikTok 网页私信）WebSocket：扩展经此上行私信、下行 AI 回复
	bridgeIngressSvc = service.NewInboxIngressService()
	bridgeHandler := bridge.NewBridgeWSHandler(bridge.GetBridgeHub(), bridgeIngressSvc)
	auth.GET("/ws/bridge", bridgeHandler.HandleWebSocket)

	// 客户 360 视图
	customer360Ctrl := controller.NewCustomer360Controller()
	auth.GET("/customer-360", customer360Ctrl.GetCustomer360)
	auth.GET("/customer-360/list", customer360Ctrl.GetCustomerList)
	auth.GET("/customer-360/basic", customer360Ctrl.GetCustomerBasicInfo)
	auth.GET("/customer-360/stats", customer360Ctrl.GetCustomerStats)
	auth.GET("/customer-360/sessions", customer360Ctrl.GetCustomerSessions)
	auth.GET("/customer-360/messages", customer360Ctrl.GetCustomerMessages)
	auth.PUT("/customer-360/tags", customer360Ctrl.UpdateCustomerTags)
	auth.GET("/customer-360/tags", customer360Ctrl.GetCustomerTags)

	// ===== 兼容路由：前端调用 /api/customer/* 路径格式 =====
	// 客户 360 兼容路由（前端 /api/customer/...）
	// 注意：静态路径需在参数路径之前注册
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

	// 客户 360° 标签规则与统计
	auth.GET("/customer-360/tag-rules", customer360Ctrl.ListTagRules)
	auth.POST("/customer-360/tag-rules", customer360Ctrl.SaveTagRule)
	auth.PUT("/customer-360/tag-rules/:id", customer360Ctrl.UpdateTagRule)
	auth.DELETE("/customer-360/tag-rules/:id", customer360Ctrl.DeleteTagRule)
	auth.GET("/customer-360/tag-stats", customer360Ctrl.GetTagStats)

	// ===== OneID 体系（多渠道身份归一与冲突解决） =====
	// 注意：静态路径必须在参数路径之前注册
	oneIDCtrl := controller.NewCustomerOneIDController()
	auth.GET("/customer/oneid/list", oneIDCtrl.ListOneID)
	auth.GET("/customer/oneid/stats", oneIDCtrl.OneIDStats)
	auth.GET("/customer/oneid/conflicts", oneIDCtrl.ListConflicts)
	auth.POST("/customer/oneid/merge", oneIDCtrl.MergeIdentity)
	auth.POST("/customer/oneid/resolve", oneIDCtrl.ResolveIdentity)
	auth.POST("/customer/oneid/conflicts/:id/resolve", oneIDCtrl.ResolveConflict)
	// 注意：/api/customer/:id 已占用 :id 参数,OneID 子路径需用不同的前缀以避免冲突
	auth.GET("/customer-oneid/:customer_id/identities", oneIDCtrl.GetIdentityMappings)
	auth.POST("/customer-oneid/:customer_id/identities", oneIDCtrl.LinkIdentity)

	// 客服会话兼容路由（前端 /api/customer/session/...）
	// 注意：静态路径需在参数路径之前注册
	auth.GET("/customer/session/list", customerSessionCtrl.GetSessions)
	auth.POST("/customer/session", customerSessionCtrl.CreateSession)
	auth.POST("/customer/session/messages", customerSessionCtrl.SendMessage)
	auth.GET("/customer/session/:id/messages", customerSessionCtrl.GetMessages)
	auth.POST("/customer/session/:id/close", customerSessionCtrl.CloseSession)
	auth.POST("/customer/session/:id/transfer", customerSessionCtrl.TransferSession)

	// 应用配置管理
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
	// 注意：具体路径必须在通配符路径之前注册，统一使用:id 参数名
	auth.GET("/messages/:id/replies", unifiedMsgCtrl.GetReplies)
	auth.GET("/messages/:id", unifiedMsgCtrl.GetMessageByID)

	// 消息中台 MQ
	messageHubCtrl := controller.NewMessageHubController(service.NewMessageHubService())
	auth.POST("/message-hub/push", messageHubCtrl.Push)
	auth.POST("/message-hub/push-batch", messageHubCtrl.PushBatch)
	auth.POST("/message-hub/push-from-channel", messageHubCtrl.PushFromChannel)
	auth.GET("/message-hub/list", messageHubCtrl.List)
	auth.GET("/message-hub/stats", messageHubCtrl.Stats)
	auth.GET("/message-hub/platforms", messageHubCtrl.Platforms)
	auth.GET("/message-hub/:id", messageHubCtrl.GetByID)
	auth.POST("/message-hub/:id/read", messageHubCtrl.MarkRead)

	// 统一收件箱
	inboxCtrl := controller.NewInboxController(service.NewInboxService())
	auth.GET("/inbox", inboxCtrl.List)
	auth.GET("/inbox/stats", inboxCtrl.Stats)
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

	// 企业级架构优化 - 方向 3: 渠道接入消息中台 - 人工接管控制
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
		// 兜底：若全局未初始化(单测等场景)则临时创建一个
		rec = service.NewIntentRecognizer(db, getGlobalDispatcher(), nil)
	}
	intentCtrl := controller.NewIntentController(rec)
	auth.POST("/intent/recognize", intentCtrl.Recognize)
	auth.POST("/intent/recognize/batch", intentCtrl.BatchRecognize)
	auth.GET("/intent/stats", intentCtrl.Stats)
	auth.GET("/intent/recent", intentCtrl.RecentIntents)
	auth.GET("/intent/dict", intentCtrl.Intents)
	// 精细意图识别（8 大类 + 7 子类）
	auth.POST("/intent/recognize/fine", intentCtrl.RecognizeFine)
	auth.GET("/intent/logs", intentCtrl.IntentLogs)
	auth.GET("/intent/stats/fine", intentCtrl.IntentStatsFine)
	// 意图识别配置（在线开关 + 持久化）
	auth.GET("/intent/config", intentCtrl.GetConfig)
	auth.PUT("/intent/config", intentCtrl.UpdateConfig)
}

// setupLLMProviderRoutes LLM Provider 降级管理路由（）
//
// failover 为 nil 时使用占位 controller（所有端点返回 503）。
// 生产环境 main.go 应通过 NewSetupLLMProviderRoutes 注入真实 failover。
func setupLLMProviderRoutes(auth *gin.RouterGroup) {
	failover := getGlobalProviderFailover()
	llmProvCtrl := controller.NewLLMProviderController(failover)
	auth.GET("/llm/providers/health", llmProvCtrl.GetHealth)
	auth.GET("/llm/providers/health/:provider", llmProvCtrl.GetProviderHealth)
	auth.POST("/llm/providers/circuit/reset", llmProvCtrl.ResetCircuit)
	auth.POST("/llm/providers/circuit/reset/:provider", llmProvCtrl.ResetCircuit)
	auth.GET("/llm/providers/policy", llmProvCtrl.GetPolicy)
	auth.PUT("/llm/providers/policy", llmProvCtrl.UpdatePolicy)
	// 文档承诺的 /api/llm-routings/* 端点（前端 ops/llm-routing 看板使用）
	// 统一路径参数为 :provider，与 controller GetProviderHealth 读取一致
	auth.GET("/llm-routings/providers/health", llmProvCtrl.GetHealth)
	auth.GET("/llm-routings/providers/:provider/health", llmProvCtrl.GetProviderHealth)
	auth.POST("/llm-routings/providers/circuit/reset", llmProvCtrl.ResetCircuit)
	auth.GET("/llm-routings/policy", llmProvCtrl.GetPolicy)
	auth.PUT("/llm-routings/policy", llmProvCtrl.UpdatePolicy)
	auth.POST("/llm-routings/resolve", llmProvCtrl.ResolveRoute)
}

// setupTraceRoutes 全链路追踪路由（）
func setupTraceRoutes(auth *gin.RouterGroup) {
	traceCtrl := controller.NewTraceController()
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
func setupSOPRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	sopCtrl := controller.NewSOPController(service.NewSOPService(db, nil))
	auth.GET("/sop", sopCtrl.List)
	auth.POST("/sop", sopCtrl.Create)
	auth.GET("/sop/stats", sopCtrl.Stats)
	auth.GET("/sop/match", sopCtrl.MatchByIntent)
	auth.GET("/sop/executions", sopCtrl.ListExecutions)
	auth.GET("/sop/executions/:id", sopCtrl.GetExecution)
	auth.POST("/sop/executions/:id/pause", sopCtrl.Pause)
	auth.POST("/sop/executions/:id/resume", sopCtrl.Resume)
	auth.POST("/sop/executions/:id/cancel", sopCtrl.Cancel)
	auth.GET("/sop/:id", sopCtrl.Get)
	auth.PUT("/sop/:id", sopCtrl.Update)
	auth.DELETE("/sop/:id", sopCtrl.Delete)
	auth.POST("/sop/:id/activate", sopCtrl.Activate)
	auth.POST("/sop/:id/deactivate", sopCtrl.Deactivate)
	// A/B 测试（PRD §5.2 G2 新增）
	auth.GET("/sop/:id/abtest/stats", sopCtrl.GetABTestStats)
	auth.PUT("/sop/:id/abtest/config", sopCtrl.UpdateABTestConfig)
	auth.POST("/sop/execute", sopCtrl.Execute)
	auth.POST("/sop/step", sopCtrl.Step)

	// FAQ 知识库 CRUD + Layer1 匹配
	faqCtrl := controller.NewFAQController(db)
	faqCtrl.RegisterRoutes(auth)

	// SOP 模板 CRUD + Layer1 匹配
	sopTplCtrl := controller.NewSOPTemplateController(db)
	sopTplCtrl.RegisterRoutes(auth)

	// 强 1对1: 知识库主表 CRUD + 业务级联
	kbCtrl := controller.NewKnowledgeBaseController(db)
	kbCtrl.RegisterRoutes(auth)

	// 强 1对1: 智能体 × 知识库 绑定 CRUD
	bindCtrl := controller.NewAgentKBBindingController(db)
	bindCtrl.RegisterRoutes(auth)
}

// setupReachPipelineRoutes 触达 Pipeline 路由
func setupReachPipelineRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 启动后台任务调度器：消费 reach.batch / reach.schedule 入队但未被执行的任务。
	// 进程级仅启动一次（service 内 sync.Once 保护），ctx 取消时优雅退出。
	reachSvc := service.NewReachPipelineService(db)
	// P1-8：注入触达失败告警钩子（配置了 ALERT_WEBHOOK_URL 时向该 webhook 推送告警，否则仅日志）
	if hook := service.NewHTTPAlertHook(os.Getenv("ALERT_WEBHOOK_URL")); hook != nil {
		reachSvc.SetAlertHook(hook)
	}
	// 注入真实触达发送器：连接 IntegrationReachAdapter + BridgeReachAdapter，
	// 使调度器真正下发到渠道（修复"触达调度器下发占位"缺口）。
	if sender := newPipelineReachSender(db); sender != nil {
		reachSvc.SetReachSender(sender)
	}
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

// setupLLMRoutingRoutes LLM 多模型路由
func setupLLMRoutingRoutes(auth *gin.RouterGroup) {
	dispatcher := getGlobalDispatcher()
	routingService := service.NewLLMRoutingService(dispatcher)
	llmCtrl := controller.NewLLMRoutingController(routingService)
	// 注入熔断器，Health 端点可展示 circuit_open / error_count / last_error
	if f := getGlobalProviderFailover(); f != nil {
		llmCtrl.SetFailover(f)
	}
	// Provider / Model 管理（:name 而非 :id）
	auth.GET("/llm/models", llmCtrl.ListModels)
	auth.GET("/llm/models/:name", llmCtrl.GetModel)
	auth.POST("/llm/models", llmCtrl.CreateModel)
	auth.PUT("/llm/models/:name", llmCtrl.UpdateModel)
	auth.DELETE("/llm/models/:name", llmCtrl.DeleteModel)
	auth.POST("/llm/models/:name/test", llmCtrl.TestModel)
	// 场景路由
	auth.GET("/llm/strategies", llmCtrl.ListStrategies)
	auth.PUT("/llm/strategies", llmCtrl.UpdateStrategies)
	auth.POST("/llm/strategies", llmCtrl.UpdateStrategies)
	// 路由审计
	auth.GET("/llm/audit", llmCtrl.ListAuditHistory)
	// 用量统计
	auth.GET("/llm/stats", llmCtrl.Stats)
	auth.GET("/llm/usage", llmCtrl.Usage)
	auth.GET("/llm/cost-stats", llmCtrl.CostStats)
	auth.GET("/llm/fallback", llmCtrl.FallbackConfig)
	// 兼容前端路径别名
	auth.GET("/llm/scene-routing", llmCtrl.ListStrategies)
	auth.PUT("/llm/scene-routing", llmCtrl.UpdateSceneRouting)
	auth.POST("/llm/scene-routing", llmCtrl.UpdateSceneRouting)
	auth.PUT("/llm/fallback", llmCtrl.UpdateSceneRouting)
	auth.POST("/llm/fallback", llmCtrl.UpdateSceneRouting)
	// 补全端点（E2E 完整性补齐）
	auth.GET("/llm/scenarios", llmCtrl.ListScenarios)
	auth.GET("/llm/health", llmCtrl.Health)
	auth.GET("/llm/scenario-stats", llmCtrl.ScenarioStats)
	auth.GET("/llm/model-type-stats", llmCtrl.ModelTypeStats)
	auth.GET("/llm/egress-alerts", llmCtrl.EgressAlerts)
	auth.GET("/llm/egress-audit", llmCtrl.EgressAudit)
}

// setupAnalyticsRoutes 数据分析 (转化漏斗 + AI 产能 + 销冠画像)
func setupAnalyticsRoutes(auth *gin.RouterGroup) {
	// 转化漏斗
	funnelCtrl := opsctrl.NewConversionFunnelController()
	auth.GET("/analytics/funnel", funnelCtrl.GetFunnel)
	auth.GET("/analytics/funnel/stage", funnelCtrl.GetStageDetails)

	// AI 产能
	aiProdCtrl := opsctrl.NewAIProductivityController()
	auth.GET("/analytics/ai-productivity", aiProdCtrl.GetReport)
	auth.GET("/analytics/ai-productivity/trend", aiProdCtrl.GetDailyTrend)

	// 销冠能力画像
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

	auditCtrl := controller.NewSecurityAuditController()
	auth.POST("/security/audit", auditCtrl.RunAudit)
	auth.GET("/security/audit/list", auditCtrl.ListResults)
	auth.GET("/security/audit/:id", auditCtrl.GetResult)
}
