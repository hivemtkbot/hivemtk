package router

import (
	knowledgectrl "marketing/internal/aiagent/knowledge/controller"
	"marketing/internal/controller"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupPlatformRoutes 平台端管理路由（需要平台权限）
//
// 开源版：已移除 OTA 版本管理（version/*）与 License 授权管理（license/*）相关路由。
// 仅保留统计/安装/心跳信息相关的只读能力。
func setupPlatformRoutes(platform *gin.RouterGroup, platformCtrl *controller.PlatformController) {
	// 驾驶舱
	platform.GET("/dashboard", platformCtrl.GetDashboard)

	// 商户管理：开源版移除 UpdateMerchantLicense 授权变更入口
	platform.GET("/merchant/list", platformCtrl.GetMerchantList)
	platform.GET("/merchant/:id/stats", platformCtrl.GetMerchantStats)

	// 开源版：移除版本管理（/version/*）和授权管理（/license/*）相关路由
	// 仅保留：商户列表、统计、用户管理、消息中心等只读/基础能力

	// 站内信
	platform.GET("/message/list", platformCtrl.GetMessageList)
	platform.GET("/message/latest", platformCtrl.GetLatestMessage)
	platform.POST("/message/send", platformCtrl.SendMessage)
	platform.POST("/message/:id/read", platformCtrl.MarkPlatformMessageRead)

	// 用户管理
	platform.GET("/user/list", platformCtrl.GetUserList)
	platform.POST("/user", platformCtrl.CreateUser)
	platform.DELETE("/user/:id", platformCtrl.DeleteUser)

	// 系统统计
	platform.GET("/stats/system", platformCtrl.GetSystemStats)
	platform.GET("/stats/overview", platformCtrl.GetPlatformStats)
	platform.GET("/stats/merchant", platformCtrl.GetPlatformMerchantStats)
}

// setupPublicRoutes 公开路由（不需要认证）
func setupPublicRoutes(public *gin.RouterGroup, liveCodeController *controller.LiveCodeController, platformCtrl *controller.PlatformController, db *gorm.DB) {
	// 健康检查
	public.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// 系统信息
	systemInfoCtrl := controller.NewSystemInfoController()
	public.GET("/system/info", systemInfoCtrl.GetSystemInfo)

	// 授权相关
	public.POST("/auth/login", controller.NewAuthController().Login)

	// 开源版：已移除"首次强制改密"(init-change-password) 与"通过授权找回密码"
	// (forgot-admin-password / reset-admin-password) 流程。找回密码统一在账号个人中心进行。

	// P1-1 MFA 登录第二步验证：使用 temp_token（不依赖 JWT）
	// 登录密码验证通过后，若用户启用了 MFA，会返回 need_mfa=true + temp_token，
	// 前端再调用此接口提交 temp_token + 6 位 TOTP 码完成登录
	public.POST("/auth/mfa/verify", controller.NewAuthController().VerifyMFALogin)

	// 系统初始化
	systemUserCtrl := controller.NewSystemUserController()
	public.POST("/system/create-default-admin", systemUserCtrl.CreateDefaultAdmin)

	// 开源版：已移除授权码管理相关路由（/license/*、/license/status）。
	// 为兼容前端 Layout.vue 对 /api/license/status 的探测（开源版无授权概念），
	// 提供只读占位接口：返回 200 + 空数据，不触发前端报错与 404 网络日志。
	public.GET("/license/status", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 200, "data": nil, "msg": "ok"})
	})
	public.GET("/license/features", func(c *gin.Context) {
		c.JSON(200, gin.H{"code": 200, "data": []interface{}{}, "msg": "ok"})
	})

	systemInitCtrl := controller.NewSystemInitController()
	public.GET("/system/init-status", systemInitCtrl.GetInitStatus)
	public.POST("/system/init-admin", systemInitCtrl.InitAdmin)
	public.POST("/system/init-complete", systemInitCtrl.InitComplete)

	// 短链/活码跳转
	public.GET("/s/:code", controller.RedirectShortLink)
	public.GET("/l/:code", liveCodeController.RedirectLiveCode)

	public.POST("/platform/register", platformCtrl.RegisterMerchant)
	public.POST("/platform/report-api-log", platformCtrl.ReportAPILog)
	// 开源版：移除 OTA 客户端更新检查接口（/platform/check-update）

	// P0-14 外部系统知识库接入（公开，使用 API Token 鉴权，不需要 JWT）
	// 商户自部署场景：商户自有 CRM/ERP/Helpdesk 通过此入口推送文档
	// 注册到 public（不走 JWT）以支持 API Token 鉴权
	// 注意：不要在 setupKnowledgeBaseRoutes 里再注册同一个路由（会冲突）
	// 审计 M7：依赖构造集中到 wirePublicDependencies，路由层只消费依赖。
	deps := wirePublicDependencies(db)
	public.POST("/knowledge-merchant/external/import", deps.knowledgeMerchantCtrl.ExternalImport)

	// D 域 P1 缺口修复 - 邮件退订确认页 + 退订提交（公开，用户从邮件点击）
	deps.emailUnsubscribeCtrl.RegisterRoutes(public, nil)

	// E 域 P1 缺口修复 - 短信上行 webhook（公开，运营商推送）
	deps.smsUnsubscribeCtrl.RegisterRoutes(public, nil)

	// E 域 P1 缺口修复 - 短信回执 webhook（公开，运营商推送）
	deps.smsDeliveryTrackerCtrl.RegisterRoutes(public, nil)

	// D 域 P1 缺口修复 - 邮件追踪像素 + Postmark/SendCloud webhook（公开）
	deps.emailOpenTrackerCtrl.RegisterRoutes(public, nil)

	// P0-6 修复：/api/system/reset 不再是公开路由——见 setupSystemAdminRoutes
	// 原 public.POST("/system/reset", systemOpsCtrl.ResetSystem) 已移除

	// 企业级架构优化 - 方向 3: 渠道接入消息中台
	// 公开 webhook 入口：所有渠道（TG/WA/小程序/邮件上行/短信上行/...）推送到此
	// 内部再加分布式锁做防抖 + 人工接管锁拦截
	public.POST("/chat/ingress", deps.inboxIngressCtrl.Ingress)
}

