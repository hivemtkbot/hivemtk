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
func setupPlatformRoutes(platform *gin.RouterGroup, platformCtrl *controller.PlatformController) {
	// 驾驶舱
	platform.GET("/dashboard", platformCtrl.GetDashboard)

	platform.GET("/merchant/list", platformCtrl.GetMerchantList)
	platform.POST("/merchant/license", platformCtrl.UpdateMerchantLicense)
	platform.GET("/merchant/:id/stats", platformCtrl.GetMerchantStats)

	// 版本管理
	platform.GET("/version/list", platformCtrl.GetVersionList)
	platform.GET("/version/latest", platformCtrl.GetLatestVersion)
	platform.GET("/version/check-update", platformCtrl.CheckUpdate)
	platform.GET("/version/:id", platformCtrl.GetVersionByID)
	platform.POST("/version", platformCtrl.CreateVersion)
	platform.POST("/version/create", platformCtrl.CreateVersion)
	platform.PUT("/version/:id", platformCtrl.UpdateVersion)
	platform.PUT("/version/:id/update", platformCtrl.UpdateVersion)
	platform.DELETE("/version/:id", platformCtrl.DeleteVersion)
	platform.DELETE("/version/:id/delete", platformCtrl.DeleteVersion)
	platform.POST("/version/:id/publish", platformCtrl.PublishVersion)
	platform.POST("/version/:id/archive", platformCtrl.ArchiveVersion)

	// 授权管理
	platform.GET("/license/list", platformCtrl.GetLicenseList)
	platform.GET("/license/status", platformCtrl.GetLicenseStatusProxy)
	platform.GET("/license/:id", platformCtrl.GetLicenseByID)
	platform.POST("/license/create", platformCtrl.CreateLicense)
	platform.POST("/license/verify", platformCtrl.VerifyLicense)
	platform.PUT("/license/:id/update", platformCtrl.UpdateLicense)
	platform.DELETE("/license/:id/delete", platformCtrl.DeleteLicense)
	platform.POST("/license/:id/renew", platformCtrl.RenewLicense)
	platform.POST("/license/:id/disable", platformCtrl.DisableLicense)
	platform.POST("/license/:id/enable", platformCtrl.EnableLicense)

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

	// 开源版：已移除授权码管理相关路由（/license/*、/license/status），
	// 仅保留统计/安装信息上报相关能力。

	systemInitCtrl := controller.NewSystemInitController()
	public.GET("/system/init-status", systemInitCtrl.GetInitStatus)
	public.POST("/system/init-admin", systemInitCtrl.InitAdmin)
	public.POST("/system/init-complete", systemInitCtrl.InitComplete)

	// 短链/活码跳转
	public.GET("/s/:code", controller.RedirectShortLink)
	public.GET("/l/:code", liveCodeController.RedirectLiveCode)

	public.POST("/platform/register", platformCtrl.RegisterMerchant)
	public.POST("/platform/report-api-log", platformCtrl.ReportAPILog)
	public.GET("/platform/check-update", platformCtrl.CheckUpdate)

	// P0-14 外部系统知识库接入（公开，使用 API Token 鉴权，不需要 JWT）
	// 商户自部署场景：商户自有 CRM/ERP/Helpdesk 通过此入口推送文档
	// 注册到 public（不走 JWT）以支持 API Token 鉴权
	// 注意：不要在 setupKnowledgeBaseRoutes 里再注册同一个路由（会冲突）
	knowledgeMerchantCtrl := knowledgectrl.NewKnowledgeMerchantController()
	public.POST("/knowledge-merchant/external/import", knowledgeMerchantCtrl.ExternalImport)

	// D 域 P1 缺口修复 - 邮件退订确认页 + 退订提交（公开，用户从邮件点击）
	emailUnsubscribeRepo := repository.NewEmailUnsubscribeRepository(db)
	emailUnsubscribeCtrl := controller.NewEmailUnsubscribeController(
		service.NewEmailUnsubscribeService(emailUnsubscribeRepo),
	)
	emailUnsubscribeCtrl.RegisterRoutes(public, nil)

	// E 域 P1 缺口修复 - 短信上行 webhook（公开，运营商推送）
	smsUnsubscribeRepo := repository.NewSmsUnsubscribeRepository(db)
	smsUnsubscribeCtrl := controller.NewSmsUnsubscribeController(
		service.NewSmsUnsubscribeService(smsUnsubscribeRepo),
	)
	smsUnsubscribeCtrl.RegisterRoutes(public, nil)

	// E 域 P1 缺口修复 - 短信回执 webhook（公开，运营商推送）
	smsDeliveryTrackerCtrl := controller.NewSmsDeliveryTrackerController(
		service.NewSmsDeliveryTrackerService(db, nil, nil),
	)
	smsDeliveryTrackerCtrl.RegisterRoutes(public, nil)

	// D 域 P1 缺口修复 - 邮件追踪像素 + Postmark/SendCloud webhook（公开）
	emailOpenTrackerCtrl := controller.NewEmailOpenTrackerController(
		service.NewEmailOpenTrackerService(nil, nil),
	)
	emailOpenTrackerCtrl.RegisterRoutes(public, nil)

	// P0-6 修复：/api/system/reset 不再是公开路由——见 setupSystemAdminRoutes
	// 原 public.POST("/system/reset", systemOpsCtrl.ResetSystem) 已移除
}

// setupSystemAdminRoutes 系统级管理路由（需要 admin 角色 + JWT 鉴权）
// 用途：高危操作（系统重置、热重启等）
// 中间件链：InitGuard → JWTAuthMiddleware → AdminAuthMiddleware → LicenseGuard
// 注意：此分组不能放进 auth 组（会强制走 LicenseGuard 先），所以独立建组
func setupSystemAdminRoutes(r *gin.Engine) {
	// 开源版：已移除"一键重置"(/api/system/reset) 等高危授权相关路由。
	// 系统重置不在产品流程内，如需重置请在账号个人中心或运维手段处理。
}
