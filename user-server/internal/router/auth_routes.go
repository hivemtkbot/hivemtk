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
	auth.POST("/auth/logout", authCtrl.Logout)
	auth.GET("/auth/current-user", authCtrl.GetCurrentUser)
	auth.POST("/auth/change-password", authCtrl.ChangePassword)

	auth.POST("/auth/mfa/setup", authCtrl.SetupMFA)
	auth.POST("/auth/mfa/confirm", authCtrl.ConfirmMFASetup)
	auth.POST("/auth/mfa/disable", authCtrl.DisableMFA)
	auth.GET("/auth/mfa/status", authCtrl.GetMFAStatus)

	auth.GET("/auth/login-events", authCtrl.ListLoginEvents)
	auth.GET("/auth/security-alerts", authCtrl.ListSecurityAlerts)
	auth.POST("/auth/security-alerts/:id/resolve", authCtrl.ResolveSecurityAlert)

	anomalyCtrl := controller.NewAnomalyLoginDetectorController()
	auth.GET("/auth/anomaly/login-events", anomalyCtrl.ListLoginEvents)
	auth.GET("/auth/anomaly/alerts", anomalyCtrl.ListAlerts)
	auth.POST("/auth/anomaly/alerts/:id/resolve", anomalyCtrl.ResolveAlert)
	auth.POST("/auth/anomaly/alerts/:id/ignore", anomalyCtrl.IgnoreAlert)

	auth.GET("/auth/password-policy", authCtrl.GetPasswordPolicy)
	auth.PUT("/auth/password-policy", authCtrl.SavePasswordPolicy)

	notifCtrl := controller.NewNotificationController(service.NewNotificationService(gormDB))
	auth.GET("/auth/notifications", notifCtrl.List)
	auth.POST("/auth/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/auth/notifications/read-all", notifCtrl.MarkAllRead)
	auth.GET("/auth/notifications/unread-count", notifCtrl.UnreadCount)

	auth.GET("/notifications", notifCtrl.List)
	auth.GET("/notifications/unread-count", notifCtrl.UnreadCount)
	auth.POST("/notifications/:id/read", notifCtrl.MarkRead)
	auth.POST("/notifications/read-all", notifCtrl.MarkAllRead)

}

