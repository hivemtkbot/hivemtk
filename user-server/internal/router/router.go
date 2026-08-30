package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/app"
	"hivemtk-user/internal/bridge"
	channelgw "hivemtk-user/internal/channelgw"
	contentservice "hivemtk-user/internal/content/service"
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/monitor"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/trace_learning"
	"hivemtk-user/internal/service/translation"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// HealthRedis 供健康检查/就绪检查探测的 Redis 客户端；未配置（REDIS_HOST 为空）时为 nil，
// 此时健康检查 redis 状态显示 not_configured（与单实例默认行为一致）。
// allowedCORSOrigins 允许的 Web 源白名单（逗号分隔），来自环境变量 CORS_ALLOW_ORIGINS_USER。
// 命名与 platform-server 的 CORS_ALLOW_ORIGINS_PLATFORM 对称（user 端后缀 _USER）。
// 未配置时仅放行 Chrome 扩展源（见 corsMiddleware），拒绝任意 Web 源携带凭据，
// 修复"反射任意 Origin + 凭据"导致的 CSRF/凭据窃取漏洞（P1）。
var allowedCORSOrigins = parseCORSOrigins(os.Getenv("CORS_ALLOW_ORIGINS_USER"))

// sseAllowedCORSOrigins SSE 端点额外允许的跨域源白名单（逗号分隔），
// 来自环境变量 SSE_CORS_ALLOW_ORIGINS。
// 最高标准审计 P1-1 修复：SSE 端点不再对任意 Origin 反射 ACAO+credentials，
// 仅放行「同源推断命中」或显式配置在白名单中的源。
var sseAllowedCORSOrigins = parseCORSOrigins(os.Getenv("SSE_CORS_ALLOW_ORIGINS"))

