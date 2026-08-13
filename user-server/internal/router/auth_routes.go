package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAuthRoutes 认证相关路由
func setupAuthRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	authCtrl := controller.NewAuthController()
	auth.POST("/auth/refresh-token", authCtrl.RefreshToken)
	auth.GET("/auth/current-user", authCtrl.GetCurrentUser)
	auth.POST("/auth/change-password", authCtrl.ChangePassword)

	// MFA 多因素认证
	// 注意：POST /auth/mfa/verify 为登录第二步（无需 JWT，使用 temp_token）
	//       已注册到 public 路由组（见 admin_routes.go setupPublicRoutes）
	//       其余 MFA 接口需要 JWT（已在 auth 路由组的 JWTAuthMiddleware 中保护）
	auth.POST("/auth/mfa/setup", authCtrl.SetupMFA)
	auth.POST("/auth/mfa/confirm", authCtrl.ConfirmMFASetup)
	auth.POST("/auth/mfa/disable", authCtrl.DisableMFA)
	auth.GET("/auth/mfa/status", authCtrl.GetMFAStatus)

	// 异常登录预警
	auth.GET("/auth/login-events", authCtrl.ListLoginEvents)
	auth.GET("/auth/security-alerts", authCtrl.ListSecurityAlerts)
	auth.POST("/auth/security-alerts/:id/resolve", authCtrl.ResolveSecurityAlert)

	// 异常登录预警 - 异常登录预警控制器（增强告警通道：审计+邮件+站内信）
	anomalyCtrl := controller.NewAnomalyLoginDetectorController()
	auth.GET("/auth/anomaly/login-events", anomalyCtrl.ListLoginEvents)
	auth.GET("/auth/anomaly/alerts", anomalyCtrl.ListAlerts)
	auth.POST("/auth/anomaly/alerts/:id/resolve", anomalyCtrl.ResolveAlert)
	auth.POST("/auth/anomaly/alerts/:id/ignore", anomalyCtrl.IgnoreAlert)

	// 密码策略
	auth.GET("/auth/password-policy", authCtrl.GetPasswordPolicy)
	auth.PUT("/auth/password-policy", authCtrl.SavePasswordPolicy)

	// 通知中心（站内通知 / 顶部铃铛 badge）
	notifCtrl := controller.NewNotificationController(service.NewNotificationService(gormDB))
	auth.GET("/auth/notifications", notifCtrl.List)
	auth.POST("/auth/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/auth/notifications/read-all", notifCtrl.MarkAllRead)
	auth.GET("/auth/notifications/unread-count", notifCtrl.UnreadCount)

	// 通知中心 - 前端兼容别名（Notifications.vue 使用 /api/notifications）
	auth.GET("/notifications", notifCtrl.List)
	auth.GET("/notifications/unread-count", notifCtrl.UnreadCount)
	auth.POST("/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/notifications/read-all", notifCtrl.MarkAllRead)

	// 阶段 4-6：人员 / 角色 / 授权管理路由统一由 router.go 的 Setup() 在
	// 装配完其它子路由后集中注册（参见 Setup() 中 setupSystemUserRoutes /
	// setupRoleRoutes / setupPermissionRoutes 调用），避免在 auth_routes 中
	// 重复注册导致 gin panic("handlers are already registered for path ...")。
}

// setupUserRoutes 用户管理路由
func setupUserRoutes(auth *gin.RouterGroup) {
	userCtrl := controller.NewSystemUserController()
	// 读操作：已登录用户可查看用户列表/详情（单租户协作场景，信息泄露风险低）
	auth.GET("/user/list", userCtrl.GetUsers)
	auth.GET("/users", userCtrl.GetUsers)
	auth.GET("/user/:id", userCtrl.GetUser)
	auth.GET("/users/:id", userCtrl.GetUser)

	// 写操作（创建/修改/重置密码/删除）：高危，仅管理员可执行。
	// 修复：原先仅挂载 JWTAuth，任意登录用户可重置管理员密码或自提权（垂直越权）。
	// 现统一包 AdminAuthMiddleware，使重置密码/创建用户等写操作必须管理员角色。
	admin := auth.Group("")
	admin.Use(middleware.AdminAuthMiddleware())
	{
		admin.POST("/user", userCtrl.CreateUser)
		admin.POST("/users", userCtrl.CreateUser)
		admin.PUT("/user/:id", userCtrl.UpdateUser)
		admin.PUT("/users/:id", userCtrl.UpdateUser)
		admin.DELETE("/user/:id", userCtrl.DeleteUser)
		admin.DELETE("/users/:id", userCtrl.DeleteUser)
		admin.PUT("/user/:id/password", userCtrl.ResetPassword)
		admin.PUT("/users/:id/password", userCtrl.ResetPassword)
	}
}

