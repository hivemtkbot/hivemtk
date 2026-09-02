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
	// AD-P0-4: 生成 MFA 恢复码端点（Service 层已有 GenerateBackupCodes）
	auth.POST("/auth/mfa/backup-codes", authCtrl.GenerateBackupCodes)

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
        auth.GET("/users/search", userCtrl.SearchUsers)
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
//
// P0-8 权限分级（2026-08-31 四轮加固）：
//   - 读（List/Get）：任意登录用户
//   - 写（Create/Update/Delete）：admin only
func setupAccountRoutes(auth *gin.RouterGroup) {
	accountCtrl := controller.NewAccountController()
	auth.GET("/account/list", accountCtrl.GetAccounts)
	auth.GET("/account/:id", accountCtrl.GetAccount)
	auth.GET("/accounts/list", accountCtrl.GetAccounts)
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	admin.POST("/account", accountCtrl.CreateAccount)
	admin.PUT("/account/:id", accountCtrl.UpdateAccount)
	admin.DELETE("/account/:id", accountCtrl.DeleteAccount)
	admin.POST("/accounts/create", accountCtrl.CreateAccount)
	admin.PUT("/accounts/update/:id", accountCtrl.UpdateAccount)
	admin.DELETE("/accounts/delete/:id", accountCtrl.DeleteAccount)
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
		admin.PUT("/shortlink/update", shortLinkCtrl.Update)
		admin.DELETE("/shortlink/delete/:id", shortLinkCtrl.Delete)
		admin.POST("/shortlink/generate", shortLinkCtrl.Create)
		admin.POST("/shortlink/:id/share", shortLinkStatsCtrl.ShareShortLink)
	}
	public.POST("/short-link/access", shortLinkCtrl.AccessShortLink)
	public.POST("/shortlink/access", shortLinkCtrl.AccessShortLink)
}

// setupLiveCodeRoutes 活码管理路由
//
// 权限分级（AD-P0-1 2026-09-02 加固）：
//   - 读（List/Get/Stats/QRCodes/QRStats）：任意登录用户
//   - 写（Create/Update/Delete/GenerateQRCode/Share/UpdateLiveCodeQR/DeleteLiveCodeQR）：admin only
// 防 staff 绕过权限篡改活码目标 URL / 恶意生成 QR。
func setupLiveCodeRoutes(auth *gin.RouterGroup, liveCodeController *controller.LiveCodeController) {
	// 读操作：任意登录用户
	auth.GET("/live-code/list", liveCodeController.GetList)
	auth.GET("/live-codes/list", liveCodeController.GetList)
	auth.GET("/live-code/:id", liveCodeController.GetByID)
	auth.GET("/live-code/:id/stats", liveCodeController.GetStats)
	auth.GET("/live-code/:id/qr-codes", liveCodeController.GetQRCodes)
	auth.GET("/live-code/:id/qr-stats", liveCodeController.GetQRStats)
	auth.GET("/live-codes/:id", liveCodeController.GetByID)
	auth.GET("/live-codes/:id/stats", liveCodeController.GetStats)
	auth.GET("/live-codes/:id/qrcodes", liveCodeController.GetQRCodes)
	auth.GET("/live-codes/qrcodes/:qr_id/stats", liveCodeController.GetQRStats)

	// 写操作：admin only
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	{
		admin.POST("/live-code", liveCodeController.Create)
		admin.PUT("/live-code/:id", liveCodeController.Update)
		admin.DELETE("/live-code/:id", liveCodeController.Delete)
		admin.POST("/live-code/:id/qr-code", liveCodeController.GenerateQRCode)
		admin.POST("/live-code/:id/share", liveCodeController.Share)
		admin.DELETE("/live-code/:id/qr-codes/:qr_id", liveCodeController.DeleteLiveCodeQR)
		admin.PUT("/live-code/:id/qr-codes/:qr_id", liveCodeController.UpdateLiveCodeQR)
		admin.POST("/live-codes/create", liveCodeController.Create)
		admin.PUT("/live-codes/:id/update", liveCodeController.Update)
		admin.DELETE("/live-codes/:id/delete", liveCodeController.Delete)
		admin.POST("/live-codes/:id/qrcodes/create", liveCodeController.GenerateQRCode)
		admin.POST("/live-codes/:id/share", liveCodeController.Share)
		admin.DELETE("/live-codes/qrcodes/:qr_id/delete", liveCodeController.DeleteLiveCodeQR)
		admin.PUT("/live-codes/qrcodes/:qr_id/update", liveCodeController.UpdateLiveCodeQR)
	}
}

// setupEmailRoutes 邮件管理路由
//
// P0-9 权限分级（2026-08-31 四轮加固）：
//   - 读（List/Get）：任意登录用户
//   - SMTP 凭据写操作 / send / jobs CRUD：admin only
// 防 staff 用系统 SMTP 向外发垃圾邮件 / 改 SMTP 凭据劫持邮件。
func setupEmailRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	emailListCtrl := controller.NewEmailListController()
	emailSmtpCtrl := controller.NewEmailSmtpController()
	emailDraftCtrl := controller.NewEmailDraftController()
	emailJobsCtrl := controller.NewEmailJobsController()
	emailSendCtrl := controller.NewEmailSendController()

	emailAdmin := auth.Group("", middleware.AdminAuthMiddleware())

	// 邮件列表（读写 admin？保持写 admin）
	auth.GET("/email/list", emailListCtrl.GetEmailListList)
	auth.GET("/email/list/:id", emailListCtrl.GetEmailListDetail)
	emailAdmin.POST("/email/list", emailListCtrl.CreateEmailList)
	emailAdmin.PUT("/email/list/:id", emailListCtrl.UpdateEmailList)
	emailAdmin.DELETE("/email/list/:id", emailListCtrl.DeleteEmailList)
	emailAdmin.POST("/email/list/:id/trace", emailListCtrl.TraceEmail)

	// SMTP 凭据：读 admin 以下保持 auth，写 admin only
	auth.GET("/email/smtp", emailSmtpCtrl.GetEmailSmtpList)
	auth.GET("/email/smtp/:id", emailSmtpCtrl.GetEmailSmtp)
	emailAdmin.POST("/email/smtp", emailSmtpCtrl.CreateEmailSmtp)
	emailAdmin.PUT("/email/smtp/:id", emailSmtpCtrl.UpdateEmailSmtp)
	emailAdmin.DELETE("/email/smtp/:id", emailSmtpCtrl.DeleteEmailSmtp)

	// 草稿（staff 可以写草稿？保持原样不改）
	auth.GET("/email/drafts", emailDraftCtrl.GetEmailDraftList)
	auth.POST("/email/drafts", emailDraftCtrl.CreateEmailDraft)
	auth.GET("/email/drafts/:id", emailDraftCtrl.GetEmailDraftDetail)
	auth.PUT("/email/drafts/:id", emailDraftCtrl.UpdateEmailDraft)
	auth.DELETE("/email/drafts/:id", emailDraftCtrl.DeleteEmailDraft)

	// Jobs + Send：admin only（实际发邮件）
	auth.GET("/email/jobs", emailJobsCtrl.GetEmailJobsList)
	emailAdmin.POST("/email/jobs", emailJobsCtrl.CreateEmailJobs)
	emailAdmin.DELETE("/email/jobs/:id", emailJobsCtrl.DeleteEmailJobs)
	auth.GET("/email/jobs/:id", emailJobsCtrl.GetEmailJobsDetail)

	emailAdmin.POST("/email/send", emailSendCtrl.SendEmail)

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


