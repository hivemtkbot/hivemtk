package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupWhatsappRoutes WhatsApp 管理路由
//
// 权限分级（2026-08-18）：写操作（Create/Update/Delete/StartLogin/CreateDraft/CreateJob 等）admin only
// 读操作任意登录用户
func setupWhatsappRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	whatsappCtrl := controller.NewWhatsappController()
	whatsappGroupMsgCtrl := controller.NewGroupMessagingController(
		whatsappCtrl.GetService(),
		service.NewClueService(),
		service.NewMessageQueueService(gormDB),
		service.NewWhatsAppTemplateService(gormDB),
	)
	// 读操作
	auth.GET("/whatsapp/accounts", whatsappCtrl.ListAccounts)
	auth.GET("/whatsapp/login-status", whatsappCtrl.LoginStatus)
	auth.GET("/whatsapp/drafts", whatsappCtrl.ListDrafts)
	auth.GET("/whatsapp/jobs", whatsappCtrl.ListJobs)
	auth.GET("/whatsapp/jobs/:id", whatsappCtrl.GetJob)
	auth.GET("/whatsapp/accounts/:id/login/status", whatsappCtrl.LoginStatus)
	auth.GET("/whatsapp/lead-groups", whatsappGroupMsgCtrl.GetLeadGroups)
	auth.GET("/whatsapp/group-messaging/status/:queue_id", whatsappGroupMsgCtrl.GetMessageStatus)
	auth.GET("/whatsapp/group-messaging/records", whatsappGroupMsgCtrl.GetSendRecords)
	auth.GET("/whatsapp/templates", whatsappGroupMsgCtrl.GetTemplates)
	auth.GET("/whatsapp/templates/:id", whatsappGroupMsgCtrl.GetTemplateByID)
	// 写操作：admin only（2026-08-18 防 staff 越权）
	admin := auth.Group("/whatsapp", middleware.AdminAuthMiddleware())
	{
		admin.POST("/accounts", whatsappCtrl.CreateAccount)
		admin.PUT("/accounts/:id", whatsappCtrl.UpdateAccount)
		admin.DELETE("/accounts/:id", whatsappCtrl.DeleteAccount)
		admin.POST("/accounts/:id/login/start", whatsappCtrl.StartLogin)
		admin.POST("/start-login", whatsappCtrl.StartLogin)
		admin.POST("/drafts", whatsappCtrl.CreateDraft)
		admin.PUT("/drafts/:id", whatsappCtrl.UpdateDraft)
		admin.DELETE("/drafts/:id", whatsappCtrl.DeleteDraft)
		admin.POST("/jobs", whatsappCtrl.CreateJob)
		admin.DELETE("/jobs/:id", whatsappCtrl.DeleteJob)
		admin.POST("/group-messaging/send", whatsappGroupMsgCtrl.SelectGroupAndSendMessage)
		admin.POST("/templates", whatsappGroupMsgCtrl.CreateTemplate)
		admin.PUT("/templates/:id", whatsappGroupMsgCtrl.UpdateTemplate)
		admin.DELETE("/templates/:id", whatsappGroupMsgCtrl.DeleteTemplate)
	}
}

// setupTelegramRoutes Telegram 机器人账号管理路由
// 用于配置 TG Bot Token + Webhook + 智能体开关，
// 配合 /api/webhook/telegram/{account_id} 自动触发智能体流程
func setupTelegramRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	telegramAccountCtrl := controller.NewTelegramAccountController(service.NewTelegramService(gormDB))
	telegramAccountCtrl.RegisterRoutes(auth)
}

// setupFeishuRoutes 飞书机器人账号管理路由
// 商户在 UI 配置 App ID / App Secret / Verification Token / Encrypt Key，
// 配合 /api/webhook/feishu/{account_id} 自动触发智能体流程
func setupFeishuRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	feishuCtrl := controller.NewFeishuAccountController(service.NewFeishuService(gormDB), service.NewFeishuIntegrationService(gormDB))
	feishuCtrl.RegisterRoutes(auth)
}

// setupWhatsAppCloudRoutes WhatsApp Cloud API 商业账号管理路由
// 商户在 UI 配置 Phone Number ID / WABA ID / Access Token / App Secret，
// 配合 /api/webhook/whatsapp/{account_id} 自动触发智能体流程
func setupWhatsAppCloudRoutes(auth *gin.RouterGroup, whatsappCloudSvc *service.WhatsAppCloudService, gormDB *gorm.DB) {
	waCtrl := controller.NewWhatsAppCloudAccountController(whatsappCloudSvc, service.NewWhatsAppCloudIntegrationService(gormDB))
	waCtrl.RegisterRoutes(auth)
}

// setupDingTalkAppRoutes 钉钉企业内部应用（支持回调收消息）路由
func setupDingTalkAppRoutes(auth *gin.RouterGroup, dingtalkAppSvc *service.DingTalkAppService) {
	dtCtrl := controller.NewDingTalkAppAccountController(dingtalkAppSvc)
	dtCtrl.RegisterRoutes(auth)
}

// setupWechatRoutes 微信公众号账号管理路由
// 商户在 UI 配置 App ID / App Secret / Token / EncodingAESKey，
// 配合 /api/webhook/wechat/{account_id} 自动触发智能体流程
func setupWechatRoutes(auth *gin.RouterGroup, gormDB *gorm.DB, ingressSvc *service.InboxIngressService) {
	wechatCtrl := controller.NewWechatController(service.NewWechatService(gormDB))
	if ingressSvc != nil {
		wechatCtrl.SetIngressSvc(ingressSvc)
	}
	wechatCtrl.RegisterRoutes(auth)
}

// setupWechatWebhookRoutes 微信公众号 webhook 路由（不需要认证）
// 微信服务器用 GET 验证 URL，用 POST 推送消息
func setupWechatWebhookRoutes(r *gin.RouterGroup, gormDB *gorm.DB, ingressSvc *service.InboxIngressService) {
	wechatCtrl := controller.NewWechatController(service.NewWechatService(gormDB))
	if ingressSvc != nil {
		wechatCtrl.SetIngressSvc(ingressSvc)
	}
	wechatCtrl.RegisterWebhookRoutes(r)
}

// setupTiktokRoutes TikTok 管理路由
func setupTiktokRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	tiktokCardCtrl := controller.NewTikTokCardController(
		service.NewTikTokCardServiceWithDB(gormDB),
	)
	tiktokCardCtrl.RegisterRoutes(auth)
}