// setupAccountRoutes 账户管理路由
func setupAccountRoutes(auth *gin.RouterGroup) {
	accountCtrl := controller.NewAccountController()
	auth.GET("/account/list", accountCtrl.GetAccounts)
	auth.GET("/account/:id", accountCtrl.GetAccount)
	auth.POST("/account", accountCtrl.CreateAccount)
	auth.PUT("/account/:id", accountCtrl.UpdateAccount)
	auth.DELETE("/account/:id", accountCtrl.DeleteAccount)
	// 前端兼容路由
	auth.GET("/accounts/list", accountCtrl.GetAccounts)
	auth.POST("/accounts/create", accountCtrl.CreateAccount)
	auth.PUT("/accounts/update/:id", accountCtrl.UpdateAccount)
	auth.DELETE("/accounts/delete/:id", accountCtrl.DeleteAccount)
}

// setupShortLinkRoutes 短链管理路由
func setupShortLinkRoutes(auth *gin.RouterGroup, public *gin.RouterGroup, gormDB *gorm.DB) {
	shortLinkCtrl := controller.NewShortLinkController(service.NewShortLinkService(gormDB))
	shortLinkStatsCtrl := controller.NewShortLinkStatsController(service.NewShortLinkService(gormDB))

	// 短链管理
	auth.GET("/short-link/list", shortLinkCtrl.GetList)
	auth.POST("/short-link", shortLinkCtrl.Create)
	auth.PUT("/short-link/:id", shortLinkCtrl.Update)
	auth.DELETE("/short-link/:id", shortLinkCtrl.Delete)
	auth.GET("/short-link/:id", shortLinkCtrl.GetByID)
	// 短链访问（按短码解析原始URL；公开端点供终端用户免登录访问，密码保护由 service 校验）
	public.POST("/short-link/access", shortLinkCtrl.AccessShortLink)
	// 短链统计
	auth.GET("/short-link/:id/stats", shortLinkStatsCtrl.GetStats)
	auth.GET("/short-link/all-stats", shortLinkStatsCtrl.GetAllStats)
	// 前端兼容路由
	auth.GET("/shortlink/list", shortLinkCtrl.GetList)
	auth.GET("/shortlink/:id", shortLinkCtrl.GetByID)
	auth.POST("/shortlink/create", shortLinkCtrl.Create)
	auth.PUT("/shortlink/update", func(c *gin.Context) { shortLinkCtrl.Update(c) })
	auth.DELETE("/shortlink/delete/:id", shortLinkCtrl.Delete)
	auth.POST("/shortlink/access", shortLinkCtrl.AccessShortLink)
	auth.POST("/shortlink/generate", shortLinkCtrl.Create)
	auth.GET("/shortlink/:id/stats", shortLinkStatsCtrl.GetStats)
	auth.GET("/shortlink/stats/all", shortLinkStatsCtrl.GetAllStats)
	auth.POST("/shortlink/:id/share", shortLinkStatsCtrl.ShareShortLink)
}

// setupLiveCodeRoutes 活码管理路由
func setupLiveCodeRoutes(auth *gin.RouterGroup, liveCodeController *controller.LiveCodeController) {
	auth.GET("/live-code/list", liveCodeController.GetList)
	auth.GET("/live-codes/list", liveCodeController.GetList)
	auth.POST("/live-code", liveCodeController.Create)
	auth.PUT("/live-code/:id", liveCodeController.Update)
	auth.DELETE("/live-code/:id", liveCodeController.Delete)
	auth.GET("/live-code/:id", liveCodeController.GetByID)
	auth.GET("/live-code/:id/stats", liveCodeController.GetStats)
	auth.POST("/live-code/:id/qr-code", liveCodeController.GenerateQRCode)
	auth.GET("/live-code/:id/qr-codes", liveCodeController.GetQRCodes)
	auth.GET("/live-code/:id/qr-stats", liveCodeController.GetQRStats)
	auth.POST("/live-code/:id/share", liveCodeController.Share)
	auth.DELETE("/live-code/:id/qr-codes/:qr_id", liveCodeController.DeleteLiveCodeQR)
	auth.PUT("/live-code/:id/qr-codes/:qr_id", liveCodeController.UpdateLiveCodeQR)
	// 前端兼容路由
	auth.GET("/live-codes/:id", liveCodeController.GetByID)
	auth.POST("/live-codes/create", liveCodeController.Create)
	auth.PUT("/live-codes/:id/update", liveCodeController.Update)
	auth.DELETE("/live-codes/:id/delete", liveCodeController.Delete)
	auth.GET("/live-codes/:id/stats", liveCodeController.GetStats)
	auth.GET("/live-codes/:id/qrcodes", liveCodeController.GetQRCodes)
	auth.POST("/live-codes/:id/qrcodes/create", liveCodeController.GenerateQRCode)
	auth.GET("/live-codes/qrcodes/:qr_id/stats", liveCodeController.GetQRStats)
	auth.POST("/live-codes/:id/share", liveCodeController.Share)
	auth.DELETE("/live-codes/qrcodes/:qr_id/delete", liveCodeController.DeleteLiveCodeQR)
	auth.PUT("/live-codes/qrcodes/:qr_id/update", liveCodeController.UpdateLiveCodeQR)
}

