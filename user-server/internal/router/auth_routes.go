package router

import (
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupAuthRoutes 认证相关路由
// 注意：初始化/忘记密码相关路由（init-change-password、forgot-admin-password、reset-admin-password）
// 已迁移到 setupPublicRoutes（公开路由），因为这些路由在初始化或无 JWT 场景下必须可访问
func setupAuthRoutes(auth *gin.RouterGroup) {
	authCtrl := controller.NewAuthController()
	auth.POST("/auth/refresh-token", authCtrl.RefreshToken)
	auth.GET("/auth/current-user", authCtrl.GetCurrentUser)
	auth.POST("/auth/change-password", authCtrl.ChangePassword)

	// P1-1 MFA 多因素认证
	// 注意：POST /auth/mfa/verify 为登录第二步（无需 JWT，使用 temp_token）
	//       已注册到 public 路由组（见 admin_routes.go setupPublicRoutes）
	//       其余 MFA 接口需要 JWT（已在 auth 路由组的 JWTAuthMiddleware 中保护）
	auth.POST("/auth/mfa/setup", authCtrl.SetupMFA)
	auth.POST("/auth/mfa/confirm", authCtrl.ConfirmMFASetup)
	auth.POST("/auth/mfa/disable", authCtrl.DisableMFA)
	auth.GET("/auth/mfa/status", authCtrl.GetMFAStatus)

	// P1-2 异常登录预警
	auth.GET("/auth/login-events", authCtrl.ListLoginEvents)
	auth.GET("/auth/security-alerts", authCtrl.ListSecurityAlerts)
	auth.POST("/auth/security-alerts/:id/resolve", authCtrl.ResolveSecurityAlert)

	// P1-2 异常登录预警 - 异常登录预警控制器（增强告警通道：审计+邮件+站内信）
	anomalyCtrl := controller.NewAnomalyLoginDetectorController()
	auth.GET("/auth/anomaly/login-events", anomalyCtrl.ListLoginEvents)
	auth.GET("/auth/anomaly/alerts", anomalyCtrl.ListAlerts)
	auth.POST("/auth/anomaly/alerts/:id/resolve", anomalyCtrl.ResolveAlert)
	auth.POST("/auth/anomaly/alerts/:id/ignore", anomalyCtrl.IgnoreAlert)

	// P1-3 密码策略
	auth.GET("/auth/password-policy", authCtrl.GetPasswordPolicy)
	auth.PUT("/auth/password-policy", authCtrl.SavePasswordPolicy)

	// P1-4 数据行级权限（team_user）
	rowLevelCtrl := controller.NewRowLevelSecurityController()
	auth.GET("/team/users/:id/data-scope", rowLevelCtrl.GetUserDataScope)
	auth.PUT("/team/users/:id/data-scope", rowLevelCtrl.UpdateUserDataScope)

	// 通知中心（站内通知 / 顶部铃铛 badge）
	notifCtrl := controller.NewNotificationController(service.NewNotificationService(db.GetDB()))
	auth.GET("/auth/notifications", notifCtrl.List)
	auth.POST("/auth/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/auth/notifications/read-all", notifCtrl.MarkAllRead)
	auth.GET("/auth/notifications/unread-count", notifCtrl.UnreadCount)

	// 通知中心 - 前端兼容别名（Notifications.vue 使用 /api/notifications）
	auth.GET("/notifications", notifCtrl.List)
	auth.GET("/notifications/unread-count", notifCtrl.UnreadCount)
	auth.POST("/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/notifications/read-all", notifCtrl.MarkAllRead)
}

// setupUserRoutes 用户管理路由
//
// 修复历史：
//   - 之前使用 controller.NewUserController()，该 Controller 操作 model.User 表
//     （独立的 TG 团队用户表，UUID 主键）。但 Profile.vue 实际更新的 admin 用户
//     位于 system_users 表（uint 主键），所以 PUT /api/users/:id 返回 "record not found"。
//   - 现改用 controller.NewSystemUserController()，该 Controller 直接操作 system_users 表。
//   - SystemUserController 已有 CreateUser/UpdateUser/DeleteUser/ResetPassword 方法，
//     缺少 GetUserList 别名（方法名叫 GetUsers），路由层用 func 包装。
func setupUserRoutes(auth *gin.RouterGroup) {
	userCtrl := controller.NewSystemUserController()
	// GetUserList 路由：SystemUserController 的方法叫 GetUsers，这里包装为 GetUserList
	auth.GET("/user/list", userCtrl.GetUsers)
	auth.GET("/users", userCtrl.GetUsers)
	auth.GET("/user/:id", userCtrl.GetUser)
	auth.GET("/users/:id", userCtrl.GetUser)
	auth.POST("/user", userCtrl.CreateUser)
	auth.POST("/users", userCtrl.CreateUser)
	auth.PUT("/user/:id", userCtrl.UpdateUser)
	auth.PUT("/users/:id", userCtrl.UpdateUser)
	auth.DELETE("/user/:id", userCtrl.DeleteUser)
	auth.DELETE("/users/:id", userCtrl.DeleteUser)
	auth.PUT("/user/:id/password", userCtrl.ResetPassword)
	auth.PUT("/users/:id/password", userCtrl.ResetPassword)
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
func setupShortLinkRoutes(auth *gin.RouterGroup) {
	shortLinkCtrl := controller.NewShortLinkController(service.NewShortLinkService(db.GetDB()))
	shortLinkStatsCtrl := controller.NewShortLinkStatsController(service.NewShortLinkService(db.GetDB()))

	// 短链管理
	auth.GET("/short-link/list", shortLinkCtrl.GetList)
	auth.POST("/short-link", shortLinkCtrl.Create)
	auth.PUT("/short-link/:id", shortLinkCtrl.Update)
	auth.DELETE("/short-link/:id", shortLinkCtrl.Delete)
	auth.GET("/short-link/:id", shortLinkCtrl.GetByID)
	// 短链统计
	auth.GET("/short-link/:id/stats", shortLinkStatsCtrl.GetStats)
	auth.GET("/short-link/all-stats", shortLinkStatsCtrl.GetAllStats)
	// 前端兼容路由
	auth.GET("/shortlink/list", shortLinkCtrl.GetList)
	auth.GET("/shortlink/:id", shortLinkCtrl.GetByID)
	auth.POST("/shortlink/create", shortLinkCtrl.Create)
	auth.PUT("/shortlink/update", func(c *gin.Context) { shortLinkCtrl.Update(c) })
	auth.DELETE("/shortlink/delete/:id", shortLinkCtrl.Delete)
	auth.POST("/shortlink/access", shortLinkCtrl.Create)
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
func setupEmailRoutes(auth *gin.RouterGroup) {
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

	// D 域 P1 缺口修复 - 邮件退订管理 + 打开率追踪
	emailUnsubscribeRepo := repository.NewEmailUnsubscribeRepository(db.GetDB())
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
func setupSmsRoutes(auth *gin.RouterGroup) {
	smsRepo := repository.NewSmsRepository()
	smsCtrl := controller.NewSmsController(service.NewSmsService(smsRepo))
	smsCtrl.RegisterRoutes(auth)

	// E 域 P1 缺口修复 - 短信退订管理 + 到达率追踪
	smsUnsubscribeRepo := repository.NewSmsUnsubscribeRepository(db.GetDB())
	smsUnsubscribeCtrl := controller.NewSmsUnsubscribeController(
		service.NewSmsUnsubscribeService(smsUnsubscribeRepo),
	)
	smsUnsubscribeCtrl.RegisterRoutes(nil, auth)

	smsDeliveryTrackerCtrl := controller.NewSmsDeliveryTrackerController(
		service.NewSmsDeliveryTrackerService(db.GetDB(), nil, nil),
	)
	smsDeliveryTrackerCtrl.RegisterRoutes(nil, auth)
}

// setupAutoReplyRoutes 自动回复路由
func setupAutoReplyRoutes(auth *gin.RouterGroup) {
	ragStack := knowledgesvc.NewRAGStack(db.GetDB())
	autoReplyCtrl := controller.NewAutoReplyController(service.NewAutoReplyService(db.GetDB()), ragStack)
	autoReplyManagerCtrl := controller.NewAutoReplyManagerController(service.NewAutoReplyService(db.GetDB()))
	xianyuAutoReplyCtrl := controller.NewXianyuAutoReplyController(service.NewXianyuAutoReplyService(db.GetDB()), ragStack)
	xiaohongshuAutoReplyCtrl := controller.NewXiaohongshuAutoReplyController(service.NewXiaohongshuAutoReplyService(db.GetDB()), ragStack)

	// 通用自动回复
	auth.POST("/auto-reply/start-login", autoReplyCtrl.StartLogin)
	auth.GET("/auto-reply/login-status", autoReplyCtrl.LoginStatus)
	auth.GET("/auto-reply/accounts", autoReplyCtrl.ListAccounts)
	auth.POST("/auto-reply/accounts", autoReplyCtrl.UpsertAccount)
	auth.POST("/auto-reply/accounts/:id/cookies", autoReplyCtrl.SaveCookies)
	auth.DELETE("/auto-reply/accounts/:id", autoReplyCtrl.DeleteAccount)
	auth.GET("/auto-reply/rule", autoReplyCtrl.GetRule)
	auth.POST("/auto-reply/rule", autoReplyCtrl.SaveRule)
	auth.GET("/auto-reply/logs", autoReplyCtrl.ListLogs)
	auth.POST("/auto-reply/start", autoReplyCtrl.Start)
	auth.POST("/auto-reply/stop", autoReplyCtrl.Stop)

	// 自动回复管理器
	auth.GET("/auto-reply/rules", autoReplyManagerCtrl.ListRules)
	auth.POST("/auto-reply/rules", autoReplyManagerCtrl.CreateRule)
	auth.PUT("/auto-reply/rules/:id", autoReplyManagerCtrl.UpdateRule)
	auth.DELETE("/auto-reply/rules/:id", autoReplyManagerCtrl.DeleteRule)
	auth.POST("/auto-reply/test-matching", autoReplyManagerCtrl.TestMatching)
	auth.POST("/auto-reply/simulate-message", autoReplyManagerCtrl.SimulateMessage)
	auth.POST("/auto-reply/test-batch-matching", autoReplyManagerCtrl.TestBatchMatching)
	auth.POST("/auto-reply/test-rate-limit", autoReplyManagerCtrl.TestRateLimit)
	auth.POST("/auto-reply/reset-daily-limit", autoReplyManagerCtrl.ResetDailyLimit)
	auth.GET("/auto-reply/rate-limit-stats", autoReplyManagerCtrl.GetRateLimitStats)
	auth.GET("/auto-reply/concurrent-stats", autoReplyManagerCtrl.GetConcurrentStats)
	auth.GET("/auto-reply/statistics", autoReplyManagerCtrl.GetStatistics)
	auth.GET("/auto-reply/headless", autoReplyCtrl.GetHeadlessMode)
	auth.POST("/auto-reply/headless", autoReplyCtrl.SetHeadlessMode)
	auth.POST("/auto-reply/headless/toggle", autoReplyCtrl.ToggleHeadless)
	auth.GET("/auto-reply/debug/status", autoReplyCtrl.GetDebugStatus)
	auth.POST("/auto-reply/debug/test-browser", autoReplyCtrl.TestBrowser)

	// 闲鱼自动回复
	auth.GET("/xianyu/auto-reply/accounts", xianyuAutoReplyCtrl.ListAccounts)
	auth.POST("/xianyu/auto-reply/login/start", xianyuAutoReplyCtrl.StartLogin)
	auth.GET("/xianyu/auto-reply/login/status", xianyuAutoReplyCtrl.LoginStatus)
	auth.POST("/xianyu/auto-reply/accounts", xianyuAutoReplyCtrl.UpsertAccount)
	auth.POST("/xianyu/auto-reply/accounts/:id/cookies", xianyuAutoReplyCtrl.SaveCookies)
	auth.DELETE("/xianyu/auto-reply/accounts/:id", xianyuAutoReplyCtrl.DeleteAccount)
	auth.GET("/xianyu/auto-reply/rules", xianyuAutoReplyCtrl.GetRule)
	auth.POST("/xianyu/auto-reply/rules", xianyuAutoReplyCtrl.SaveRule)
	auth.GET("/xianyu/auto-reply/logs", xianyuAutoReplyCtrl.ListLogs)
	auth.POST("/xianyu/auto-reply/start", xianyuAutoReplyCtrl.Start)
	auth.POST("/xianyu/auto-reply/stop", xianyuAutoReplyCtrl.Stop)
	auth.GET("/xianyu/auto-reply/health", xianyuAutoReplyCtrl.Health)

	// 闲鱼自动回复 - 别名路由（前端兼容：/xianyu-auto-reply/*）
	auth.GET("/xianyu-auto-reply/accounts", xianyuAutoReplyCtrl.ListAccounts)
	auth.GET("/xianyu-auto-reply/rule", xianyuAutoReplyCtrl.GetRule)
	auth.GET("/xianyu-auto-reply/logs", xianyuAutoReplyCtrl.ListLogs)
	auth.GET("/xianyu-auto-reply/login-status", xianyuAutoReplyCtrl.LoginStatus)

	// 小红书自动回复
	auth.POST("/xiaohongshu/auto-reply/start-login", xiaohongshuAutoReplyCtrl.StartLogin)
	auth.GET("/xiaohongshu/auto-reply/login-status", xiaohongshuAutoReplyCtrl.LoginStatus)
	auth.GET("/xiaohongshu/auto-reply/accounts", xiaohongshuAutoReplyCtrl.ListAccounts)
	auth.POST("/xiaohongshu/auto-reply/accounts", xiaohongshuAutoReplyCtrl.UpsertAccount)
	auth.POST("/xiaohongshu/auto-reply/accounts/:id/cookies", xiaohongshuAutoReplyCtrl.SaveCookies)
	auth.DELETE("/xiaohongshu/auto-reply/accounts/:id", xiaohongshuAutoReplyCtrl.DeleteAccount)
	auth.GET("/xiaohongshu/auto-reply/rules", xiaohongshuAutoReplyCtrl.GetRule)
	auth.POST("/xiaohongshu/auto-reply/rules", xiaohongshuAutoReplyCtrl.SaveRule)
	auth.GET("/xiaohongshu/auto-reply/logs", xiaohongshuAutoReplyCtrl.ListLogs)
	auth.POST("/xiaohongshu/auto-reply/start", xiaohongshuAutoReplyCtrl.Start)
	auth.POST("/xiaohongshu/auto-reply/stop", xiaohongshuAutoReplyCtrl.Stop)
	auth.GET("/xiaohongshu/auto-reply/health", xiaohongshuAutoReplyCtrl.Health)
}
