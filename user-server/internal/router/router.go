package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"marketing/internal/bridge"
	contentservice "marketing/internal/content/service"
	"marketing/internal/controller"
	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/repository"
	"marketing/internal/service"
	i18nservice "marketing/internal/service/i18n"

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
				// 浏览器扩展 popup/content script 需跨域调用 API，按源反射放行
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
		c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Requested-With")
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

func Setup(r *gin.Engine) {
	// WhatsApp Cloud 账号服务（在函数级作用域声明，供 Webhook URL 验证与 Cloud 路由共用）
	var whatsappCloudSvc *service.WhatsAppCloudService
	var webhookSvc *service.WebhookService
	var dingtalkAppSvc *service.DingTalkAppService

	// 启用 405 Method Not Allowed：当路径已注册但 HTTP 方法不匹配时，
	// 返回 405 而非默认 404，便于客户端（含 API 扫描器）区分「路径不存在」与「方法错误」。
	// 例如 POST /api/customer-sessions/:id/takeover 被以 GET 访问时，
	// 返回 405 而非 404，避免误判为「路由缺失」。
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"code":    "METHOD_NOT_ALLOWED_405",
			"message": "请求方法不被支持",
			"method":  c.Request.Method,
			"path":    c.Request.URL.Path,
		})
	})

	// 基础中间件
	r.Use(corsMiddleware()) // 全局 CORS：允许 Chrome 扩展 popup 跨域调用 API（含 OPTIONS 预检）
	r.Use(gin.Recovery())

	// 多语言：解析请求语言注入上下文，供业务层返回本地化提示
	r.Use(middleware.LocaleMiddleware())

	// ContextMiddleware：注入 IP / User-Agent 等公共上下文字段，供后续 handler / service 复用
	r.Use(middleware.ContextMiddleware())

	// 初始化全局事件总线（在 Service 构造之前）
	// 试点：OperationLog 异步写入；后续可在 initEventBus 中追加订阅者
	initEventBus()

	// 健康检查端点（公开）
	r.GET("/health", HealthCheck(HealthRedis))
	r.GET("/healthz", LivenessCheck())
	r.GET("/readyz", ReadinessCheck(HealthRedis))

	// 私域部署: 已移除 Prometheus 指标采集 (/metrics 端点 + PrometheusMetricsMiddleware)
	// 关键指标 (wall_ms / LCP / Layer1 命中率) 通过应用层日志 + layer_decision_logs 表审计。
	// 巡检方式: SQL 查询 + scripts/post_deploy_check.sh

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
	// 4) initInferenceOrchestrator —— 装配推理闭环编排器（优化：原本死代码，本次激活）
	initGlobalToolExecutor()
	initGlobalToolRouter() // 装配 ToolRouter（熔断 + 限流 + 成本统计 + 全局统计），激活原本死代码
	registerAllAgentTools(db.GetDB())
	// 网页私信桥接：构造 BridgeReachAdapter 并把 AI 回复经 WebSocket 回写 Chrome 扩展。
	// 必须在 registerAllAgentTools 之后、Agent Loop 激活之前调用（其内部会注册桥接出站回调）。
	registerAgentReachTools(db.GetDB())
	initInferenceOrchestrator() // 优化：激活推理闭环编排器（历史死代码）

	engine := buildSalesEngine(db.GetDB())
	orchestrator := buildSmartOrchestrator(engine)
	aiAgentSvcGlobal := service.NewAIAgentService()
	channelBindingSvcGlobal := service.NewChannelAgentBindingService()
	csAgentSvcGlobal := service.NewCustomerServiceAgentService()
	// 注入到 SmartCSOrchestrator（客服座席挂载智能体路由）
	orchestrator.SetCustomerServiceAgentService(context.Background(), csAgentSvcGlobal)

	// v1.2 出海方案：初始化 LangConfigResolver（双语言配置读取器）。
	// 注入到所有用户消息入口（chat HTTP / WS / webhook），实现多层兜底解析。
	// resolver 自身永不报错，下游即便配置缺失也会拿到默认 zh。
	langResolver := i18nservice.NewLangConfigResolver(
		repository.NewChatChannelRepository(),
		repository.NewAIAgentRepository(),
	)

	// M2：初始化资产市场运行时覆盖层（业务运行时优先读取生效中的已购资产）
	service.InitAssetResolver(db.GetDB())
	// 注入「读取生效中 marketing_workflow 资产」函数，打破 content/service 循环依赖
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

	// 公开路由（不需要认证）
	public := r.Group("/api")
	{
		setupPublicRoutes(public, liveCodeController, platformCtrl, db.GetDB())
		// 公开 chat API（AppKey 鉴权）
		setupChatPublicRoutes(public, db.GetDB(), orchestrator, langResolver)
	}

	// 访客 WebSocket（公开，无鉴权）
	setupChatPublicWebSocket(r, langResolver)

	// 卡片分享路由（公开，不需要认证）
	setupCardShareRoutes(r)

	// 静态文件服务（chat embed 页面 + embed SDK）
	// 私域部署：用户把 user-web/dist 部署到 user-server 同源，
	// 这样嵌入的 iframe 聊天窗可以无跨域问题加载。
	setupEmbedStaticRoutes(r)

	// 认证路由
	auth := r.Group("/api")
	auth.Use(middleware.InitGuard()) // 1) 系统必须已初始化
	// 开源版：移除 LicenseGuard 中间件（License 模型删除，授权流程下线）
	auth.Use(middleware.JWTAuthMiddleware()) // 2) JWT 必须有效
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

		// 客户 RFM 联动分层
		setupCustomerRFMRoutes(auth)

		// 流失挽回队列
		setupRecoveryQueueRoutes(auth)

		// 系统管理（高危操作：重启/日志/备份/恢复/配置写入，需管理员权限）
		systemAdmin := auth.Group("")
		systemAdmin.Use(middleware.AdminAuthMiddleware())
		setupSystemRoutes(systemAdmin)

		// 人员管理（v3.1 §3.1：/api/system/users/*）
		setupSystemUserRoutes(auth)

		// 角色管理（v3.1 §3.2：/api/system/roles/*）
		setupRoleRoutes(auth)

		// 授权管理（v3.1 §3.4：/api/system/permissions/*）
		setupPermissionRoutes(auth)

		// RAG 知识库
		setupRagRoutes(auth)

		// 知识库管理
		setupKnowledgeBaseRoutes(auth)

		// WhatsApp (Web 扫码)
		setupWhatsappRoutes(auth)

		// WhatsApp Cloud (Meta 商业 API)
		whatsappCloudSvc = service.NewWhatsAppCloudService(db.GetDB())
		setupWhatsAppCloudRoutes(auth, whatsappCloudSvc)

		// 钉钉企业内部应用（支持回调收消息）
		webhookSvc = service.NewWebhookService(db.GetDB())
		dingtalkAppSvc = service.NewDingTalkAppService(db.GetDB(), webhookSvc)

		setupDingTalkAppRoutes(auth, dingtalkAppSvc)

		// Telegram
		setupTelegramRoutes(auth)

		// 飞书
		setupFeishuRoutes(auth)

		// TikTok
		setupTiktokRoutes(auth)

		// 企业微信
		setupWeComRoutes(auth)

		// 客服会话管理
		setupCustomerServiceRoutes(auth, aiAgentSvcGlobal, langResolver)

		// 网页桥接 WebSocket（抖音/小红书/TikTok 网页私信）：扩展经此上行私信、下行 AI 回复。
		// 不要求前端 JWT——账号以 channel+account_id 自证身份（私有化部署单用户场景）。
		// 仅过 InitGuard（系统须已初始化），不过 JWTAuthMiddleware，故无需在 popup 填 token。
		bridgeIngressSvc = service.NewInboxIngressService()
		bridgeHandler := bridge.NewBridgeWSHandler(bridge.GetBridgeHub(), bridgeIngressSvc)
		bridgeWS := r.Group("/api")
		bridgeWS.Use(middleware.InitGuard())
		bridgeWS.GET("/ws/bridge", bridgeHandler.HandleWebSocket)

		// 网页私信桥接账号：持久化 + 归属校验 + 管理路由（抖音/小红书/TikTok）
		bridgeRepo := bridge.NewBridgeAccountRepository(db.GetDB())
		bridge.RegisterBridgeAccountRepo(bridgeRepo)
		bridge.RegisterOwnershipChecker(func(ctx context.Context, userID uint, channel, accountID string) (bool, error) {
			acc, err := bridgeRepo.GetByChannelAccount(ctx, channel, accountID)
			if err == gorm.ErrRecordNotFound || acc == nil {
				// 尚未注册：允许连接，注册帧会创建并归属到当前用户（G4 防水平越权）
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return acc.UserID == userID, nil
		})
		bridgeAccountCtrl := controller.NewBridgeAccountController()
		bridgeAccountCtrl.RegisterRoutes(auth)

		// 把 AI 触发实现注入桥接入站服务：抖音/小红书/TikTok 新消息经此触发 AI 客服并原路回写扩展
		// （必须在 setupCustomerServiceRoutes 之后，因为 bridgeIngressSvc 是在其中被创建的）
		if bridgeIngressSvc != nil && webhookSvc != nil {
			bridgeIngressSvc.SetAITrigger(webhookSvc)
			logger.Infof("[Bridge] bridge AITrigger 已注入（抖音/小红书/TikTok 网页私信 AI 链路已连通）")
		}
		// 注入统一收件箱服务：桥接消息落库 message_hub 后同步会话到 inbox_conversations，
		// 否则 unifiedInbox/list 统一收件箱看不到抖音/小红书/TikTok 网页私信聊天内容。
		if bridgeIngressSvc != nil {
			bridgeIngressSvc.SetInboxService(service.NewInboxService())
			logger.Infof("[Bridge] bridge InboxService 已注入（统一收件箱会话同步已连通）")
		}

		// 客服 Web Widget 渠道管理（前端 ChatChannel.vue 列表/创建/编辑依赖）
		setupChatChannelAdminRoutes(auth, db.GetDB())

		// v1.2 出海多语言方案：术语表管理 + 校验预览
		setupI18nRoutes(auth, db.GetDB())

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

		// 缺口修复路由
		// LLM Provider 降级管理 / 全链路追踪 / SSE 实时驾驶舱
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

		// 资产市场（平台购买 + 本地同源同构 CRUD）
		setupAssetMarketRoutes(auth)

		// 方向9：资产包模式 — OpenAI messages 资产包 CRUD + Weave 织布算法
		setupAssetBundleRoutes(auth)

		// 流失预警
		setupChurnRoutes(auth)

		// 第三方对接
		setupIntegrationRoutes(auth)

		// 社群管理
		setupCommunityRoutes(auth)

		// 备份恢复
		setupBackupRoutes(auth)

		// 数据库迁移（原"版本升级"，M3 重命名以避免与 OTA 概念混淆）
		setupMigrationRoutes(auth)

		// 文件上传
		// controller.UploadFile 是 free function（无 struct 包装），无需工厂方法。
		auth.POST("/upload", controller.UploadFile)

		// 工具链调试与可观测 API
		// 端点：/api/agent/tools/{list,get,execute,stats,audit,cost,circuit/reset}
		setupToolDebugRoutes(auth)

		// 工具权限白名单管理 API（，原本已实现但未装配， 优化激活）
		// 端点：/api/agent/tools/permission/{default,global,agents}
		setupToolPermissionRoutes(auth)

		// AI 工具配置管理 API
		// 端点：/api/ai-tools/{list,get,status,accounts}
		setupAIToolConfigRoutes(auth, db.GetDB())

		// 推理闭环 API（，原本已实现但未装配， 优化激活）
		// 端点：/api/agent/inference/{run,stats}
		// 注意：initInferenceOrchestrator 必须在 router.Setup 早期调用；
		// 若未初始化，handleInferenceRun 会返回 503。
		setupInferenceRoutes(auth)

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

		// 前端 API 路径别名（兼容前端调用习惯）
		// 必须放在所有 setup* 之后，避免更具体的路由被覆盖
		setupFrontendAliases(auth, r)
	}

	// 多渠道 Webhook 路由（公开，不需要鉴权）
	webhookCtrl := controller.NewWebhookController(webhookSvc)
	// WhatsApp Cloud 账号服务注入，用于回调 URL 验证（GET /api/webhook/whatsapp/{id}）
	webhookCtrl.SetWhatsAppCloudService(whatsappCloudSvc)
	// 钉钉应用账号服务注入，用于回调验签 + 入站收消息（GET/POST /api/webhook/dingtalk/{id}）
	webhookCtrl.SetDingTalkAppService(dingtalkAppSvc)
	// 飞书账号服务注入，用于回调 URL 验证（GET /api/webhook/feishu/{id} 校验 VerificationToken）
	webhookCtrl.SetFeishuService(service.NewFeishuService(db.GetDB()))
	// 修复：智能体引擎 8 步链路真实依赖注入
	// 不再 nil 注入，让 SalesEngine 真正调用 intent/memory/sop/rag/script/customer
	webhookCtrl.SetSalesEngine(engine)
	// Phase 3：注入智能体统一编排器（LLM + 客服座席结合体）
	// Webhook 入站消息优先走 SmartCSOrchestrator.HandleIncoming 9 步编排
	webhookCtrl.SetSmartOrchestrator(orchestrator)
	// v1.2 出海方案：注入多语言解析器，供 webhook 入口注入双语言 ctx
	webhookCtrl.SetLangResolver(langResolver)
	webhookCtrl.RegisterRoutes(r)

	// 多 AI 智能体路由：注入到 WebhookService（渠道绑定智能体）
	webhookCtrl.SetAgentBindingService(channelBindingSvcGlobal)

	// 启动期对账：为所有已启用且 webhook 配置齐全的 Telegram 账号重新 setWebhook。
	// 解决「TG 配置→启动→AI销售」断链：服务器重启 / 域名变更 / UI 新建账号后，
	// 若不重新注册 webhook，Telegram 不再推送更新，导致全链路静默断裂。
	// best-effort，外网请求不阻塞启动流程。
	go service.ReconcileTelegramWebhooks(service.NewTelegramService(db.GetDB()))

	// 平台端路由（需要平台权限）
	platform := r.Group("/api/platform")
	platform.Use(middleware.InitGuard()) // 1) 系统必须已初始化
	platform.Use(middleware.JWTAuthMiddleware())
	platform.Use(middleware.AdminAuthMiddleware())
	// 开源版：移除 LicenseGuard 中间件
	{
		setupPlatformRoutes(platform, platformCtrl)
	}
}
