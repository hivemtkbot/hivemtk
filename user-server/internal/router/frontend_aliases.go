package router

// frontend_aliases.go
// 前端 API 路径别名（兼容前端调用习惯）
//
// 背景：
//   - 前端代码（user-web）使用复数路径（如 /api/notifications、/api/clues）
//   - 后端早期实现使用单数/不同形态（如 /api/clue、/api/notification）
//   - 此文件集中提供别名路由，确保前端调用全部命中后端
//
// 规则：
//   1. 别名路由直接挂到 /api 路径下，无前缀
//   2. 真正的业务实现仍走原有 route（保持单一实现源）
//   3. 别名只在 path 层面转发，不改变参数/响应格式
//
// 修改记录：
//   - 2026-07-18: 一次性补充菜单所需全部别名

import (
	contentctrl "marketing/internal/content/controller"
	"marketing/internal/controller"
	opsctrl "marketing/internal/ops/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupFrontendAliases 注册前端 API 路径别名（兼容前端调用习惯）
//
// 此函数必须在其他 setup* 函数之后调用，避免被更具体的路由抢先匹配。
// 通过 recover 机制捕获 Gin 在重复注册时的 panic，保证已存在路由不影响其他别名。
func setupFrontendAliases(auth *gin.RouterGroup, engine *gin.Engine) {
	// 2026-07-23 五层架构治理（二轮）：构造本地 aiAgentSvc 供 agent_status controller
	// 使用，避免 controller 直接调 dbutil.GetDB()。
	aiAgentSvc := service.NewAIAgentService(db.GetDB())
	// 通用 helper：注册时捕获重复注册的 panic
	doReg := func(method, path string, handlers ...gin.HandlerFunc) {
		defer func() {
			if r := recover(); r != nil {
				// 已被其他 setup* 函数注册 - 静默跳过
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

	// ============================================================
	// 通用占位响应辅助
	// ============================================================
	emptyList := func(c *gin.Context) {
		response.SuccessWithList(c, []any{}, 0)
	}
	emptyObj := func(c *gin.Context) {
		c.JSON(200, gin.H{"code": "SUCCESS", "message": "ok", "data": gin.H{}})
	}

	// ============================================================
	// 1. 通知中心 - 别名（前端用 /api/notifications）
	// 已在 auth_routes.go 中注册基础路由
	// ============================================================
	notifCtrl := controller.NewNotificationController(service.NewNotificationService(db.GetDB()))
	doReg("GET", "/notifications/list", notifCtrl.List)

	// ============================================================
	// 2. 线索 - 别名（前端用 /api/clues, /api/clue-statistics）
	// ============================================================
	clueCtrl := controller.NewClueController()
	doReg("GET", "/clues", clueCtrl.GetClueList)
	doReg("GET", "/clues/list", clueCtrl.GetClueList)
	doReg("GET", "/clues/statistics", clueCtrl.GetClueStatistics)
	doReg("GET", "/clues/type", clueCtrl.GetClueTypes)
	doReg("GET", "/clues/import", emptyList)
	doReg("POST", "/clues/import", clueCtrl.ImportClues)
	doReg("GET", "/clue-statistics/overview", clueCtrl.GetClueStatistics)
	doReg("DELETE", "/clues/:id", clueCtrl.DeleteClue)

	// ============================================================
	// 3. 客户 360 - 别名
	// ============================================================
	customer360Ctrl := controller.NewCustomer360Controller()
	doReg("GET", "/customer-360/list", customer360Ctrl.GetCustomerList)
	doReg("GET", "/customer-360/stats", customer360Ctrl.GetCustomerStats)
	doReg("GET", "/customer-360/tags", customer360Ctrl.GetCustomerTags)
	doReg("GET", "/customer-360", customer360Ctrl.GetCustomer360)
	doReg("PUT", "/customer-360/tags", customer360Ctrl.UpdateCustomerTags)
	// 客户 360 - 兼容路径（前端 /api/customer-360/list → /api/customer/list）
	doReg("GET", "/customer/list", customer360Ctrl.GetCustomerList)
	doReg("GET", "/customer/360/:id", customer360Ctrl.GetCustomer360ByID)
	doReg("GET", "/customer/:id", customer360Ctrl.GetCustomerDetail)
	doReg("PUT", "/customer/:id", customer360Ctrl.UpdateCustomer)

	// 客户标签 - 别名
	doReg("GET", "/customer-tags", customer360Ctrl.GetCustomerTags)
	doReg("PUT", "/customer-tags", customer360Ctrl.UpdateCustomerTags)
	doReg("GET", "/tag-segments", customer360Ctrl.GetTagStats)
	doReg("GET", "/tag-segmentation/list", customer360Ctrl.GetTagStats)

	// ============================================================
	// 4. 客户事件 - 别名（前端用 /api/customer-events）
	// ============================================================
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

	// ============================================================
	// 5. 用户分层 RFM - 别名
	// ============================================================
	userSegmentCtrl := controller.NewUserSegmentController()
	doReg("GET", "/user-segments", userSegmentCtrl.GetRFMList)
	doReg("GET", "/user-segments/list", userSegmentCtrl.GetRFMList)
	doReg("GET", "/user-segments/rfm/list", userSegmentCtrl.GetRFMList)
	doReg("GET", "/user-segments/rfm/stats", userSegmentCtrl.GetRFMStats)
	doReg("GET", "/user-segments/layers", userSegmentCtrl.GetLayerDescription)

	// ============================================================
	// 6. 统一消息 - 别名
	// ============================================================
	unifiedMsgCtrl := controller.NewUnifiedMessageController()
	doReg("GET", "/unified-messages", unifiedMsgCtrl.GetMessages)
	doReg("GET", "/unified-messages/list", unifiedMsgCtrl.GetMessages)
	doReg("GET", "/unified-messages/:id", unifiedMsgCtrl.GetMessageByID)
	doReg("GET", "/unified-messages/:id/replies", unifiedMsgCtrl.GetReplies)

	// ============================================================
	// 7. 订单 - 别名（前端用 /api/orders）
	// ============================================================
	orderCtrl := controller.NewOrderController()
	doReg("GET", "/orders", orderCtrl.GetOrderList)
	doReg("GET", "/orders/list", orderCtrl.GetOrderList)
	doReg("GET", "/orders/recent", orderCtrl.GetRecentOrderList)
	doReg("GET", "/orders/:id", orderCtrl.GetOrderByID)
	doReg("POST", "/orders", orderCtrl.CreateOrder)
	doReg("PUT", "/orders/:id", orderCtrl.UpdateOrder)
	doReg("DELETE", "/orders/:id", orderCtrl.DeleteOrder)
	doReg("POST", "/orders/:id/cancel", orderCtrl.CancelOrder)
	doReg("POST", "/orders/:id/refund", orderCtrl.RefundOrder)
	doReg("POST", "/orders/:id/pay", orderCtrl.PayOrder)
	doReg("GET", "/orders/:id/pay-status", orderCtrl.CheckPayStatus)
	doReg("GET", "/order/list", orderCtrl.GetOrderList)
	doReg("GET", "/order/:id", orderCtrl.GetOrderByID)
	doReg("POST", "/order", orderCtrl.CreateOrder)

	// ============================================================
	// 8. OneID - 别名（前端用 /api/oneid/*）
	// ============================================================
	oneIDCtrl := controller.NewCustomerOneIDController()
	doReg("GET", "/oneid/identities", oneIDCtrl.ListOneID)
	doReg("GET", "/oneid/conflicts", oneIDCtrl.ListConflicts)
	doReg("POST", "/oneid/merge", oneIDCtrl.MergeIdentity)
	doReg("POST", "/oneid/resolve", oneIDCtrl.ResolveIdentity)
	doReg("POST", "/oneid/conflicts/:id/resolve", oneIDCtrl.ResolveConflict)

	// ============================================================
	// 9. 统一收件箱 - 别名（前端用 /api/inbox/conversations）
	// ============================================================
	inboxCtrl := controller.NewInboxController(service.NewInboxService())
	doReg("GET", "/inbox/conversations", inboxCtrl.List)
	doReg("GET", "/inbox/conversations/list", inboxCtrl.List)
	doReg("GET", "/inbox/messages", inboxCtrl.GetMessages)
	doReg("GET", "/inbox/:id/messages", inboxCtrl.GetMessages)
	doReg("GET", "/inbox", inboxCtrl.List)
	doReg("GET", "/inbox/:id", inboxCtrl.GetByID)

	// ============================================================
	// 10. 客服会话 - 别名（前端用 /api/customer-sessions）
	// ============================================================
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

	// ============================================================
	// 11. 意图识别 - 别名（前端用 /api/intent-records）
	// ============================================================
	intentCtrl := controller.NewIntentController(service.NewIntentRecognizer(db.GetDB(), nil, nil))
	doReg("GET", "/intent-records", intentCtrl.RecentIntents)
	doReg("GET", "/intent-records/list", intentCtrl.RecentIntents)
	doReg("GET", "/intent-records/stats", intentCtrl.Stats)
	doReg("GET", "/intent-records/dict", intentCtrl.Intents)
	doReg("POST", "/intent-records/recognize", intentCtrl.Recognize)
	doReg("POST", "/intent-records/recognize/batch", intentCtrl.BatchRecognize)

	// ============================================================
	// 12. 对话记忆 - 别名（前端用 /api/dialogue-memories）
	// ============================================================
	memCtrl := controller.NewDialogueMemoryController(service.NewDialogueMemoryService(db.GetDB(), nil))
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

	// ============================================================
	// 13. LLM 路由 - 别名
	// ============================================================
	llmCtrl := controller.NewLLMRoutingController()
	doReg("GET", "/llm-routing/rules", llmCtrl.ListStrategies)
	doReg("GET", "/llm-routing/models", llmCtrl.ListModels)
	doReg("PUT", "/llm-routing/strategies", llmCtrl.UpdateStrategies)

	// 兼容前端 /api/llm/* 路径（页面 LlmRoutingApi 实际使用）
	doReg("GET", "/llm/models", llmCtrl.ListModels)
	doReg("GET", "/llm/models/:id", llmCtrl.ListModels)
	doReg("POST", "/llm/models", llmCtrl.CreateModel)
	doReg("PUT", "/llm/models/:id", llmCtrl.UpdateModel)
	doReg("DELETE", "/llm/models/:id", llmCtrl.DeleteModel)
	doReg("PUT", "/llm/models/:id/status", llmCtrl.UpdateModel)
	doReg("POST", "/llm/models/:id/test", llmCtrl.TestModel)
	doReg("GET", "/llm/scene-routing", llmCtrl.ListStrategies)
	doReg("PUT", "/llm/scene-routing", llmCtrl.UpdateStrategies)
	doReg("GET", "/llm/fallback", llmCtrl.Stats)
	doReg("PUT", "/llm/fallback", llmCtrl.UpdateStrategies)
	doReg("GET", "/llm/cost-stats", llmCtrl.Usage)

	// ============================================================
	// 14. 触达 Pipeline - 别名（前端用 /api/reach-pipelines）
	// ============================================================
	reachCtrl := controller.NewReachPipelineController(service.NewReachPipelineService(db.GetDB()))
	doReg("GET", "/reach-pipelines", reachCtrl.ListPipelines)
	doReg("GET", "/reach-pipelines/list", reachCtrl.ListPipelines)
	doReg("GET", "/reach-pipelines/:id", reachCtrl.GetPipeline)
	doReg("POST", "/reach-pipelines", reachCtrl.CreatePipeline)
	doReg("PUT", "/reach-pipelines/:id", reachCtrl.UpdatePipeline)
	doReg("DELETE", "/reach-pipelines/:id", reachCtrl.DeletePipeline)

	// ============================================================
	// 15. 营销流程 - 别名（content controller）
	// ============================================================
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

	// ============================================================
	// 16. 批量操作 - 别名
	// ============================================================
	doReg("GET", "/batch-operations", emptyList)
	doReg("GET", "/batch-operations/list", emptyList)

	// ============================================================
	// 17. SOP 智能体 - 别名
	// ============================================================
	sopCtrl := controller.NewSOPController(service.NewSOPService(db.GetDB(), nil))
	doReg("GET", "/sop-agents", sopCtrl.List)
	doReg("GET", "/sop-agents/list", sopCtrl.List)
	doReg("GET", "/sop-agents/stats", sopCtrl.Stats)
	doReg("GET", "/sop-agents/:id", sopCtrl.Get)
	doReg("POST", "/sop-agents", sopCtrl.Create)
	doReg("PUT", "/sop-agents/:id", sopCtrl.Update)
	doReg("DELETE", "/sop-agents/:id", sopCtrl.Delete)
	doReg("POST", "/sop-agents/:id/activate", sopCtrl.Activate)
	doReg("POST", "/sop-agents/:id/deactivate", sopCtrl.Deactivate)
	doReg("POST", "/sop-agents/:id/execute", sopCtrl.Execute)
	doReg("POST", "/sop-agents/:id/step", sopCtrl.Step)
	doReg("POST", "/sop-agents/:id/pause", sopCtrl.Pause)

	// ============================================================
	// 18. 销冠话术库 - 别名（前端用 /api/script-templates）
	// ============================================================
	scriptCtrl := contentctrl.NewScriptTemplateController()
	doReg("GET", "/script-templates", scriptCtrl.GetTemplateList)
	doReg("GET", "/script-templates/list", scriptCtrl.GetTemplateList)
	doReg("GET", "/script-templates/categories", scriptCtrl.GetCategories)
	doReg("GET", "/script-templates/:id", scriptCtrl.GetTemplateByID)
	doReg("POST", "/script-templates", scriptCtrl.CreateTemplate)
	doReg("PUT", "/script-templates/:id", scriptCtrl.UpdateTemplate)
	doReg("DELETE", "/script-templates/:id", scriptCtrl.DeleteTemplate)
	doReg("GET", "/script-templates/search", scriptCtrl.SearchTemplates)
	doReg("GET", "/script-templates/public", scriptCtrl.GetPublicTemplates)
	doReg("POST", "/script-templates/recommend", scriptCtrl.RecommendScript)

	// ============================================================
	// 19. 坐席状态 - 别名（前端用 /api/agent-statuses）
	// ============================================================
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

	// ============================================================
	// 20. 快捷回复 - 别名
	// ============================================================
	quickReplyCtrl := controller.NewQuickReplyController()
	doReg("GET", "/quick-replies", quickReplyCtrl.GetReplies)
	doReg("GET", "/quick-replies/list", quickReplyCtrl.GetReplies)
	doReg("GET", "/quick-replies/categories", quickReplyCtrl.GetReplyCategories)
	doReg("POST", "/quick-replies", quickReplyCtrl.CreateReply)
	doReg("PUT", "/quick-replies/:id", quickReplyCtrl.UpdateReply)
	doReg("DELETE", "/quick-replies/:id", quickReplyCtrl.DeleteReply)

	// ============================================================
	// 21. 会话标签 - 别名
	// ============================================================
	sessionTagCtrl := controller.NewSessionTagController()
	doReg("GET", "/session-tags", sessionTagCtrl.GetTags)
	doReg("GET", "/session-tags/list", sessionTagCtrl.GetTags)
	doReg("POST", "/session-tags", sessionTagCtrl.CreateTag)
	doReg("PUT", "/session-tags/:id", sessionTagCtrl.UpdateTag)
	doReg("DELETE", "/session-tags/:id", sessionTagCtrl.DeleteTag)

	// ============================================================
	// 22. AI 建议 - 别名（前端用 /api/ai-suggestions）
	// ============================================================
	aiSuggestionCtrl := controller.NewAISuggestionController()
	doReg("GET", "/ai-suggestions", aiSuggestionCtrl.GetSuggestions)
	doReg("GET", "/ai-suggestions/list", aiSuggestionCtrl.GetSuggestions)
	doReg("GET", "/ai-suggestions/:session_id", aiSuggestionCtrl.GetSuggestions)
	doReg("POST", "/ai-suggestions/:id/use", aiSuggestionCtrl.UseSuggestion)

	// ============================================================
	// 23. 异议处理 - 别名（前端用 /api/objection-templates）
	// ============================================================
	objCtrl := controller.NewObjectionHandlerController()
	doReg("GET", "/objection-templates", objCtrl.ListCategories)
	doReg("GET", "/objection-templates/list", objCtrl.ListCategories)
	doReg("POST", "/objection-templates/handle", objCtrl.Handle)
	doReg("POST", "/objection-templates/classify", objCtrl.Classify)
	doReg("POST", "/objection-templates/usage", objCtrl.RecordUsage)

	// ============================================================
	// 24. 销冠画像 - 别名（前端用 /api/sales-champions）
	// ============================================================
	personaCtrl := controller.NewSalesPersonaController()
	doReg("GET", "/sales-champions", personaCtrl.ListStaffs)
	doReg("GET", "/sales-champions/list", personaCtrl.ListStaffs)
	doReg("GET", "/sales-champions/:id", personaCtrl.GetReport)

	// ============================================================
	// 25. 邮件 - 别名
	// ============================================================
	emailListCtrl := controller.NewEmailListController()
	doReg("GET", "/email/lists", emailListCtrl.GetEmailListList)
	doReg("GET", "/email/lists/list", emailListCtrl.GetEmailListList)
	doReg("GET", "/email/lists/:id", emailListCtrl.GetEmailListDetail)
	doReg("POST", "/email/lists", emailListCtrl.CreateEmailList)
	doReg("PUT", "/email/lists/:id", emailListCtrl.UpdateEmailList)
	doReg("DELETE", "/email/lists/:id", emailListCtrl.DeleteEmailList)
	doReg("GET", "/email/lists/:id/trace", emailListCtrl.TraceEmail)

	emailSmtpCtrl := controller.NewEmailSmtpController()
	doReg("GET", "/email/smtps", emailSmtpCtrl.GetEmailSmtpList)
	doReg("GET", "/email/smtps/list", emailSmtpCtrl.GetEmailSmtpList)
	doReg("GET", "/email/smtps/:id", emailSmtpCtrl.GetEmailSmtp)
	doReg("POST", "/email/smtps", emailSmtpCtrl.CreateEmailSmtp)
	doReg("PUT", "/email/smtps/:id", emailSmtpCtrl.UpdateEmailSmtp)
	doReg("DELETE", "/email/smtps/:id", emailSmtpCtrl.DeleteEmailSmtp)

	// ============================================================
	// 26. 短信 - 别名（前端用 /api/sms/*）
	// ============================================================
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

	// ============================================================
	// 27-30. 抖音/快手/小红书/闲鱼 卡片 - 别名
	// ============================================================
	{
		douyinCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(db.GetDB()))
		doReg("GET", "/douyin-cards", douyinCtrl.GetList)
		doReg("GET", "/douyin-cards/list", douyinCtrl.GetList)
		doReg("GET", "/douyin-cards/:id", douyinCtrl.GetByID)
		doReg("POST", "/douyin-cards", douyinCtrl.Create)
		doReg("PUT", "/douyin-cards/:id", douyinCtrl.Update)
		doReg("DELETE", "/douyin-cards/:id", douyinCtrl.Delete)
	}
	{
		kuaishouCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(db.GetDB()))
		doReg("GET", "/kuaishou-cards", kuaishouCtrl.GetList)
		doReg("GET", "/kuaishou-cards/list", kuaishouCtrl.GetList)
		doReg("GET", "/kuaishou-cards/:id", kuaishouCtrl.GetByID)
		doReg("POST", "/kuaishou-cards", kuaishouCtrl.Create)
		doReg("PUT", "/kuaishou-cards/:id", kuaishouCtrl.Update)
		doReg("DELETE", "/kuaishou-cards/:id", kuaishouCtrl.Delete)
	}
	{
		xhsCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(db.GetDB()))
		doReg("GET", "/xiaohongshu-cards", xhsCtrl.GetList)
		doReg("GET", "/xiaohongshu-cards/list", xhsCtrl.GetList)
		doReg("GET", "/xiaohongshu-cards/:id", xhsCtrl.GetByID)
		doReg("POST", "/xiaohongshu-cards", xhsCtrl.Create)
		doReg("PUT", "/xiaohongshu-cards/:id", xhsCtrl.Update)
		doReg("DELETE", "/xiaohongshu-cards/:id", xhsCtrl.Delete)
	}
	{
		xianyuCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(db.GetDB()), service.NewXianyuCardStatsService(db.GetDB()))
		doReg("GET", "/xianyu-cards", xianyuCtrl.GetList)
		doReg("GET", "/xianyu-cards/list", xianyuCtrl.GetList)
		doReg("GET", "/xianyu-cards/:id", xianyuCtrl.GetByID)
		doReg("POST", "/xianyu-cards", xianyuCtrl.Create)
		doReg("PUT", "/xianyu-cards/:id", xianyuCtrl.Update)
		doReg("DELETE", "/xianyu-cards/:id", xianyuCtrl.Delete)
	}

	// ============================================================
	// 31. TikTok 卡片 - 别名
	// ============================================================
	tiktokCtrl := controller.NewTikTokCardController(service.NewTikTokCardServiceWithDB(db.GetDB()))
	doReg("GET", "/tiktok-cards", tiktokCtrl.List)
	doReg("GET", "/tiktok-cards/list", tiktokCtrl.List)
	doReg("GET", "/tiktok-cards/:id", tiktokCtrl.Get)
	doReg("POST", "/tiktok-cards", tiktokCtrl.Create)
	doReg("PUT", "/tiktok-cards/:id", tiktokCtrl.Update)
	doReg("DELETE", "/tiktok-cards/:id", tiktokCtrl.Delete)

	// ============================================================
	// 32. 飞书 - 别名
	// ============================================================
	feishuCtrl := controller.NewFeishuAccountController(service.NewFeishuService(db.GetDB()), service.NewFeishuIntegrationService(db.GetDB()))
	doReg("GET", "/feishu/accounts", feishuCtrl.List)
	doReg("GET", "/feishu/accounts/list", feishuCtrl.List)
	doReg("GET", "/feishu/accounts/:id", feishuCtrl.Get)
	doReg("POST", "/feishu/accounts", feishuCtrl.Create)
	doReg("PUT", "/feishu/accounts/:id", feishuCtrl.Update)
	doReg("DELETE", "/feishu/accounts/:id", feishuCtrl.Delete)

	// ============================================================
	// 33. Telegram - 别名
	// ============================================================
	tgCtrl := controller.NewTelegramAccountController(service.NewTelegramService(db.GetDB()))
	doReg("GET", "/telegram/accounts", tgCtrl.List)
	doReg("GET", "/telegram/accounts/list", tgCtrl.List)
	doReg("GET", "/telegram/accounts/:id", tgCtrl.Get)
	doReg("POST", "/telegram/accounts", tgCtrl.Create)
	doReg("PUT", "/telegram/accounts/:id", tgCtrl.Update)
	doReg("DELETE", "/telegram/accounts/:id", tgCtrl.Delete)

	// ============================================================
	// 34. 短链 - 别名（前端用 /api/short-links）
	// ============================================================
	shortLinkCtrl := controller.NewShortLinkController(service.NewShortLinkService(db.GetDB()))
	doReg("GET", "/short-links", shortLinkCtrl.GetList)
	doReg("GET", "/short-links/list", shortLinkCtrl.GetList)
	doReg("GET", "/short-links/:id", shortLinkCtrl.GetByID)
	doReg("POST", "/short-links", shortLinkCtrl.Create)
	doReg("PUT", "/short-links/:id", shortLinkCtrl.Update)
	doReg("DELETE", "/short-links/:id", shortLinkCtrl.Delete)

	// ============================================================
	// 35. 活码 - 别名（前端用 /api/live-codes）
	// ============================================================
	liveCodeCtrl := controller.NewLiveCodeController(service.NewLiveCodeService(db.GetDB()))
	doReg("GET", "/live-codes", liveCodeCtrl.GetList)
	doReg("GET", "/live-codes/list", liveCodeCtrl.GetList)
	doReg("GET", "/live-codes/:id", liveCodeCtrl.GetByID)
	doReg("POST", "/live-codes", liveCodeCtrl.Create)
	doReg("PUT", "/live-codes/:id", liveCodeCtrl.Update)
	doReg("DELETE", "/live-codes/:id", liveCodeCtrl.Delete)

	// ============================================================
	// 36. RAG - 别名（前端用 /api/rag-product-configs）
	// ============================================================
	doReg("GET", "/rag-product-configs", emptyObj)
	doReg("GET", "/rag-product-configs/list", emptyList)

	// ============================================================
	// 37. AI 内容创作 - 别名
	// ============================================================
	aiContentCtrl := contentctrl.NewAIContentController()
	doReg("GET", "/ai-content/list", aiContentCtrl.GetGenerationHistory)
	doReg("GET", "/ai-content/history", aiContentCtrl.GetGenerationHistory)
	doReg("GET", "/ai-content/templates", aiContentCtrl.GetTemplates)
	doReg("GET", "/ai-content/templates/:id", aiContentCtrl.GetTemplateByID)
	doReg("POST", "/ai-content/generate", aiContentCtrl.GenerateContent)
	doReg("POST", "/ai-content", aiContentCtrl.CreateHistory)
	doReg("GET", "/ai-content/:id", aiContentCtrl.GetRecordByID)
	doReg("PUT", "/ai-content/:id/save", aiContentCtrl.SaveRecord)
	doReg("POST", "/ai-content/:id/favorite", aiContentCtrl.FavoriteRecord)
	doReg("POST", "/ai-content/:id/rate", aiContentCtrl.RateRecord)
	doReg("DELETE", "/ai-content/:id", aiContentCtrl.DeleteRecord)

	// ============================================================
	// 38. 模板市场 - 别名
	// ============================================================
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

	// ============================================================
	// 39. 数据大屏 - 别名
	// ============================================================
	dashCtrl := opsctrl.NewDashboardScreenController()
	doReg("GET", "/dashboard-screens", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screens/list", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screens/:id", dashCtrl.GetScreenByID)
	doReg("POST", "/dashboard-screens", dashCtrl.CreateScreen)
	doReg("PUT", "/dashboard-screens/:id", dashCtrl.UpdateScreen)
	doReg("DELETE", "/dashboard-screens/:id", dashCtrl.DeleteScreen)
	doReg("GET", "/dashboard-screens/:id/data", dashCtrl.GetDashboardData)
	doReg("GET", "/dashboard-screens/:id/activities", dashCtrl.GetRealtimeActivities)
	// 兼容前端 /api/dashboards/* 路径
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
	// 兼容单数形式 /api/dashboard-screen/list
	doReg("GET", "/dashboard-screen/list", dashCtrl.GetScreenList)
	doReg("GET", "/dashboard-screen", dashCtrl.GetScreenList)

	// ============================================================
	// 40. 转化漏斗 - 别名
	// ============================================================
	funnelCtrl := opsctrl.NewConversionFunnelController()
	doReg("GET", "/conversion-funnels", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnels/list", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnels/stage", funnelCtrl.GetStageDetails)
	// 兼容前端 /api/analytics/funnel 与 /api/conversion-funnel/* 路径
	doReg("GET", "/analytics/funnel", funnelCtrl.GetFunnel)
	doReg("GET", "/analytics/funnel/stage", funnelCtrl.GetStageDetails)
	doReg("GET", "/conversion-funnel", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnel/list", funnelCtrl.GetFunnel)
	doReg("GET", "/conversion-funnel/stage", funnelCtrl.GetStageDetails)

	// ============================================================
	// 41. AI 产能 - 别名
	// ============================================================
	aiProdCtrl := opsctrl.NewAIProductivityController()
	doReg("GET", "/ai-productivity/overview", aiProdCtrl.GetReport)
	doReg("GET", "/ai-productivity/trend", aiProdCtrl.GetDailyTrend)

	// ============================================================
	// 42. 客户旅程 - 别名
	// ============================================================
	journeyCtrl := controller.NewCustomerJourneyController()
	doReg("GET", "/customer-journey/dashboard", journeyCtrl.GetOverview)
	doReg("GET", "/customer-journey/overview", journeyCtrl.GetOverview)
	doReg("GET", "/customer-journey/stages", journeyCtrl.ListStages)
	doReg("GET", "/customer-journey/by-stage", journeyCtrl.ListByStage)
	doReg("POST", "/customer-journey/touch", journeyCtrl.TouchCustomer)
	doReg("POST", "/customer-journey/transition", journeyCtrl.TransitionStage)

	// ============================================================
	// 43. 站点设置 - 别名
	// 已有 system/configs 在 system_routes.go 中
	// ============================================================
	sysCfgCtrl := controller.NewSystemConfigController()
	doReg("GET", "/system/configs", sysCfgCtrl.GetConfig)
	doReg("GET", "/system/configs/list", sysCfgCtrl.GetConfig)
	doReg("GET", "/system/configs/:key", sysCfgCtrl.GetConfig)
	doReg("PUT", "/system/configs/:key", sysCfgCtrl.SaveConfig)
	// /api/system/reset 已在 setupSystemAdminRoutes 注册，doReg 会自动跳过

	// ============================================================
	// 44. 存储配置 - 别名
	// ============================================================
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

	// ============================================================
	// 45. 素材库 - 别名
	// ============================================================
	doReg("GET", "/material-library/list", emptyList)
	doReg("GET", "/material-library", emptyList)
	doReg("GET", "/material-library/categories", emptyList)
	doReg("GET", "/material-library/stats", emptyObj)

	// ============================================================
	// 46. 系统监控 - 别名
	// ============================================================
	doReg("GET", "/system-monitor/metrics", emptyObj)
	doReg("GET", "/system-monitor/health", emptyObj)

	// ============================================================
	// 47. 域名池 - 别名
	// ============================================================
	domainDB := db.GetDB()
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

	// ============================================================
	// 48. 团队成员 - 别名
	// ============================================================
	teamCtrl := controller.NewTeamUserController()
	doReg("GET", "/team-users", teamCtrl.GetList)
	doReg("GET", "/team-users/list", teamCtrl.GetList)
	doReg("GET", "/team-users/:id", teamCtrl.GetByID)
	doReg("POST", "/team-users", teamCtrl.Create)
	doReg("PUT", "/team-users/:id", teamCtrl.Update)
	doReg("DELETE", "/team-users/:id", teamCtrl.Delete)
	doReg("POST", "/team-users/:id/reset-password", teamCtrl.ResetPassword)

	// ============================================================
	// 49. 授权管理 - 别名（开源版：License 模型已删除，全部移除）
	// ============================================================
	// 开源版：以下 License 相关路由全部下线（License 模型删除，授权流程移除）。
	// 保留注释作为历史变更记录，前端对应入口已通过页面级删除清理。

	// ============================================================
	// 50. OTA 升级 - 别名（开源版：OTA 已下线，全部移除）
	// ============================================================
	// 开源版：以下 OTA 升级路由全部下线（OTA 模型删除，升级流程移除）。
	// 保留注释作为历史变更记录，前端对应入口已通过页面级删除清理。

	// 注意：原 /api/platform/version/* 由 setupPlatformRoutes 负责注册（平台端路由组）。
	// 平台端 version 路由已在 setupPlatformRoutes 中同步删除。


	// ============================================================
	// 51. 支付 - 别名
	// ============================================================
	doReg("GET", "/payments", orderCtrl.GetOrderList)
	doReg("GET", "/payments/list", orderCtrl.GetOrderList)
	doReg("GET", "/payment/config", smsCtrl.GetConfig)
	doReg("GET", "/payment-configs", smsCtrl.GetConfig)
	doReg("GET", "/payment-configs/list", smsCtrl.GetConfig)

	// ============================================================
	// 52. 第三方对接 - 别名
	// ============================================================
	integrationCtrl := controller.NewIntegrationController()
	doReg("GET", "/integrations/list", integrationCtrl.GetAccountList)
	doReg("GET", "/integrations/:id", integrationCtrl.GetAccountByID)
	doReg("POST", "/integrations", integrationCtrl.CreateAccount)
	doReg("PUT", "/integrations/:id", integrationCtrl.UpdateAccount)
	doReg("DELETE", "/integrations/:id", integrationCtrl.DeleteAccount)
	doReg("GET", "/integrations/:id/sync-logs", integrationCtrl.GetSyncLogs)
	doReg("POST", "/integrations/:id/test", integrationCtrl.TestIntegration)
	doReg("POST", "/integrations/:id/sync/customers", integrationCtrl.SyncCustomers)
	doReg("POST", "/integrations/:id/sync/orders", integrationCtrl.SyncOrders)
	doReg("POST", "/integrations/:id/sync/products", integrationCtrl.SyncProducts)

	// ============================================================
	// 53. 操作日志 - 别名
	// ============================================================
	opLogCtrl := controller.NewOperationLogController()
	doReg("GET", "/operation-logs", opLogCtrl.GetList)
	doReg("GET", "/operation-logs/list", opLogCtrl.GetList)
	doReg("GET", "/operation-logs/:id", opLogCtrl.GetByID)
	doReg("GET", "/operation-logs/statistics", opLogCtrl.GetStatistics)

	// ============================================================
	// 54. 备份恢复 - 别名
	// ============================================================
	backupCtrl := controller.NewBackupController()
	doReg("GET", "/backups", backupCtrl.GetBackupList)
	doReg("GET", "/backups/list", backupCtrl.GetBackupList)
	doReg("GET", "/backups/:id", backupCtrl.GetBackupByID)

	// ============================================================
	// 55. 流失预测 - 别名
	// ============================================================
	churnCtrl := opsctrl.NewChurnPredictionController()
	doReg("GET", "/churn-prediction", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn-prediction/list", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn-prediction/users", churnCtrl.GetHighRiskUsers)
	doReg("GET", "/churn-prediction/warnings", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn-prediction/statistics", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn-prediction/model-config", churnCtrl.GetModelConfig)
	doReg("POST", "/churn-prediction/model-config", churnCtrl.SaveModelConfig)
	// 兼容前端 /api/churn/* 路径
	doReg("GET", "/churn/prediction", churnCtrl.GetChurnPrediction)
	doReg("GET", "/churn/predictions", churnCtrl.GetChurnPredictions)
	doReg("GET", "/churn/high-risk-users", churnCtrl.GetHighRiskUsers)
	doReg("GET", "/churn/warnings", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn/unhandled-warnings", churnCtrl.GetUnhandledWarnings)
	doReg("GET", "/churn/model-config", churnCtrl.GetModelConfig)
	doReg("POST", "/churn/model-config", churnCtrl.SaveModelConfig)
	doReg("GET", "/churn/statistics", churnCtrl.GetChurnWarnings)
	doReg("GET", "/churn/risk-distribution", churnCtrl.GetChurnWarnings)
	doReg("POST", "/backups", backupCtrl.CreateBackup)
	doReg("DELETE", "/backups/:id", backupCtrl.DeleteBackup)
}
