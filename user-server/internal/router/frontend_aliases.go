package router


import (
	"hivemtk-user/internal/app"
	contentctrl "hivemtk-user/internal/content/controller"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	opsctrl "hivemtk-user/internal/ops/controller"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupFrontendAliases 注册前端 API 路径别名（兼容前端调用习惯）
//
// 此函数必须在其他 setup* 函数之后调用，避免被更具体的路由抢先匹配。
// 通过 recover 机制捕获 Gin 在重复注册时的 panic，保证已存在路由不影响其他别名。
func setupFrontendAliases(auth *gin.RouterGroup, engine *gin.Engine, gormDB *gorm.DB) {
	aiAgentSvc := service.NewAIAgentServiceWithDB(gormDB)
	doReg := func(method, path string, handlers ...gin.HandlerFunc) {
		defer func() {
			if r := recover(); r != nil {
				_ = r
			}
		}()
		switch method {
		case "GET":
			auth.GET(path, handlers...)
		case "POST":
			auth.POST(path, handlers...)
		case "PUT":
			auth.PUT(path, handlers...)
		case "DELETE":
			auth.DELETE(path, handlers...)
		case "PATCH":
			auth.PATCH(path, handlers...)
		}
	}
	// doRegAdmin 注册仅 admin 可访问的路由别名（防 staff 改 LLM 配置 / 创建恶意 base_url）
	doRegAdmin := func(method, path string, handlers ...gin.HandlerFunc) {
		defer func() {
			if r := recover(); r != nil {
				_ = r
			}
		}()
		adminHandlers := append([]gin.HandlerFunc{middleware.AdminAuthMiddleware()}, handlers...)
		switch method {
		case "GET":
			auth.GET(path, adminHandlers...)
		case "POST":
			auth.POST(path, adminHandlers...)
		case "PUT":
			auth.PUT(path, adminHandlers...)
		case "DELETE":
			auth.DELETE(path, adminHandlers...)
		case "PATCH":
			auth.PATCH(path, adminHandlers...)
		}
	}

	emptyList := func(c *gin.Context) {
		response.SuccessWithList(c, []any{}, 0)
	}
	emptyObj := func(c *gin.Context) {
		c.JSON(200, gin.H{"code": "SUCCESS", "message": "ok", "data": gin.H{}})
	}

	notifCtrl := controller.NewNotificationController(service.NewNotificationService(gormDB))
	doReg("GET", "/notifications/list", notifCtrl.List)

	clueCtrl := controller.NewClueController()
	doReg("GET", "/clues", clueCtrl.GetClueList)
	doReg("GET", "/clues/list", clueCtrl.GetClueList)
	doReg("GET", "/clues/statistics", clueCtrl.GetClueStatistics)
	doReg("GET", "/clues/type", clueCtrl.GetClueTypes)
	doReg("GET", "/clues/import", emptyList)
	doReg("POST", "/clues/import", clueCtrl.ImportClues)
	doReg("GET", "/clue-statistics/overview", clueCtrl.GetClueStatistics)
	doReg("DELETE", "/clues/:id", clueCtrl.DeleteClue)

	customer360Ctrl := controller.NewCustomer360Controller()
	doReg("GET", "/customer-360/list", customer360Ctrl.GetCustomerList)
	doReg("GET", "/customer-360/stats", customer360Ctrl.GetCustomerStats)
	doReg("GET", "/customer-360/tags", customer360Ctrl.GetCustomerTags)
	doReg("GET", "/customer-360", customer360Ctrl.GetCustomer360)
	doReg("PUT", "/customer-360/tags", customer360Ctrl.UpdateCustomerTags)
	doReg("GET", "/customer/list", customer360Ctrl.GetCustomerList)
	doReg("GET", "/customer/360/:id", customer360Ctrl.GetCustomer360ByID)
	doReg("GET", "/customer/:id", customer360Ctrl.GetCustomerDetail)
	doReg("PUT", "/customer/:id", customer360Ctrl.UpdateCustomer)

	doReg("GET", "/customer-tags", customer360Ctrl.GetCustomerTags)
	doReg("PUT", "/customer-tags", customer360Ctrl.UpdateCustomerTags)
	doReg("GET", "/tag-segments", customer360Ctrl.GetTagStats)
	doReg("GET", "/tag-segmentation/list", customer360Ctrl.GetTagStats)

	customerEventCtrl := controller.NewCustomerEventController()
	doReg("GET", "/customer-events", customerEventCtrl.GetEventStats)
	doReg("GET", "/customer-events/list", customerEventCtrl.GetEventStats)
	doReg("GET", "/customer-events/stats", customerEventCtrl.GetEventStats)
	doReg("GET", "/customer-events/customer/:customer_id", customerEventCtrl.GetEventHistory)
	doReg("POST", "/customer-events/track", customerEventCtrl.TrackEvent)
	doReg("POST", "/customer-events/pageview", customerEventCtrl.TrackPageView)
	doReg("POST", "/customer-events/click", customerEventCtrl.TrackClick)
	doReg("POST", "/customer-events/purchase", customerEventCtrl.TrackPurchase)
	doReg("POST", "/customer-events/signup", customerEventCtrl.TrackSignup)
	doReg("POST", "/customer-events/login", customerEventCtrl.TrackLogin)
	doReg("POST", "/customer-events/add-to-cart", customerEventCtrl.TrackAddToCart)
	doReg("DELETE", "/customer-events/customer/:customer_id", customerEventCtrl.DeleteEvent)

	userSegmentCtrl := controller.NewUserSegmentController()
	doReg("GET", "/user-segments", userSegmentCtrl.ListRFMRules)
	doReg("GET", "/user-segments/list", userSegmentCtrl.ListRFMRules)
	doReg("GET", "/user-segments/rfm/list", userSegmentCtrl.GetRFMList)
	doReg("GET", "/user-segments/rfm/stats", userSegmentCtrl.GetRFMStats)
	doReg("GET", "/user-segments/layers", userSegmentCtrl.GetLayerDescription)

	unifiedMsgCtrl := controller.NewUnifiedMessageController()
	doReg("GET", "/unified-messages", unifiedMsgCtrl.GetMessages)
	doReg("GET", "/unified-messages/list", unifiedMsgCtrl.GetMessages)
	doReg("GET", "/unified-messages/:id", unifiedMsgCtrl.GetMessageByID)
	doReg("GET", "/unified-messages/:id/replies", unifiedMsgCtrl.GetReplies)

	oneIDCtrl := controller.NewCustomerOneIDController()
	doReg("GET", "/oneid/identities", oneIDCtrl.ListOneID)
	doReg("GET", "/oneid/conflicts", oneIDCtrl.ListConflicts)
	doReg("POST", "/oneid/merge", oneIDCtrl.MergeIdentity)
	doReg("POST", "/oneid/resolve", oneIDCtrl.ResolveIdentity)
	doReg("POST", "/oneid/conflicts/:id/resolve", oneIDCtrl.ResolveConflict)
	// OPT-UX-04: OneID 合并规则 CRUD
	doReg("GET", "/oneid/merge-rules", oneIDCtrl.GetMergeRules)
	doReg("POST", "/oneid/merge-rules", oneIDCtrl.SaveMergeRules)
	// MergeRuleConfig.vue 命中预览（返回 candidateCount + samples）
	doReg("POST", "/oneid/merge-rules/preview", oneIDCtrl.PreviewMergeRules)

	inboxCtrl := controller.NewInboxController(service.NewInboxService())
	doReg("GET", "/inbox/conversations", inboxCtrl.List)
	doReg("GET", "/inbox/conversations/list", inboxCtrl.List)
	doReg("GET", "/inbox/messages", inboxCtrl.GetMessages)
	doReg("GET", "/inbox/:id/messages", inboxCtrl.GetMessages)
	doReg("GET", "/inbox", inboxCtrl.List)
	doReg("GET", "/inbox/:id", inboxCtrl.GetByID)

	customerSessionCtrl := controller.NewCustomerSessionController()
	doReg("GET", "/customer-sessions", customerSessionCtrl.GetSessions)
	doReg("GET", "/customer-sessions/list", customerSessionCtrl.GetSessions)
	doReg("GET", "/customer-sessions/pending", customerSessionCtrl.GetPendingSessions)
	doReg("GET", "/customer-sessions/:id", customerSessionCtrl.GetSessionByID)
	doReg("GET", "/customer-sessions/:id/messages", customerSessionCtrl.GetMessages)
	doReg("POST", "/customer-sessions/:id/messages", customerSessionCtrl.SendMessage)
	doReg("POST", "/customer-sessions/:id/transfer", customerSessionCtrl.TransferSession)
	doReg("POST", "/customer-sessions/:id/tags", customerSessionCtrl.TagSession)
	doReg("POST", "/customer-sessions/:id/rate", customerSessionCtrl.RateSession)
	doReg("PUT", "/customer-sessions/:id/status", customerSessionCtrl.UpdateSessionStatus)

	intentRec := service.GetIntentRecognizer()
	if intentRec == nil {
		intentRec = service.NewIntentRecognizer(gormDB, app.GetGlobalDispatcher(), nil)
	}
	intentCtrl := controller.NewIntentController(intentRec)
	doReg("GET", "/intent-records", intentCtrl.RecentIntents)
	doReg("GET", "/intent-records/list", intentCtrl.RecentIntents)
	doReg("GET", "/intent-records/stats", intentCtrl.Stats)
	doReg("GET", "/intent-records/dict", intentCtrl.Intents)
	doReg("POST", "/intent-records/recognize", intentCtrl.Recognize)
	doReg("POST", "/intent-records/recognize/batch", intentCtrl.BatchRecognize)
	doReg("GET", "/intent-records/config", intentCtrl.GetConfig)
	doReg("PUT", "/intent-records/config", intentCtrl.UpdateConfig)

	memCtrl := controller.NewDialogueMemoryController(service.NewDialogueMemoryService(gormDB, nil))
	doReg("GET", "/dialogue-memories", memCtrl.Stats)
	doReg("GET", "/dialogue-memories/list", memCtrl.Stats)
	doReg("GET", "/dialogue-memories/stats", memCtrl.Stats)
	doReg("GET", "/dialogue-memories/:customer_id", memCtrl.ShortTerm)
	doReg("POST", "/dialogue-memories/messages", memCtrl.AppendMessage)
	doReg("GET", "/dialogue-memories/short", memCtrl.ShortTerm)
	doReg("GET", "/dialogue-memories/long", memCtrl.LongTerm)
	doReg("POST", "/dialogue-memories/facts", memCtrl.UpdateKeyFacts)
	doReg("POST", "/dialogue-memories/objections", memCtrl.RecordObjection)
	doReg("POST", "/dialogue-memories/purchase-intent", memCtrl.UpdatePurchaseIntent)
	doReg("POST", "/dialogue-memories/intent-trail", memCtrl.RecordIntent)
	doReg("POST", "/dialogue-memories/sop-history", memCtrl.RecordSOP)
	doReg("GET", "/dialogue-memories/context", memCtrl.BuildContext)

	routingSvc := service.NewLLMRoutingService(app.GetGlobalDispatcher())
	llmCtrl := controller.NewLLMRoutingController(routingSvc)
	doReg("GET", "/llm-routing/rules", llmCtrl.ListStrategies)
	doReg("GET", "/llm-routing/models", llmCtrl.ListModels)
	doRegAdmin("PUT", "/llm-routing/strategies", llmCtrl.UpdateStrategies)

	doReg("GET", "/llm/models", llmCtrl.ListModels)
	doReg("GET", "/llm/models/:id", llmCtrl.ListModels)
	doRegAdmin("POST", "/llm/models", llmCtrl.CreateModel)
	doRegAdmin("PUT", "/llm/models/:id", llmCtrl.UpdateModel)
	doRegAdmin("DELETE", "/llm/models/:id", llmCtrl.DeleteModel)
	doRegAdmin("PUT", "/llm/models/:id/status", llmCtrl.UpdateModel)
	doRegAdmin("POST", "/llm/models/:id/test", llmCtrl.TestModel)
	doReg("GET", "/llm/scene-routing", llmCtrl.ListStrategies)
	doRegAdmin("PUT", "/llm/scene-routing", llmCtrl.UpdateStrategies)
	doReg("GET", "/llm/fallback", llmCtrl.Stats)
	doRegAdmin("PUT", "/llm/fallback", llmCtrl.UpdateStrategies)
	doReg("GET", "/llm/cost-stats", llmCtrl.Usage)

	reachCtrl := controller.NewReachPipelineController(service.NewReachPipelineService(gormDB))
	doReg("GET", "/reach-pipelines", reachCtrl.ListPipelines)
	doReg("GET", "/reach-pipelines/list", reachCtrl.ListPipelines)
	doReg("GET", "/reach-pipelines/:id", reachCtrl.GetPipeline)
	doReg("POST", "/reach-pipelines", reachCtrl.CreatePipeline)
	doReg("PUT", "/reach-pipelines/:id", reachCtrl.UpdatePipeline)
	doReg("DELETE", "/reach-pipelines/:id", reachCtrl.DeletePipeline)

	marketingFlowCtrl := contentctrl.NewMarketingFlowController()
	doReg("GET", "/marketing-flows", marketingFlowCtrl.GetFlowList)
	doReg("GET", "/marketing-flows/list", marketingFlowCtrl.GetFlowList)
	doReg("GET", "/marketing-flows/:id", marketingFlowCtrl.GetFlowByID)
	doReg("POST", "/marketing-flows", marketingFlowCtrl.CreateFlow)
	doReg("PUT", "/marketing-flows/:id", marketingFlowCtrl.UpdateFlow)
	doReg("DELETE", "/marketing-flows/:id", marketingFlowCtrl.DeleteFlow)
	doReg("POST", "/marketing-flows/:id/activate", marketingFlowCtrl.ActivateFlow)
	doReg("POST", "/marketing-flows/:id/pause", marketingFlowCtrl.PauseFlow)
	doReg("POST", "/marketing-flows/:id/stop", marketingFlowCtrl.StopFlow)
	doReg("GET", "/marketing-flows/executions", marketingFlowCtrl.GetExecutionList)
	doReg("GET", "/marketing-flows/executions/stats", marketingFlowCtrl.GetExecutionStats)

	doReg("GET", "/batch-operations", emptyList)
	doReg("GET", "/batch-operations/list", emptyList)

	sopCtrl := controller.NewSOPController(service.NewSOPService(gormDB, nil))
	doReg("GET", "/sop-agents", sopCtrl.List)
	doReg("GET", "/sop-agents/list", sopCtrl.List)
	doReg("GET", "/sop-agents/stats", sopCtrl.Stats)
	doReg("GET", "/sop-agents/:id", sopCtrl.Get)
	doRegAdmin("POST", "/sop-agents", sopCtrl.Create)
	doRegAdmin("PUT", "/sop-agents/:id", sopCtrl.Update)
	doRegAdmin("DELETE", "/sop-agents/:id", sopCtrl.Delete)
	doRegAdmin("POST", "/sop-agents/:id/activate", sopCtrl.Activate)
	doRegAdmin("POST", "/sop-agents/:id/deactivate", sopCtrl.Deactivate)
	doRegAdmin("POST", "/sop-agents/:id/execute", sopCtrl.Execute)
	doRegAdmin("POST", "/sop-agents/:id/step", sopCtrl.Step)
	doRegAdmin("POST", "/sop-agents/:id/pause", sopCtrl.Pause)

	scriptCtrl := contentctrl.NewScriptTemplateController()
	doReg("GET", "/script-templates", scriptCtrl.GetTemplateList)
	doReg("GET", "/script-templates/list", scriptCtrl.GetTemplateList)
	doReg("GET", "/script-templates/categories", scriptCtrl.GetCategories)
	doReg("GET", "/script-templates/:id", scriptCtrl.GetTemplateByID)
	doRegAdmin("POST", "/script-templates", scriptCtrl.CreateTemplate)
	doRegAdmin("PUT", "/script-templates/:id", scriptCtrl.UpdateTemplate)
	doRegAdmin("DELETE", "/script-templates/:id", scriptCtrl.DeleteTemplate)
	doReg("GET", "/script-templates/search", scriptCtrl.SearchTemplates)
	doReg("GET", "/script-templates/public", scriptCtrl.GetPublicTemplates)
	doReg("POST", "/script-templates/recommend", scriptCtrl.RecommendScript)

	agentStatusCtrl := controller.NewAgentStatusController(aiAgentSvc)
	doReg("GET", "/agent-statuses", agentStatusCtrl.GetOnlineAgents)
	doReg("GET", "/agent-statuses/online", agentStatusCtrl.GetOnlineAgents)
	doReg("GET", "/agent-statuses/list", agentStatusCtrl.ListAllAgents)
	doReg("GET", "/agent-statuses/:id", agentStatusCtrl.GetAgentStatus)
	doReg("PUT", "/agent-statuses/:id/status", agentStatusCtrl.UpdateAgentStatus)
	doReg("POST", "/agent-statuses/:id/online", agentStatusCtrl.GoOnline)
	doReg("POST", "/agent-statuses/:id/offline", agentStatusCtrl.GoOffline)
	doReg("GET", "/agent-statuses/available", agentStatusCtrl.GetOnlineAgents)
	doReg("GET", "/agent-statuses/:id/sessions", agentStatusCtrl.GetAgentSessions)

	quickReplyCtrl := controller.NewQuickReplyController()
	doReg("GET", "/quick-replies", quickReplyCtrl.GetReplies)
	doReg("GET", "/quick-replies/list", quickReplyCtrl.GetReplies)
	doReg("GET", "/quick-replies/categories", quickReplyCtrl.GetReplyCategories)
	doReg("POST", "/quick-replies", quickReplyCtrl.CreateReply)
	doReg("PUT", "/quick-replies/:id", quickReplyCtrl.UpdateReply)
	doReg("DELETE", "/quick-replies/:id", quickReplyCtrl.DeleteReply)

	sessionTagCtrl := controller.NewSessionTagController()
	doReg("GET", "/session-tags", sessionTagCtrl.GetTags)
	doReg("GET", "/session-tags/list", sessionTagCtrl.GetTags)
	doReg("POST", "/session-tags", sessionTagCtrl.CreateTag)
	doReg("PUT", "/session-tags/:id", sessionTagCtrl.UpdateTag)
	doReg("DELETE", "/session-tags/:id", sessionTagCtrl.DeleteTag)

	aiSuggestionCtrl := controller.NewAISuggestionController()
	doReg("GET", "/ai-suggestions", aiSuggestionCtrl.GetSuggestions)
	doReg("GET", "/ai-suggestions/list", aiSuggestionCtrl.GetSuggestions)
	doReg("GET", "/ai-suggestions/:session_id", aiSuggestionCtrl.GetSuggestions)
	doReg("POST", "/ai-suggestions/:id/use", aiSuggestionCtrl.UseSuggestion)

	objCtrl := controller.NewObjectionHandlerController()
	doReg("GET", "/objection-templates", objCtrl.ListCategories)
	doReg("GET", "/objection-templates/list", objCtrl.ListCategories)
	doReg("POST", "/objection-templates/handle", objCtrl.Handle)
	doReg("POST", "/objection-templates/classify", objCtrl.Classify)
	doReg("POST", "/objection-templates/usage", objCtrl.RecordUsage)

	personaCtrl := controller.NewSalesPersonaController()
	doReg("GET", "/sales-champions", personaCtrl.ListStaffs)
	doReg("GET", "/sales-champions/list", personaCtrl.ListStaffs)
	doReg("GET", "/sales-champions/:id", personaCtrl.GetReport)

	emailListCtrl := controller.NewEmailListController()
	doReg("GET", "/email/lists", emailListCtrl.GetEmailListList)
	doReg("GET", "/email/lists/list", emailListCtrl.GetEmailListList)
	doReg("GET", "/email/lists/:id", emailListCtrl.GetEmailListDetail)
	doReg("POST", "/email/lists", emailListCtrl.CreateEmailList)
	doReg("PUT", "/email/lists/:id", emailListCtrl.UpdateEmailList)
	doReg("DELETE", "/email/lists/:id", emailListCtrl.DeleteEmailList)
	doReg("GET", "/email/lists/:id/trace", emailListCtrl.TraceEmail)
	doReg("GET", "/email/lists/:id/tracking", emailListCtrl.GetTracking)

	emailSmtpCtrl := controller.NewEmailSmtpController()
	doReg("GET", "/email/smtps", emailSmtpCtrl.GetEmailSmtpList)
	doReg("GET", "/email/smtps/list", emailSmtpCtrl.GetEmailSmtpList)
	doReg("GET", "/email/smtps/:id", emailSmtpCtrl.GetEmailSmtp)
	doReg("POST", "/email/smtps", emailSmtpCtrl.CreateEmailSmtp)
	doReg("PUT", "/email/smtps/:id", emailSmtpCtrl.UpdateEmailSmtp)
	doReg("DELETE", "/email/smtps/:id", emailSmtpCtrl.DeleteEmailSmtp)

	smsCtrl := controller.NewSmsController(service.NewSmsService(repository.NewSmsRepository()))
	doReg("GET", "/sms/records", smsCtrl.GetSmsList)
	doReg("GET", "/sms/records/list", smsCtrl.GetSmsList)
	doReg("POST", "/sms/records", smsCtrl.SendSms)

	doReg("GET", "/sms/drafts", smsCtrl.GetDraftList)
	doReg("GET", "/sms/drafts/list", smsCtrl.GetDraftList)
	doReg("POST", "/sms/drafts", smsCtrl.CreateDraft)
	doReg("PUT", "/sms/drafts/:id", smsCtrl.UpdateDraft)
	doReg("DELETE", "/sms/drafts/:id", smsCtrl.DeleteDraft)

	doReg("GET", "/sms/jobs", smsCtrl.GetJobList)
	doReg("GET", "/sms/jobs/list", smsCtrl.GetJobList)
	doReg("POST", "/sms/jobs", smsCtrl.CreateJob)
	doReg("POST", "/sms/jobs/:id/pause", smsCtrl.PauseJob)
	doReg("POST", "/sms/jobs/:id/resume", smsCtrl.ResumeJob)
	doReg("DELETE", "/sms/jobs/:id", smsCtrl.DeleteJob)

	doReg("GET", "/sms/configs", smsCtrl.GetConfig)
	doReg("GET", "/sms/configs/list", smsCtrl.GetConfig)
	doReg("POST", "/sms/configs", smsCtrl.SaveConfig)

	{
		douyinCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(gormDB))
		doReg("GET", "/douyin-cards", douyinCtrl.GetList)
		doReg("GET", "/douyin-cards/list", douyinCtrl.GetList)
		doReg("GET", "/douyin-cards/:id", douyinCtrl.GetByID)
		doReg("POST", "/douyin-cards", douyinCtrl.Create)
		doReg("PUT", "/douyin-cards/:id", douyinCtrl.Update)
		doReg("DELETE", "/douyin-cards/:id", douyinCtrl.Delete)
	}
	{
		kuaishouCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(gormDB))
		doReg("GET", "/kuaishou-cards", kuaishouCtrl.GetList)
		doReg("GET", "/kuaishou-cards/list", kuaishouCtrl.GetList)
		doReg("GET", "/kuaishou-cards/:id", kuaishouCtrl.GetByID)
		doReg("POST", "/kuaishou-cards", kuaishouCtrl.Create)
		doReg("PUT", "/kuaishou-cards/:id", kuaishouCtrl.Update)
		doReg("DELETE", "/kuaishou-cards/:id", kuaishouCtrl.Delete)
	}
	{
		xhsCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(gormDB))
		doReg("GET", "/xiaohongshu-cards", xhsCtrl.GetList)
		doReg("GET", "/xiaohongshu-cards/list", xhsCtrl.GetList)
		doReg("GET", "/xiaohongshu-cards/:id", xhsCtrl.GetByID)
		doReg("POST", "/xiaohongshu-cards", xhsCtrl.Create)
		doReg("PUT", "/xiaohongshu-cards/:id", xhsCtrl.Update)
		doReg("DELETE", "/xiaohongshu-cards/:id", xhsCtrl.Delete)
	}
	{
		xianyuCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(gormDB), service.NewXianyuCardStatsService(gormDB))
		doReg("GET", "/xianyu-cards", xianyuCtrl.GetList)
		doReg("GET", "/xianyu-cards/list", xianyuCtrl.GetList)
		doReg("GET", "/xianyu-cards/:id", xianyuCtrl.GetByID)
		doReg("POST", "/xianyu-cards", xianyuCtrl.Create)
		doReg("PUT", "/xianyu-cards/:id", xianyuCtrl.Update)
		doReg("DELETE", "/xianyu-cards/:id", xianyuCtrl.Delete)
	}

	tiktokCtrl := controller.NewTikTokCardController(
		service.NewTikTokCardServiceWithDB(gormDB),
	)
	doReg("GET", "/tiktok-cards", tiktokCtrl.List)
	doReg("GET", "/tiktok-cards/list", tiktokCtrl.List)
	doReg("GET", "/tiktok-cards/:id", tiktokCtrl.Get)
	doReg("POST", "/tiktok-cards", tiktokCtrl.Create)
	doReg("PUT", "/tiktok-cards/:id", tiktokCtrl.Update)
	doReg("DELETE", "/tiktok-cards/:id", tiktokCtrl.Delete)

	feishuCtrl := controller.NewFeishuAccountController(service.NewFeishuService(gormDB), service.NewFeishuIntegrationService(gormDB))
	doReg("GET", "/feishu/accounts", feishuCtrl.List)
	doReg("GET", "/feishu/accounts/list", feishuCtrl.List)
	doReg("GET", "/feishu/accounts/:id", feishuCtrl.Get)
	doReg("POST", "/feishu/accounts", feishuCtrl.Create)
	doReg("PUT", "/feishu/accounts/:id", feishuCtrl.Update)
	doReg("DELETE", "/feishu/accounts/:id", feishuCtrl.Delete)

	tgCtrl := controller.NewTelegramAccountController(service.NewTelegramService(gormDB))
	doReg("GET", "/telegram/accounts", tgCtrl.List)
	doReg("GET", "/telegram/accounts/list", tgCtrl.List)
	doReg("GET", "/telegram/accounts/:id", tgCtrl.Get)
	doReg("POST", "/telegram/accounts", tgCtrl.Create)
	doReg("PUT", "/telegram/accounts/:id", tgCtrl.Update)
	doReg("DELETE", "/telegram/accounts/:id", tgCtrl.Delete)

	shortLinkCtrl := controller.NewShortLinkController(service.NewShortLinkService(gormDB))
	doReg("GET", "/short-links", shortLinkCtrl.GetList)
	doReg("GET", "/short-links/list", shortLinkCtrl.GetList)
	doReg("GET", "/short-links/:id", shortLinkCtrl.GetByID)
	doReg("POST", "/short-links", shortLinkCtrl.Create)
	doReg("PUT", "/short-links/:id", shortLinkCtrl.Update)
	doReg("DELETE", "/short-links/:id", shortLinkCtrl.Delete)

	liveCodeCtrl := controller.NewLiveCodeController(service.NewLiveCodeService(gormDB))
	doReg("GET", "/live-codes", liveCodeCtrl.GetList)
	doReg("GET", "/live-codes/list", liveCodeCtrl.GetList)
	doReg("GET", "/live-codes/:id", liveCodeCtrl.GetByID)
	doReg("POST", "/live-codes", liveCodeCtrl.Create)
	doReg("PUT", "/live-codes/:id", liveCodeCtrl.Update)
	doReg("DELETE", "/live-codes/:id", liveCodeCtrl.Delete)

	doReg("GET", "/rag-product-configs", emptyObj)
	doReg("GET", "/rag-product-configs/list", emptyList)

	aiContentCtrl := contentctrl.NewAIContentController()
	doReg("GET", "/ai-content/list", aiContentCtrl.GetGenerationHistory)
	doReg("GET", "/ai-content/history", aiContentCtrl.GetGenerationHistory)
	doReg("GET", "/ai-content/templates", aiContentCtrl.GetTemplates)
	doReg("GET", "/ai-content/templates/:id", aiContentCtrl.GetTemplateByID)
	doRegAdmin("POST", "/ai-content/generate", aiContentCtrl.GenerateContent)
	doRegAdmin("POST", "/ai-content", aiContentCtrl.CreateHistory)
	doReg("GET", "/ai-content/:id", aiContentCtrl.GetRecordByID)
	doRegAdmin("POST", "/ai-content/:id/save", aiContentCtrl.SaveRecord)
	doReg("POST", "/ai-content/:id/favorite", aiContentCtrl.FavoriteRecord)
	doReg("POST", "/ai-content/:id/rate", aiContentCtrl.RateRecord)
	doRegAdmin("DELETE", "/ai-content/:id", aiContentCtrl.DeleteRecord)

	templateCtrl := contentctrl.NewTemplateMarketController()
	doReg("GET", "/market-templates", templateCtrl.GetTemplateList)
	doReg("GET", "/market-templates/list", templateCtrl.GetTemplateList)
	doReg("GET", "/market-templates/official", templateCtrl.GetOfficialTemplates)
	doReg("GET", "/market-templates/search", templateCtrl.SearchTemplates)
	doReg("GET", "/market-templates/my-downloads", templateCtrl.GetMyDownloads)
	doReg("GET", "/market-templates/:id", templateCtrl.GetTemplateByID)
	doReg("POST", "/market-templates", templateCtrl.CreateTemplate)
	doReg("POST", "/market-templates/:id/download", templateCtrl.DownloadTemplate)
	doReg("POST", "/market-templates/:id/rate", templateCtrl.RateTemplate)

	dashCtrl := opsctrl.NewDashboardScreenController()
	doReg("GET", "/dashboard-screens", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screens/list", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screens/:id", dashCtrl.GetScreenByID)
	doReg("POST", "/dashboard-screens", dashCtrl.CreateScreen)
	doReg("PUT", "/dashboard-screens/:id", dashCtrl.UpdateScreen)
	doReg("DELETE", "/dashboard-screens/:id", dashCtrl.DeleteScreen)
	doReg("GET", "/dashboard-screens/:id/data", dashCtrl.GetDashboardData)
	doReg("GET", "/dashboard-screens/:id/activities", dashCtrl.GetRealtimeActivities)
	doReg("GET", "/dashboards", dashCtrl.GetScreenList)
	doReg("GET", "/dashboards/list", dashCtrl.GetScreenList)
	doReg("GET", "/dashboards/:id", dashCtrl.GetScreenByID)
	doReg("POST", "/dashboards", dashCtrl.CreateScreen)
	doReg("PUT", "/dashboards/:id", dashCtrl.UpdateScreen)
	doReg("DELETE", "/dashboards/:id", dashCtrl.DeleteScreen)
	doReg("GET", "/dashboards/:id/data", dashCtrl.GetDashboardData)
	doReg("GET", "/dashboards/data", dashCtrl.GetDashboardData)
	doReg("GET", "/dashboards/activities", dashCtrl.GetRealtimeActivities)
	doReg("GET", "/dashboards/public/:code", dashCtrl.PublicViewScreen)
	doReg("GET", "/dashboard-screen/list", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screen", dashCtrl.GetScreenList)

	funnelCtrl := opsctrl.NewConversionFunnelController()
	doReg("GET", "/conversion-funnels", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnels/list", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnels/stage", funnelCtrl.GetStageDetails)
	doReg("GET", "/analytics/funnel", funnelCtrl.GetFunnel)
	doReg("GET", "/analytics/funnel/stage", funnelCtrl.GetStageDetails)
	doReg("GET", "/conversion-funnel", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnel/list", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnel/stage", funnelCtrl.GetStageDetails)

	aiProdCtrl := opsctrl.NewAIProductivityController()
	doReg("GET", "/ai-productivity/overview", aiProdCtrl.GetReport)
	doReg("GET", "/ai-productivity/trend", aiProdCtrl.GetDailyTrend)

	journeyCtrl := controller.NewCustomerJourneyController()
	doReg("GET", "/customer-journey/dashboard", journeyCtrl.GetOverview)
	doReg("GET", "/customer-journey/overview", journeyCtrl.GetOverview)
	doReg("GET", "/customer-journey/stages", journeyCtrl.ListStages)
	doReg("GET", "/customer-journey/by-stage", journeyCtrl.ListByStage)
	doReg("POST", "/customer-journey/touch", journeyCtrl.TouchCustomer)
	doReg("POST", "/customer-journey/transition", journeyCtrl.TransitionStage)

	sysCfgCtrl := controller.NewSystemConfigController()
	doReg("GET", "/system/configs", sysCfgCtrl.GetConfig)
	doReg("GET", "/system/configs/list", sysCfgCtrl.GetConfig)
	doReg("GET", "/system/configs/:key", sysCfgCtrl.GetConfig)
	doReg("PUT", "/system/configs/:key", sysCfgCtrl.SaveConfig)

	obsCtrl := controller.NewObsConfigController()
	doReg("GET", "/obs-configs", obsCtrl.GetConfigList)
	doReg("GET", "/obs-configs/list", obsCtrl.GetConfigList)
	doReg("GET", "/obs-configs/default", obsCtrl.GetDefaultConfig)
	doReg("GET", "/obs-configs/:id", obsCtrl.GetConfig)
	doReg("POST", "/obs-configs", obsCtrl.CreateConfig)
	doReg("PUT", "/obs-configs/:id", obsCtrl.UpdateConfig)
	doReg("DELETE", "/obs-configs/:id", obsCtrl.DeleteConfig)
	doReg("POST", "/obs-configs/:id/test", obsCtrl.TestConnection)
	doReg("POST", "/obs-configs/:id/default", obsCtrl.SetDefault)

	doReg("GET", "/material-library/list", emptyList)
	doReg("GET", "/material-library", emptyList)
	doReg("GET", "/material-library/categories", emptyList)
	doReg("GET", "/material-library/stats", emptyObj)

	doReg("GET", "/system-monitor/metrics", emptyObj)
	doReg("GET", "/system-monitor/health", emptyObj)

	// R35 契约漂移处置（audit_checklist.md：3组真实在用端点的优雅空态，防页面报错）
	emptyListResp := func(c *gin.Context) {
		c.JSON(200, gin.H{"code": "SUCCESS", "message": "ok", "data": gin.H{"list": []any{}, "total": 0}})
	}
	doReg("GET", "/csat/stats", emptyObj)
	doReg("GET", "/csat/template", emptyObj)
	doReg("GET", "/csat/trend", emptyList)
	doReg("GET", "/csat/negative", emptyListResp)
	doReg("GET", "/mentions/mine", emptyListResp)
	doReg("GET", "/system/menus", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": "SUCCESS", "message": "ok", "data": []any{}})
	})

	domainDB := gormDB
	domainPoolRepo := repository.NewDomainPoolRepository(domainDB)
	domainCtrl := controller.NewDomainPoolController(
		service.NewDomainPoolService(domainDB),
		service.NewDomainHealthService(domainDB, domainPoolRepo),
	)
	doReg("GET", "/domain-pool", domainCtrl.List)
	doReg("GET", "/domain-pool/list", domainCtrl.List)
	doReg("GET", "/domain-pool/:id", domainCtrl.GetByID)
	doReg("POST", "/domain-pool", domainCtrl.Create)
	doReg("PUT", "/domain-pool/:id", domainCtrl.Update)
	doReg("DELETE", "/domain-pool/:id", domainCtrl.Delete)




	integrationCtrl := controller.NewIntegrationController()
	doReg("GET", "/integrations/list", integrationCtrl.GetAccountList)
	doReg("GET", "/integrations/:id", integrationCtrl.GetAccountByID)
	doReg("POST", "/integrations", integrationCtrl.CreateAccount)
	doReg("PUT", "/integrations/:id", integrationCtrl.UpdateAccount)
	doReg("DELETE", "/integrations/:id", integrationCtrl.DeleteAccount)
	doReg("GET", "/integrations/:id/sync-logs", integrationCtrl.GetSyncLogs)
	doReg("POST", "/integrations/:id/test", integrationCtrl.TestIntegration)
	doReg("POST", "/integrations/:id/sync/customers", integrationCtrl.SyncCustomers)
	doReg("POST", "/integrations/:id/sync/products", integrationCtrl.SyncProducts)

	opLogCtrl := controller.NewOperationLogController()
	doReg("GET", "/operation-logs", opLogCtrl.GetList)
	doReg("GET", "/operation-logs/list", opLogCtrl.GetList)
	doReg("GET", "/operation-logs/:id", opLogCtrl.GetByID)
	doReg("GET", "/operation-logs/statistics", opLogCtrl.GetStatistics)

	backupCtrl := controller.NewBackupController()
	doReg("GET", "/backups", backupCtrl.GetBackupList)
	doReg("GET", "/backups/list", backupCtrl.GetBackupList)
	doReg("GET", "/backups/:id", backupCtrl.GetBackupByID)

	churnCtrl := opsctrl.NewChurnPredictionController()
	doReg("GET", "/churn-prediction", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn-prediction/list", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn-prediction/users", churnCtrl.GetHighRiskUsers)
	doReg("GET", "/churn-prediction/warnings", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn-prediction/unhandled-warnings", churnCtrl.GetUnhandledWarnings)
	doReg("GET", "/churn-prediction/statistics", churnCtrl.GetChurnStatistics)
	doReg("GET", "/churn-prediction/risk-distribution", churnCtrl.GetRiskDistribution)
	doReg("GET", "/churn-prediction/model-config", churnCtrl.GetModelConfig)
	doReg("POST", "/churn-prediction/model-config", churnCtrl.SaveModelConfig)
	doReg("GET", "/churn/prediction", churnCtrl.GetChurnPrediction)
	doReg("GET", "/churn/predictions", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn/high-risk-users", churnCtrl.GetHighRiskUsers)
	doReg("GET", "/churn/warnings", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn/unhandled-warnings", churnCtrl.GetUnhandledWarnings)
	doReg("GET", "/churn/model-config", churnCtrl.GetModelConfig)
	doReg("POST", "/churn/model-config", churnCtrl.SaveModelConfig)
	doReg("GET", "/churn/statistics", churnCtrl.GetChurnStatistics)
	doReg("GET", "/churn/risk-distribution", churnCtrl.GetRiskDistribution)
	doReg("POST", "/backups", backupCtrl.CreateBackup)
	doReg("DELETE", "/backups/:id", backupCtrl.DeleteBackup)
}

