package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupWhatsappRoutes WhatsApp 管理路由
func setupWhatsappRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	whatsappCtrl := controller.NewWhatsappController()
	auth.GET("/whatsapp/accounts", whatsappCtrl.ListAccounts)
	auth.POST("/whatsapp/accounts", whatsappCtrl.CreateAccount)
	auth.POST("/whatsapp/start-login", whatsappCtrl.StartLogin)
	auth.GET("/whatsapp/login-status", whatsappCtrl.LoginStatus)
	auth.GET("/whatsapp/drafts", whatsappCtrl.ListDrafts)
	auth.POST("/whatsapp/drafts", whatsappCtrl.CreateDraft)
	auth.POST("/whatsapp/jobs", whatsappCtrl.CreateJob)
	auth.PUT("/whatsapp/accounts/:id", whatsappCtrl.UpdateAccount)
	auth.DELETE("/whatsapp/accounts/:id", whatsappCtrl.DeleteAccount)
	auth.PUT("/whatsapp/drafts/:id", whatsappCtrl.UpdateDraft)
	auth.DELETE("/whatsapp/drafts/:id", whatsappCtrl.DeleteDraft)
	auth.GET("/whatsapp/jobs", whatsappCtrl.ListJobs)
	auth.GET("/whatsapp/jobs/:id", whatsappCtrl.GetJob)
	auth.DELETE("/whatsapp/jobs/:id", whatsappCtrl.DeleteJob)
	auth.POST("/whatsapp/accounts/:id/login/start", whatsappCtrl.StartLogin)
	auth.GET("/whatsapp/accounts/:id/login/status", whatsappCtrl.LoginStatus)

	whatsappGroupMsgCtrl := controller.NewGroupMessagingController(
		whatsappCtrl.GetService(),
		service.NewClueService(),
		service.NewMessageQueueService(gormDB),
		service.NewWhatsAppTemplateService(gormDB),
	)
	auth.GET("/whatsapp/lead-groups", whatsappGroupMsgCtrl.GetLeadGroups)
	auth.POST("/whatsapp/group-messaging/send", whatsappGroupMsgCtrl.SelectGroupAndSendMessage)
	auth.GET("/whatsapp/group-messaging/status/:queue_id", whatsappGroupMsgCtrl.GetMessageStatus)
	auth.GET("/whatsapp/group-messaging/records", whatsappGroupMsgCtrl.GetSendRecords)
	auth.GET("/whatsapp/templates", whatsappGroupMsgCtrl.GetTemplates)
	auth.POST("/whatsapp/templates", whatsappGroupMsgCtrl.CreateTemplate)
	auth.PUT("/whatsapp/templates/:id", whatsappGroupMsgCtrl.UpdateTemplate)
	auth.DELETE("/whatsapp/templates/:id", whatsappGroupMsgCtrl.DeleteTemplate)
	auth.GET("/whatsapp/templates/:id", whatsappGroupMsgCtrl.GetTemplateByID)
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