// requestScheme 推断请求协议（优先反代透传头，其次 TLS 状态）
func requestScheme(r *http.Request) string {
	if r == nil {
		return "https"
	}
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		return proto
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// isSameOrigin 判断请求 Origin 是否与请求 Host 同源（读 Host 推断）。
// 最高标准审计 P1-1：SSE 白名单的第一优先级——同源请求本就无需 CORS，
// 反射同源 Origin 不构成跨域凭据暴露面。
func isSameOrigin(origin string, r *http.Request) bool {
	if origin == "" || r == nil || r.Host == "" {
		return false
	}
	return origin == requestScheme(r)+"://"+r.Host
}

// sseOriginAllowed SSE 端点 Origin 放行判定：
//   - 与请求 Host 同源 → 放行
//   - 命中 SSE_CORS_ALLOW_ORIGINS 或全局 CORS_ALLOW_ORIGINS_USER 白名单 → 放行
//   - 其余一律拒绝（不再通配反射）
//
// 浏览器扩展场景走上方 chrome-extension:// 分支，不受此函数影响；
// Content Script 若需在任意网页下连接 SSE，运维应显式配置 SSE_CORS_ALLOW_ORIGINS。
func sseOriginAllowed(origin string, r *http.Request) bool {
	if isSameOrigin(origin, r) {
		return true
	}
	if origin == "" {
		return false
	}
	for _, list := range [][]string{sseAllowedCORSOrigins, allowedCORSOrigins} {
		for _, a := range list {
			if a == origin {
				return true
			}
		}
	}
	return false
}

func parseCORSOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// corsMiddleware 处理跨域请求（CORS）。
//
// 安全策略（P1 修复）：
//   - 浏览器扩展从 chrome-extension://<id> 源发起请求，按源反射放行（扩展为预期调用方）。
//   - 配置在 CORS_ALLOW_ORIGINS_USER 中的 Web 源放行。
//   - SSE 端点（/api/bridge/outbox/sse）仅放行同源（按请求 Host 推断）与
//     SSE_CORS_ALLOW_ORIGINS / CORS_ALLOW_ORIGINS_USER 白名单源（最高标准审计 P1-1 修复）。
//   - 其余任意 Origin 一律不返回 ACAO（浏览器将阻止带凭据的跨域调用），杜绝任意网页
//     借用户浏览器凭据调用敏感 API。
//
// 此前实现直接回显请求 Origin 且附带 Access-Control-Allow-Credentials: true，等同于
// 对任意网站开放凭据型跨域访问。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		allow := false
		if origin != "" {
			switch {
			case strings.HasPrefix(origin, "chrome-extension://"):
				// v3 审计 P1-5：扩展 ID 白名单配置化（逗号分隔完整 origin，如
				// chrome-extension://abcdef...）。未配置时保留旧行为（放行任意扩展）
				// 以兼容存量浏览器扩展；生产建议显式配置 CORS_ALLOWED_EXTENSIONS 收敛。
				allowedExts := strings.Split(os.Getenv("CORS_ALLOWED_EXTENSIONS"), ",")
				if len(allowedExts) > 0 && allowedExts[0] != "" {
					for _, ext := range allowedExts {
						if origin == strings.TrimSpace(ext) {
							allow = true
							break
						}
					}
				} else {
					allow = true
				}
			default:
				// 最高标准审计 P1-1 修复：SSE 端点不再对任意 Origin 放行。
				// 原实现对 /outbox/sse 通配反射 ACAO 且带 credentials=true，
				// 一旦任何端点引入 Cookie 会话即升级为凭据型 CSRF。
				// 新策略：同源（按 Host 推断）或显式白名单才放行。
				fullPath := c.Request.URL.Path
				if strings.HasSuffix(fullPath, "/outbox/sse") || strings.Contains(fullPath, "outbox/sse") {
					allow = sseOriginAllowed(origin, c.Request)
				} else {
					for _, a := range allowedCORSOrigins {
						if a == origin {
							allow = true
							break
						}
					}
				}
			}
		}
		if allow {
		c.Header("Access-Control-Allow-Origin", origin)
		c.Header("Vary", "Origin")
	}
	c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Requested-With,X-Trace-Id,Last-Event-ID,Cache-Control")
		c.Header("Access-Control-Expose-Headers", "Last-Event-ID,X-Trace-Id")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

var HealthRedis Pinger

// SetHealthRedis 由 main 在启动时注入 Redis 客户端（仅当 REDIS_HOST 配置可达时）。
func SetHealthRedis(p Pinger) {
	HealthRedis = p
}

func Setup(r *gin.Engine, gormDB *gorm.DB) {
	// WhatsApp Cloud 账号服务（在函数级作用域声明，供 Webhook URL 验证与 Cloud 路由共用）
	var whatsappCloudSvc *service.WhatsAppCloudService
	var webhookSvc *service.WebhookService
	var dingtalkAppSvc *service.DingTalkAppService

	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    "METHOD_NOT_ALLOWED_405",
			"message": "请求方法不被支持",
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
		})
	})

	r.Use(corsMiddleware()) 
	r.Use(gin.Recovery())

	r.Use(middleware.LocaleMiddleware())

	r.Use(middleware.ContextMiddleware())

	app.InitEventBus()

	injectMiddlewarePorts()

	r.GET("/health", HealthCheck(HealthRedis, gormDB))
	r.GET("/healthz", LivenessCheck())
	r.GET("/readyz", ReadinessCheck(HealthRedis, gormDB))


	r.Use(middleware.RateLimitMiddleware(middleware.RateLimitConfig{
		RPS:        10,
		BucketSize: 100,
		Enabled:    true,
		ExemptPaths: []string{
			"/api/bridge/ingest",
			"/api/ws/channel",
		},
	}))

	r.Use(middleware.TraceMiddleware())

	r.Use(middleware.APIInteractionLogger())

	r.Use(middleware.AuditMiddleware())

	liveCodeController := controller.NewLiveCodeController(service.NewLiveCodeService(gormDB))

	platformCtrl := controller.NewPlatformController()

	app.InitGlobalToolExecutor()
	app.InitGlobalToolRouter()
	app.RegisterAllAgentTools(gormDB)
	app.InitInferenceOrchestrator()

	engine := app.BuildSalesEngine(gormDB)
	orchestrator := app.BuildSmartOrchestrator(engine)
	aiAgentSvcGlobal := service.NewAIAgentService()
	channelBindingSvcGlobal := service.NewChannelAgentBindingService()
	csAgentSvcGlobal := service.NewCustomerServiceAgentService()
	orchestrator.SetCustomerServiceAgentService(context.Background(), csAgentSvcGlobal)

	langResolver := translation.NewLangConfigResolver(
		repository.NewChatChannelRepository(),
		repository.NewAIAgentRepository(),
	)

	service.InitAssetResolver(gormDB)
	contentservice.SetWorkflowAssetResolver(func(ctx context.Context) (json.RawMessage, bool) {
		if r := service.GetAssetResolver(); r != nil {
			if w, ok := r.GetActiveWorkflow(ctx); ok && w != nil {
				if b, err := json.Marshal(w); err == nil {
					return b, true
				}
			}
		}
		return nil, false
	})

	public := r.Group("/api")
	{
		setupPublicRoutes(public, liveCodeController, platformCtrl, gormDB)
		setupChatPublicRoutes(public, gormDB, orchestrator, langResolver)
		setupSSORoutes(public, gormDB)
		setupSelfServiceRoutes(public, gormDB)
		// R48 T1: 公开帮助中心（免登录，仅 public_visible 文档白名单）
		hcCtrl := controller.NewHelpCenterController()
		public.GET("/public/help-center/categories", hcCtrl.Categories)
		public.GET("/public/help-center/articles", hcCtrl.Articles)
		public.GET("/public/help-center/articles/:id", hcCtrl.ArticleDetail)
	}

	setupChatPublicWebSocket(r, langResolver)

	setupCardShareRoutes(r, gormDB)

	setupEmbedStaticRoutes(r)

		auth := r.Group("/api")
	auth.Use(middleware.AICrawlerMonitor(func(engine, path, ua, ip string) {
		// AI 爬虫访问自动落库（异步不阻塞）
	})) 
	// 观测端点：JWT 保护（v3 审计 P1 从 bridgeWS 组迁入）
	monitor.RegisterRoutes(auth)

	// 桥接凭证管理（admin，JWT）：查询状态 / 轮换（v3 BRIDGE_TOKEN_PROTOCOL）
	// 注意必须挂在 BridgeIngressGuard 之外——它是凭证自身的引导/轮换入口
	//
	// Round32 复测修复：这三个路由注册在下方 auth.Use(JWTAuthMiddleware()) 之前，
	// 导致 RequireAdminMiddleware 取不到 role 恒 401（bridge/token 功能不可用），
	// 而 trace-eval/knowledge-weights 实际匿名可访问（写操作触发评估 + 数据泄露）。
	// 显式补挂 JWT（trigger 另加 admin）。
	bridgeTokenCtrl := controller.NewBridgeTokenController()
	auth.GET("/bridge/token/status", middleware.JWTAuthMiddleware(), middleware.RequireAdminMiddleware(), bridgeTokenCtrl.GetStatus)
	auth.POST("/bridge/token/reset", middleware.JWTAuthMiddleware(), middleware.RequireAdminMiddleware(), bridgeTokenCtrl.ResetBridgeToken)
	tlCtrl := controller.NewTraceLearningController(trace_learning.Global())
	auth.POST("/monitor/trace-eval/trigger", middleware.JWTAuthMiddleware(), middleware.RequireAdminMiddleware(), tlCtrl.TriggerEval)
	auth.GET("/monitor/trace-eval/logs", middleware.JWTAuthMiddleware(), tlCtrl.EvalLogs)
	auth.GET("/monitor/knowledge-weights", middleware.JWTAuthMiddleware(), tlCtrl.KnowledgeWeights)

	auth.Use(middleware.JWTAuthMiddleware()) 
	{
		setupAuthRoutes(auth, gormDB)

		setupUserRoutes(auth)

		setupAccountRoutes(auth)

		setupAlertRoutes(auth)

		setupShortLinkRoutes(auth, public, gormDB)

		setupLiveCodeRoutes(auth, liveCodeController)

		setupEmailRoutes(auth, gormDB)

		setupSmsRoutes(auth, gormDB)

		setupCardRoutes(auth, gormDB)

		setupCardStatsRoutes(auth, gormDB)

		setupDomainPoolRoutes(auth, gormDB)

		setupMaterialRoutes(auth)

		setupClueRoutes(auth)
		SetupGeoRoutes(auth, gormDB)
		setupLeadMiningRoutes(auth)

		setupCustomerRFMRoutes(auth)

		setupRecoveryQueueRoutes(auth)

		systemAdmin := auth.Group("")
		systemAdmin.Use(middleware.AdminAuthMiddleware())
		setupSystemRoutes(systemAdmin)

		setupSystemUserRoutes(auth)

		setupRoleRoutes(auth)

		setupPermissionRoutes(auth)

		setupRagRoutes(auth, gormDB)

		setupKnowledgeBaseRoutes(auth)

		setupWhatsappRoutes(auth, gormDB)

		whatsappCloudSvc = service.NewWhatsAppCloudService(gormDB)
		setupWhatsAppCloudRoutes(auth, whatsappCloudSvc, gormDB)

		webhookSvc = service.NewWebhookService(gormDB)
		dingtalkAppSvc = service.NewDingTalkAppService(gormDB, webhookSvc)

		setupDingTalkAppRoutes(auth, dingtalkAppSvc)

		setupTelegramRoutes(auth, gormDB)

		setupFeishuRoutes(auth, gormDB)

		bridgeIngressSvc := service.NewInboxIngressService()

		setupWechatRoutes(auth, gormDB, bridgeIngressSvc)

		setupTiktokRoutes(auth, gormDB)

		setupWeComRoutes(auth, gormDB)

		setupCustomerServiceRoutes(auth, aiAgentSvcGlobal, langResolver)

	app.SetBridgeIngressSvc(bridgeIngressSvc) 
	bridgeHandler := bridge.NewBridgeIngestHandler(bridgeIngressSvc)

	// Phase 1: 注入 OutboxQuerier（让 SSE fetcher 能查 DB）
	messageHubRepo := repository.NewMessageHubRepositoryWithDB(gormDB)
	bridgeHandler.SetOutboxQuerier(messageHubRepo)

	// Phase 1: 设置 service → bridge SSE 事件回调
	// service.DeliverBridgeOutbound 成功落库后，通过此回调通知 bridge.GlobalSSEBus
	service.SetGlobalSSEPublisher(func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time) {
		bridge.GlobalSSEBus.Publish(bridge.SSEEvent{
			ID:             strconv.FormatUint(hubID, 10),
			Event:          "new_outbound",
			ConversationID: convID,
			MsgType:        msgType,
			ReceiverID:     receiverID,
			Seq:            int(hubID),
			Data: map[string]any{
				"hub_id":          hubID,
				"platform":        channel,
				"account_id":      accountID,
				"conversation_id": convID,
				"content":         content,
				"msg_type":        msgType,
				"receiver_id":     receiverID,
				"is_ai_reply":     isAIReply,
			},
			Timestamp: createdAt,
		})
	})

	// Phase 1: 设置 GlobalBridgeReachAdapter（供 AI Agent reach.*.send 工具和主动外联使用）
	tooluseBridgeAdapter := bridge.NewBridgeReachAdapter(
		app.NewIntegrationReachAdapterFromDB(gormDB),
		bridgeIngressSvc,
	)
	bridge.GlobalBridgeReachAdapter = tooluseBridgeAdapter

	douyinLeadMiner := service.NewWebhookService(gormDB).DouyinLeadMiner()
	bridgeHandler.SetLeadMiner(douyinLeadMiner)

	bridgeWS := r.Group("/api")
		bridgeWS.Use(middleware.InitGuard())
		// v3 审计 P0-1：桥接通道凭证守卫（X-Bridge-Token，见 middleware.BridgeIngressGuard）
		bridgeWS.Use(middleware.BridgeIngressGuard())


		bridgeWS.POST("/bridge/ingest", bridgeHandler.HandleHTTPIngest)
		bridgeWS.GET("/bridge/outbox", bridgeHandler.GetBridgeOutbox)
		bridgeWS.POST("/bridge/outbox/ack", bridgeHandler.AckBridgeOutbox)

		// v3 审计：SSE 端点替换长轮询（业界：Twilio Flex / Intercom 实践）
		// 复用 bridgeHandler 的 outbox 数据源（DB）
		bridgeWS.GET("/bridge/outbox/sse", bridgeHandler.HandleOutboxSSE)

		// Capabilities 查询端点：供前端查询当前可用的传输方式
		// v3 审计整改：业务逻辑抽出至 controller/bridge_capabilities.go，router 仅做映射
		bridgeWS.GET("/bridge/capabilities", controller.NewBridgeCapabilitiesController().GetCapabilities)

		// v3 审计：MCP server（Model Context Protocol 2025-06-18）
		// 让 Claude Desktop / Cursor / Continue.dev 等客户端可直接连接 user-server 调用工具
		// 当前仅暴露 initialize/ping；tools/list+tools/call 需后续挂上 tooluse registry
		// v3 审计发现：mcpServer 不能是单例（initialize 状态会污染）；每次请求新建
		// v3 审计整改：JSON-RPC 处理逻辑抽出至 controller/mcp.go，router 仅做映射
		bridgeWS.POST("/mcp", controller.NewMCPController().Handle)

		channelPipeline := channelgw.NewPipeline(bridgeIngressSvc)
		channelWSTransport := channelgw.NewWSTransport(channelPipeline, channelgw.Default)
		bridgeWS.GET("/ws/channel", channelWSTransport.HandleWS)

		// v3 审计 P1 修复：monitor/trace-learning 是运营者观测端点（user-web 调用），
		// 不属于扩展桥接通道——迁出 bridgeWS 组（否则被 BridgeIngressGuard 拦截，
		// 前端无 X-Bridge-Token 必 401），改挂 JWT auth 组。

		tracing.Init(gormDB)
		tooluse.ToolTraceSink = tracing.ReportToolCall

		bridgeRepo := bridge.NewBridgeAccountRepository(gormDB)
		bridge.RegisterBridgeAccountRepo(bridgeRepo)
		bridge.RegisterOwnershipChecker(func(ctx context.Context, userID uint, channel, accountID string) (bool, error) {
			acc, err := bridgeRepo.GetByChannelAccount(ctx, channel, accountID)
			// 最高标准审计 P1-1/P1-4 修复：归属校验 fail-closed。
			// 原实现账号不存在（ErrRecordNotFound/nil）时返回 true 放行——
			// 越权 ingest 反而因"查无此号"通过校验。现改为：
			//   - 账号不存在 → false（拒绝）
			//   - DB 其他错误 → false + err（调用方按 500 处理，同样拒绝）
			//   - 存在 → 严格比对 UserID
			if err != nil {
				return false, err
			}
			if acc == nil {
				return false, nil
			}
			return acc.UserID == userID, nil
		})
		bridgeAccountCtrl := controller.NewBridgeAccountController()
		bridgeAccountCtrl.RegisterRoutes(auth)

		if bridgeIngressSvc != nil && webhookSvc != nil {
			bridgeIngressSvc.SetAITrigger(webhookSvc)
			webhookSvc.SetIngressSvc(bridgeIngressSvc)
			logger.Infof("[Bridge] bridge AITrigger 已注入（抖音/小红书/TikTok 网页私信 AI 链路已连通）")
		}
		if bridgeIngressSvc != nil {
			bridgeIngressSvc.SetInboxService(service.NewInboxService())
			logger.Infof("[Bridge] bridge InboxService 已注入（统一收件箱会话同步已连通）")
			bridgeIngressSvc.SetLeadMining(service.NewLeadMiningService())
			logger.Infof("[Bridge] bridge LeadMining 已注入（线索发掘异步监听已连通）")
		}

		setupChatChannelAdminRoutes(auth, gormDB)

		setupI18nRoutes(auth, gormDB)

		setupEventRoutes(auth)

		setupMessageRoutes(auth, gormDB)

		setupPlatformAccountRoutes(auth)

		setupWeComHealthRoutes(auth, gormDB)

		setupIntentRoutes(auth, gormDB)

		setupDialogueMemoryRoutes(auth, gormDB)

		setupReachPipelineRoutes(auth, gormDB)

		setupProactiveReachRoutes(auth, gormDB)

		// 2026-08-16 严肃化：13 渠道统一配置入口
		setupChannelOverviewRoutes(auth, gormDB)

	// 微信公众号 webhook（GET 验证 + POST 收消息）
	wechatWebhookGroup := r.Group("/api")
	setupWechatWebhookRoutes(wechatWebhookGroup, gormDB, bridgeIngressSvc)

		setupSOPRoutes(auth, gormDB)

		setupWorkflowOrchestratorRoutes(auth, gormDB)

		setupLLMRoutingRoutes(auth)

		setupLLMProviderRoutes(auth)
		setupTraceRoutes(auth)
		setupSSEDashboardRoutes(auth)

		setupAnalyticsRoutes(auth)

		setupObjectionHandlerRoutes(auth)

		setupCustomerJourneyRoutes(auth)

		setupQualityRoutes(auth)

		setupSecurityAuditRoutes(auth, gormDB)

		setupBatchRoutes(auth)

		setupAIContentRoutes(auth)

		setupUserSegmentRoutes(auth)

		setupMarketingFlowRoutes(auth)

		setupCustomReportRoutes(auth)

		setupDashboardRoutes(auth, public)

		setupTemplateRoutes(auth)

		setupScriptRoutes(auth)

		setupABTestRoutes(auth)

		setupTuningRoutes(auth)

		setupAssetMarketRoutes(auth, gormDB)

		setupAssetBundleRoutes(auth, gormDB)

		setupChurnRoutes(auth)

		setupIntegrationRoutes(auth)

		setupCommunityRoutes(auth)

		setupBackupRoutes(auth)

		setupMigrationRoutes(auth, gormDB)

		auth.POST("/upload", controller.UploadFile)

		setupToolDebugRoutes(auth)

		app.SetupToolPermissionRoutes(auth)

		setupAIToolConfigRoutes(auth, gormDB)

		app.SetupInferenceRoutes(auth)

		aiAgentCtrl := controller.NewAIAgentControllerWithService(aiAgentSvcGlobal)
		aiAgentCtrl.SetSalesEngine(engine)
		aiAgentCtrl.RegisterRoutes(auth)

		channelBindingCtrl := controller.NewChannelAgentBindingControllerWithService(channelBindingSvcGlobal)
		channelBindingCtrl.RegisterRoutes(auth)

		csAgentMountCtrl := controller.NewCustomerServiceAgentControllerWithService(csAgentSvcGlobal)
		csAgentMountCtrl.RegisterRoutes(auth)

		setupFrontendAliases(auth, r, gormDB)
	}

	webhookCtrl := controller.NewWebhookController(webhookSvc)
	webhookCtrl.SetWhatsAppCloudService(whatsappCloudSvc)
	webhookCtrl.SetDingTalkAppService(dingtalkAppSvc)
	webhookCtrl.SetFeishuService(service.NewFeishuService(gormDB))
	webhookCtrl.SetSalesEngine(engine)
	webhookCtrl.SetSmartOrchestrator(orchestrator)
	webhookCtrl.SetLangResolver(langResolver)
	webhookCtrl.RegisterRoutes(r)

	webhookCtrl.SetAgentBindingService(channelBindingSvcGlobal)

	go service.ReconcileTelegramWebhooks(service.NewTelegramService(gormDB))

	platform := r.Group("/api/platform")
	platform.Use(middleware.InitGuard()) 
	platform.Use(middleware.JWTAuthMiddleware())
	platform.Use(middleware.AdminAuthMiddleware())
	{
		setupPlatformRoutes(platform, platformCtrl)
	}
}

