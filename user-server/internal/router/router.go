package router

import (
	"marketing/internal/controller"
	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// HealthRedis 供健康检查/就绪检查探测的 Redis 客户端；未配置（REDIS_HOST 为空）时为 nil，
// 此时健康检查 redis 状态显示 not_configured（与单实例默认行为一致）。
var HealthRedis Pinger

// SetHealthRedis 由 main 在启动时注入 Redis 客户端（仅当 REDIS_HOST 配置可达时）。
func SetHealthRedis(p Pinger) {
	HealthRedis = p
}

func Setup(r *gin.Engine) {
	// 基础中间件
	r.Use(gin.Recovery())

	// 多语言：解析请求语言注入上下文，供业务层返回本地化提示
	r.Use(middleware.LocaleMiddleware())

	// P1-4: 初始化全局事件总线（在 Service 构造之前）
	// 试点：OperationLog 异步写入；后续可在 initEventBus 中追加订阅者
	initEventBus()

	// 健康检查端点（公开）
	r.GET("/health", HealthCheck(HealthRedis))
	r.GET("/healthz", LivenessCheck())
	r.GET("/readyz", ReadinessCheck(HealthRedis))

	// P0-19: Prometheus 指标暴露端点
	// P1-1 修复：仅允许 loopback / 私有网段访问；外部需配置 METRICS_TOKEN 走 Bearer 鉴权
	r.GET("/metrics", middleware.MetricsAuthMiddleware(), middleware.MetricsHandler)

	// P0-19: Prometheus 指标采集中间件
	r.Use(middleware.PrometheusMetricsMiddleware())

	// 安全中间件
	// 限流中间件 - 防止 DDoS 和暴力请求
	r.Use(middleware.RateLimitMiddleware(middleware.RateLimitConfig{
		RPS:        10,
		BucketSize: 100,
		Enabled:    true,
	}))

	// 追踪中间件必须最先注册（位于脱敏/审计之前）：为请求分配 trace_id 并绑定到 context，
	// 后续 handler / service / 编排 / 触达全链路复用同一 trace_id，便于线上定位问题。
	r.Use(middleware.TraceMiddleware())

	// 日志脱敏中间件 - 对敏感信息进行脱敏
	r.Use(middleware.SensitiveLogMiddleware())

	// 审计中间件（全局）
	r.Use(middleware.AuditMiddleware())

	// 活码控制器（需要在多个路由组中使用）
	liveCodeController := controller.NewLiveCodeController(service.NewLiveCodeService(db.GetDB()))

	// 平台控制器（需要在多个路由组中使用）
	platformCtrl := controller.NewPlatformController()

	// 多 AI 智能体架构：提前构建共享 service 和 SalesEngine
	// auth 路由组（智能体管理 CRUD）和 webhook 路由组（智能体路由）共享同一份实例
	//
	// 装配顺序（关键）：
	//   1) initGlobalToolExecutor()    —— 创建全局 ToolExecutor（含装饰器链：限流/重试/审计/计费）
	//   2) registerAllAgentTools(db)   —— 注册全部 41 个智能体工具到全局注册中心
	//                                      （reach×20 + pm×3 + customer×8 + knowledge×4 + business×6）
	//   3) buildSalesEngine(db)        —— 此时 GetGlobalExecutor() 返回非 nil，
	//                                      SalesEngine 注入 ToolExecutorAdapter 后 Agent Loop (ReAct) 激活
	initGlobalToolExecutor()
	registerAllAgentTools(db.GetDB())

	engine := buildSalesEngine(db.GetDB())
	orchestrator := buildSmartOrchestrator(engine)
	aiAgentSvcGlobal := service.NewAIAgentService(db.GetDB())
	channelBindingSvcGlobal := service.NewChannelAgentBindingService(db.GetDB(), aiAgentSvcGlobal)
	csAgentSvcGlobal := service.NewCustomerServiceAgentService(db.GetDB(), aiAgentSvcGlobal)
	// 注入到 SmartCSOrchestrator（客服座席挂载智能体路由）
	orchestrator.SetCustomerServiceAgentService(csAgentSvcGlobal)

	// 公开路由（不需要认证）
	public := r.Group("/api")
	{
		setupPublicRoutes(public, liveCodeController, platformCtrl, db.GetDB())
		// P0-10 ADR-010: 公开 chat API（AppKey 鉴权）
		setupChatPublicRoutes(public, db.GetDB(), orchestrator)
	}

	// P0-10 ADR-010: 访客 WebSocket（公开，无鉴权）
	setupChatPublicWebSocket(r)

	// 卡片分享路由（公开，不需要认证）
	setupCardShareRoutes(r)

	// P0-10 ADR-010: 静态文件服务（chat embed 页面 + embed SDK）
	// 私域部署：用户把 user-web/dist 部署到 user-server 同源，
	// 这样嵌入的 iframe 聊天窗可以无跨域问题加载。
	setupEmbedStaticRoutes(r)

	// 认证路由
	auth := r.Group("/api")
	auth.Use(middleware.InitGuard())         // 1) 系统必须已初始化
	auth.Use(middleware.LicenseGuard())      // 2) 授权必须有效
	auth.Use(middleware.JWTAuthMiddleware()) // 3) JWT 必须有效
	{
		// 认证相关
		setupAuthRoutes(auth)

		// 用户管理
		setupUserRoutes(auth)

		// 账户管理
		setupAccountRoutes(auth)

		// 短链管理
		setupShortLinkRoutes(auth)

		// 活码管理
		setupLiveCodeRoutes(auth, liveCodeController)

		// 邮件管理
		setupEmailRoutes(auth)

		// 短信管理
		setupSmsRoutes(auth)

		// 卡片管理（抖音、快手、小红书、闲鱼）
		setupCardRoutes(auth)

		// 卡片统计
		setupCardStatsRoutes(auth)

		// 自动回复
		setupAutoReplyRoutes(auth)

		// 域名池管理
		setupDomainPoolRoutes(auth)

		// 素材管理
		setupMaterialRoutes(auth)

		// 线索管理
		setupClueRoutes(auth)

		// H 域 P1: 客户 RFM 联动分层
		setupCustomerRFMRoutes(auth)

		// H 域 P1: 流失挽回队列
		setupRecoveryQueueRoutes(auth)

		// 订单管理
		setupOrderRoutes(auth)

		// 系统管理
		setupSystemRoutes(auth)

		// RAG 知识库
		setupRagRoutes(auth)

		// 知识库管理
		setupKnowledgeBaseRoutes(auth)

		// WhatsApp (Web 扫码)
		setupWhatsappRoutes(auth)

		// WhatsApp Cloud (Meta 商业 API)
		setupWhatsAppCloudRoutes(auth)

		// Telegram
		setupTelegramRoutes(auth)

		// 飞书
		setupFeishuRoutes(auth)

		// TikTok
		setupTiktokRoutes(auth)

		// 企业微信
		setupWeComRoutes(auth)

		// 客服会话管理
		setupCustomerServiceRoutes(auth)

		// P0-10 ADR-010: 客服 Web Widget 渠道管理（前端 ChatChannel.vue 列表/创建/编辑依赖）
		setupChatChannelAdminRoutes(auth, db.GetDB())

		// 客户事件追踪(CDP)
		setupEventRoutes(auth)

		// 统一消息管理
		setupMessageRoutes(auth, db.GetDB())

		// 平台账号管理
		setupPlatformAccountRoutes(auth)

		// 企微账号健康度
		setupWeComHealthRoutes(auth, db.GetDB())

		// 意图识别
		setupIntentRoutes(auth, db.GetDB())

		// 对话记忆
		setupDialogueMemoryRoutes(auth, db.GetDB())

		// 触达 Pipeline 框架
		setupReachPipelineRoutes(auth, db.GetDB())

		// SOP 智能体
		setupSOPRoutes(auth, db.GetDB())

		// LLM 多模型路由
		setupLLMRoutingRoutes(auth)

		// M 域 P1 缺口修复路由（2026-07-21）
		// M-1 LLM Provider 降级管理 / M-3 全链路追踪 / M-4 SSE 实时驾驶舱
		setupLLMProviderRoutes(auth)
		setupTraceRoutes(auth)
		setupSSEDashboardRoutes(auth)

		// 数据分析 (转化漏斗 + 智能体产能 + 智能体画像)
		setupAnalyticsRoutes(auth)

		// 异议处理
		setupObjectionHandlerRoutes(auth)

		// 客户旅程大屏 (G10)
		setupCustomerJourneyRoutes(auth)

		// 性能压测 + 安全审计
		setupQualityRoutes(auth)

		// 团队用户管理
		setupTeamRoutes(auth)

		// 批量操作
		setupBatchRoutes(auth)

		// AI 内容创作
		setupAIContentRoutes(auth)

		// 用户分层 RFM
		setupUserSegmentRoutes(auth)

		// 营销自动化流程
		setupMarketingFlowRoutes(auth)

		// 自定义报表
		setupCustomReportRoutes(auth)

		// 数据大屏
		setupDashboardRoutes(auth, public)

		// 模板市场
		setupTemplateRoutes(auth)

		// 话术库
		setupScriptRoutes(auth)

		// A/B 测试
		setupABTestRoutes(auth)

		// 置信度/拟人度/反馈学习 统一管理 API
		setupTuningRoutes(auth)

		// 流失预警
		setupChurnRoutes(auth)

		// 第三方对接
		setupIntegrationRoutes(auth)

		// 社群管理
		setupCommunityRoutes(auth)

		// 备份恢复
		setupBackupRoutes(auth)

		// 版本升级
		setupUpgradeRoutes(auth)

		// 文件上传
		auth.POST("/upload", controller.UploadFile)

		// 多 AI 智能体架构（MULTI_AI_AGENT_DESIGN）
		// 使用提前构建的共享 service 实例，确保 AIAgentService 缓存在所有使用方之间一致
		// 智能体管理（CRUD + 测试 + 上下文加载）
		aiAgentCtrl := controller.NewAIAgentControllerWithService(aiAgentSvcGlobal)
		aiAgentCtrl.SetSalesEngine(engine)
		aiAgentCtrl.RegisterRoutes(auth)

		// 渠道账号绑定智能体
		channelBindingCtrl := controller.NewChannelAgentBindingControllerWithService(channelBindingSvcGlobal)
		channelBindingCtrl.RegisterRoutes(auth)

		// 客服座席挂载智能体
		csAgentMountCtrl := controller.NewCustomerServiceAgentControllerWithService(csAgentSvcGlobal)
		csAgentMountCtrl.RegisterRoutes(auth)

		// P2-X 前端 API 路径别名（兼容前端调用习惯）
		// 必须放在所有 setup* 之后，避免更具体的路由被覆盖
		setupFrontendAliases(auth, r)
	}

	// P0-14 多渠道 Webhook 路由（公开，不需要鉴权）
	webhookCtrl := controller.NewWebhookController()
	// P0-A 修复：智能体引擎 8 步链路真实依赖注入
	// 不再 nil 注入，让 SalesEngine 真正调用 intent/memory/sop/rag/script/customer
	webhookCtrl.SetSalesEngine(engine)
	// Phase 3：注入智能体统一编排器（LLM + 客服座席结合体）
	// Webhook 入站消息优先走 SmartCSOrchestrator.HandleIncoming 9 步编排
	webhookCtrl.SetSmartOrchestrator(orchestrator)
	webhookCtrl.RegisterRoutes(r)
	defer webhookCtrl.Stop()

	// 多 AI 智能体路由：注入到 WebhookService（渠道绑定智能体）
	webhookCtrl.SetAgentBindingService(channelBindingSvcGlobal)

	// 平台端路由（需要平台权限）
	platform := r.Group("/api/platform")
	platform.Use(middleware.InitGuard()) // 1) 系统必须已初始化
	platform.Use(middleware.JWTAuthMiddleware())
	platform.Use(middleware.AdminAuthMiddleware())
	platform.Use(middleware.LicenseGuard())
	{
		setupPlatformRoutes(platform, platformCtrl)
	}

	// P0-6 修复：系统级管理路由（高危操作，需 admin 角色）
	setupSystemAdminRoutes(r)
}
