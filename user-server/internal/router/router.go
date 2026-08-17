package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

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
// allowedCORSOrigins 允许的 Web 源白名单（逗号分隔），来自环境变量 CORS_ALLOW_ORIGINS。
// 未配置时仅放行 Chrome 扩展源（见 corsMiddleware），拒绝任意 Web 源携带凭据，
// 修复"反射任意 Origin + 凭据"导致的 CSRF/凭据窃取漏洞（P1）。
var allowedCORSOrigins = parseCORSOrigins(os.Getenv("CORS_ALLOW_ORIGINS"))

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
//   - 配置在 CORS_ALLOW_ORIGINS 中的 Web 源放行。
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
				allow = true
			default:
				for _, a := range allowedCORSOrigins {
					if a == origin {
						allow = true
						break
					}
				}
			}
		}
		if allow {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Requested-With,X-Trace-Id")
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
	app.RegisterAgentReachTools(gormDB)
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
	}

	setupChatPublicWebSocket(r, langResolver)

	setupCardShareRoutes(r, gormDB)

	setupEmbedStaticRoutes(r)

	auth := r.Group("/api")
	auth.Use(middleware.InitGuard()) 
	auth.Use(middleware.JWTAuthMiddleware()) 
	{
		setupAuthRoutes(auth, gormDB)

		setupUserRoutes(auth)

		setupAccountRoutes(auth)

		setupShortLinkRoutes(auth, public, gormDB)

		setupLiveCodeRoutes(auth, liveCodeController)

		setupEmailRoutes(auth, gormDB)

		setupSmsRoutes(auth, gormDB)

		setupCardRoutes(auth, gormDB)

		setupCardStatsRoutes(auth, gormDB)

		setupDomainPoolRoutes(auth, gormDB)

		setupMaterialRoutes(auth)

		setupClueRoutes(auth)
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

	douyinLeadMiner := service.NewWebhookService(gormDB).DouyinLeadMiner()
	bridgeHandler.SetLeadMiner(douyinLeadMiner)

	bridgeWS := r.Group("/api")
		bridgeWS.Use(middleware.InitGuard())
		bridgeWS.POST("/bridge/ingest", bridgeHandler.HandleHTTPIngest)
		bridgeWS.GET("/bridge/outbox", bridgeHandler.GetBridgeOutbox)
		bridgeWS.POST("/bridge/outbox/ack", bridgeHandler.AckBridgeOutbox)

		channelPipeline := channelgw.NewPipeline(bridgeIngressSvc)
		channelWSTransport := channelgw.NewWSTransport(channelPipeline, channelgw.Default)
		bridgeWS.GET("/ws/channel", channelWSTransport.HandleWS)
		bridgeWS.POST("/bridge/ai-selectors", bridge.AISelectors)

		monitor.RegisterRoutes(bridgeWS)

		tlCtrl := controller.NewTraceLearningController(trace_learning.Global())
		bridgeWS.POST("/monitor/trace-eval/trigger", tlCtrl.TriggerEval)
		bridgeWS.GET("/monitor/trace-eval/logs", tlCtrl.EvalLogs)
		bridgeWS.GET("/monitor/knowledge-weights", tlCtrl.KnowledgeWeights)

		tracing.Init(gormDB)
		tooluse.ToolTraceSink = tracing.ReportToolCall

		bridgeRepo := bridge.NewBridgeAccountRepository(gormDB)
		bridge.RegisterBridgeAccountRepo(bridgeRepo)
		bridge.RegisterOwnershipChecker(func(ctx context.Context, userID uint, channel, accountID string) (bool, error) {
			acc, err := bridgeRepo.GetByChannelAccount(ctx, channel, accountID)
			if err == gorm.ErrRecordNotFound || acc == nil {
				return true, nil
			}
			if err != nil {
				return false, err
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