// publicDeps 聚合 setupPublicRoutes 所需的仓储/服务/控制器依赖。
//
// 审计 M7（路由层直接构造 service/repository）：原本这些 new 散落在路由注册处，
// 既不便测试也加深路由层与具体实现的耦合。此处集中构造，路由层只负责“消费依赖 + 注册路由”，
// 后续可平滑替换为 wire/fx 等 DI 容器（当前规模下显式 wiring 已足够清晰且低风险）。
type publicDeps struct {
	knowledgeMerchantCtrl  *knowledgectrl.KnowledgeMerchantController
	emailUnsubscribeCtrl   *controller.EmailUnsubscribeController
	smsUnsubscribeCtrl     *controller.SmsUnsubscribeController
	smsDeliveryTrackerCtrl *controller.SmsDeliveryTrackerController
	emailOpenTrackerCtrl   *controller.EmailOpenTrackerController
	inboxIngressCtrl       *controller.InboxIngressController
}

func wirePublicDependencies(db *gorm.DB) publicDeps {
	emailUnsubscribeRepo := repository.NewEmailUnsubscribeRepository(db)
	smsUnsubscribeRepo := repository.NewSmsUnsubscribeRepository(db)
	return publicDeps{
		knowledgeMerchantCtrl:  knowledgectrl.NewKnowledgeMerchantController(),
		emailUnsubscribeCtrl:   controller.NewEmailUnsubscribeController(service.NewEmailUnsubscribeService(emailUnsubscribeRepo)),
		smsUnsubscribeCtrl:     controller.NewSmsUnsubscribeController(service.NewSmsUnsubscribeService(smsUnsubscribeRepo)),
		smsDeliveryTrackerCtrl: controller.NewSmsDeliveryTrackerController(service.NewSmsDeliveryTrackerService(db, nil, nil)),
		emailOpenTrackerCtrl:   controller.NewEmailOpenTrackerController(service.NewEmailOpenTrackerService(nil, nil)),
		inboxIngressCtrl:       controller.NewInboxIngressController(db),
	}
}

// setupSystemAdminRoutes 系统级管理路由（需要 admin 角色 + JWT 鉴权）
// 用途：高危操作（系统重置、热重启等）
// 中间件链：InitGuard → JWTAuthMiddleware → AdminAuthMiddleware → LicenseGuard
// 注意：此分组不能放进 auth 组（会强制走 LicenseGuard 先），所以独立建组
func setupSystemAdminRoutes(r *gin.Engine) {
	// 开源版：已移除"一键重置"(/api/system/reset) 等高危授权相关路由。
	// 系统重置不在产品流程内，如需重置请在账号个人中心或运维手段处理。
}