// setupEmailRoutes 邮件管理路由
func setupEmailRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	emailListCtrl := controller.NewEmailListController()
	emailSmtpCtrl := controller.NewEmailSmtpController()
	emailDraftCtrl := controller.NewEmailDraftController()
	emailJobsCtrl := controller.NewEmailJobsController()
	emailSendCtrl := controller.NewEmailSendController()

	// 邮件列表
	auth.GET("/email/list", emailListCtrl.GetEmailListList)
	auth.POST("/email/list", emailListCtrl.CreateEmailList)
	auth.PUT("/email/list/:id", emailListCtrl.UpdateEmailList)
	auth.DELETE("/email/list/:id", emailListCtrl.DeleteEmailList)
	auth.GET("/email/list/:id", emailListCtrl.GetEmailListDetail)
	auth.POST("/email/list/:id/trace", emailListCtrl.TraceEmail)

	// 邮件 SMTP
	auth.GET("/email/smtp", emailSmtpCtrl.GetEmailSmtpList)
	auth.GET("/email/smtp/:id", emailSmtpCtrl.GetEmailSmtp)
	auth.POST("/email/smtp", emailSmtpCtrl.CreateEmailSmtp)
	auth.PUT("/email/smtp/:id", emailSmtpCtrl.UpdateEmailSmtp)
	auth.DELETE("/email/smtp/:id", emailSmtpCtrl.DeleteEmailSmtp)

	// 邮件草稿
	auth.GET("/email/drafts", emailDraftCtrl.GetEmailDraftList)
	auth.POST("/email/drafts", emailDraftCtrl.CreateEmailDraft)
	auth.GET("/email/drafts/:id", emailDraftCtrl.GetEmailDraftDetail)
	auth.PUT("/email/drafts/:id", emailDraftCtrl.UpdateEmailDraft)
	auth.DELETE("/email/drafts/:id", emailDraftCtrl.DeleteEmailDraft)

	// 邮件任务
	auth.GET("/email/jobs", emailJobsCtrl.GetEmailJobsList)
	auth.POST("/email/jobs", emailJobsCtrl.CreateEmailJobs)
	auth.DELETE("/email/jobs/:id", emailJobsCtrl.DeleteEmailJobs)
	auth.GET("/email/jobs/:id", emailJobsCtrl.GetEmailJobsDetail)

	// 邮件发送
	auth.POST("/email/send", emailSendCtrl.SendEmail)

	// D 域 缺口修复 - 邮件退订管理 + 打开率追踪
	emailUnsubscribeRepo := repository.NewEmailUnsubscribeRepository(gormDB)
	emailUnsubscribeCtrl := controller.NewEmailUnsubscribeController(
		service.NewEmailUnsubscribeService(emailUnsubscribeRepo),
	)
	emailUnsubscribeCtrl.RegisterRoutes(nil, auth)

	emailOpenTrackerCtrl := controller.NewEmailOpenTrackerController(
		service.NewEmailOpenTrackerService(nil, nil),
	)
	emailOpenTrackerCtrl.RegisterRoutes(nil, auth)
}

// setupSmsRoutes 短信管理路由
func setupSmsRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	smsRepo := repository.NewSmsRepository()
	smsCtrl := controller.NewSmsController(service.NewSmsService(smsRepo))
	smsCtrl.RegisterRoutes(auth)

	// E 域 缺口修复 - 短信退订管理 + 到达率追踪
	smsUnsubscribeRepo := repository.NewSmsUnsubscribeRepository(gormDB)
	smsUnsubscribeCtrl := controller.NewSmsUnsubscribeController(
		service.NewSmsUnsubscribeService(smsUnsubscribeRepo),
	)
	smsUnsubscribeCtrl.RegisterRoutes(nil, auth)

	smsDeliveryTrackerCtrl := controller.NewSmsDeliveryTrackerController(
		service.NewSmsDeliveryTrackerService(gormDB, nil, nil),
	)
	smsDeliveryTrackerCtrl.RegisterRoutes(nil, auth)
}