// setupUserRoutes 用户管理路由
func setupUserRoutes(auth *gin.RouterGroup) {
	userCtrl := controller.NewSystemUserController()
	auth.GET("/user/list", userCtrl.GetUsers)
	auth.GET("/users", userCtrl.GetUsers)
	auth.GET("/user/:id", userCtrl.GetUser)
	auth.GET("/users/:id", userCtrl.GetUser)

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

// setupAlertRoutes 告警规则路由
//
// 权限分级（plan v3.1 §T8）：
//   - 读（List/Get/Histories）：任意登录用户
//   - 写（Create/Update/Delete/SetStatus/ResolveHistory）：admin only
//   - 防 staff 自建"假告警"或关闭关键告警掩盖越权行为。
func setupAlertRoutes(auth *gin.RouterGroup) {
	alertCtrl := controller.NewAlertRuleController()

	auth.GET("/alerts/rules", alertCtrl.List)
	auth.GET("/alerts/rules/:id", alertCtrl.GetByID)
	auth.GET("/alerts/histories", alertCtrl.ListHistory)
	// OpsOverview 顶栏未读告警角标（前端 GET /api/monitor/alerts/unread）
	auth.GET("/monitor/alerts/unread", alertCtrl.Unread)

	admin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		admin.POST("/alerts/rules", alertCtrl.Create)
		admin.PUT("/alerts/rules/:id", alertCtrl.Update)
		admin.DELETE("/alerts/rules/:id", alertCtrl.Delete)
		admin.PUT("/alerts/rules/status", alertCtrl.SetStatus)
		admin.POST("/alerts/histories/resolve", alertCtrl.ResolveHistory)
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
	auth.GET("/accounts/list", accountCtrl.GetAccounts)
	auth.POST("/accounts/create", accountCtrl.CreateAccount)
	auth.PUT("/accounts/update/:id", accountCtrl.UpdateAccount)
	auth.DELETE("/accounts/delete/:id", accountCtrl.DeleteAccount)
}

// setupShortLinkRoutes 短链管理路由
//
// 权限分级（2026-08-18 三轮发现）：写操作（Create/Update/Delete）admin only
// 防 staff 把短链 target_url 改成恶意地址 → 客户扫码跳转钓鱼站。
func setupShortLinkRoutes(auth *gin.RouterGroup, public *gin.RouterGroup, gormDB *gorm.DB) {
	shortLinkCtrl := controller.NewShortLinkController(service.NewShortLinkService(gormDB))
	shortLinkStatsCtrl := controller.NewShortLinkStatsController(service.NewShortLinkService(gormDB))

	// 读操作：任意登录用户（业务展示）
	auth.GET("/short-link/list", shortLinkCtrl.GetList)
	auth.GET("/short-link/:id", shortLinkCtrl.GetByID)
	auth.GET("/short-link/:id/stats", shortLinkStatsCtrl.GetStats)
	auth.GET("/short-link/all-stats", shortLinkStatsCtrl.GetAllStats)
	auth.GET("/shortlink/list", shortLinkCtrl.GetList)
	auth.GET("/shortlink/:id", shortLinkCtrl.GetByID)
	auth.GET("/shortlink/:id/stats", shortLinkStatsCtrl.GetStats)
	auth.GET("/shortlink/stats/all", shortLinkStatsCtrl.GetAllStats)
	// 写操作：admin only（防短链钓鱼）
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		admin.POST("/short-link", shortLinkCtrl.Create)
		admin.PUT("/short-link/:id", shortLinkCtrl.Update)
		admin.DELETE("/short-link/:id", shortLinkCtrl.Delete)
		admin.POST("/shortlink/create", shortLinkCtrl.Create)
		admin.PUT("/shortlink/update", func(c *gin.Context) { shortLinkCtrl.Update(c) })
		admin.DELETE("/shortlink/delete/:id", shortLinkCtrl.Delete)
		admin.POST("/shortlink/generate", shortLinkCtrl.Create)
		admin.POST("/shortlink/:id/share", shortLinkStatsCtrl.ShareShortLink)
	}
	public.POST("/short-link/access", shortLinkCtrl.AccessShortLink)
	public.POST("/shortlink/access", shortLinkCtrl.AccessShortLink)
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

	auth.GET("/email/list", emailListCtrl.GetEmailListList)
	auth.POST("/email/list", emailListCtrl.CreateEmailList)
	auth.PUT("/email/list/:id", emailListCtrl.UpdateEmailList)
	auth.DELETE("/email/list/:id", emailListCtrl.DeleteEmailList)
	auth.GET("/email/list/:id", emailListCtrl.GetEmailListDetail)
	auth.POST("/email/list/:id/trace", emailListCtrl.TraceEmail)

	auth.GET("/email/smtp", emailSmtpCtrl.GetEmailSmtpList)
	auth.GET("/email/smtp/:id", emailSmtpCtrl.GetEmailSmtp)
	auth.POST("/email/smtp", emailSmtpCtrl.CreateEmailSmtp)
	auth.PUT("/email/smtp/:id", emailSmtpCtrl.UpdateEmailSmtp)
	auth.DELETE("/email/smtp/:id", emailSmtpCtrl.DeleteEmailSmtp)

	auth.GET("/email/drafts", emailDraftCtrl.GetEmailDraftList)
	auth.POST("/email/drafts", emailDraftCtrl.CreateEmailDraft)
	auth.GET("/email/drafts/:id", emailDraftCtrl.GetEmailDraftDetail)
	auth.PUT("/email/drafts/:id", emailDraftCtrl.UpdateEmailDraft)
	auth.DELETE("/email/drafts/:id", emailDraftCtrl.DeleteEmailDraft)

	auth.GET("/email/jobs", emailJobsCtrl.GetEmailJobsList)
	auth.POST("/email/jobs", emailJobsCtrl.CreateEmailJobs)
	auth.DELETE("/email/jobs/:id", emailJobsCtrl.DeleteEmailJobs)
	auth.GET("/email/jobs/:id", emailJobsCtrl.GetEmailJobsDetail)

	auth.POST("/email/send", emailSendCtrl.SendEmail)

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


