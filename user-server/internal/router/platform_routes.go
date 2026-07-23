package router

import (
	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// setupWhatsappRoutes WhatsApp 管理路由
func setupWhatsappRoutes(auth *gin.RouterGroup) {
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
	// 前端兼容路由
	auth.POST("/whatsapp/accounts/:id/login/start", whatsappCtrl.StartLogin)
	auth.GET("/whatsapp/accounts/:id/login/status", whatsappCtrl.LoginStatus)

	// WhatsApp 群发消息功能
	whatsappGroupMsgCtrl := controller.NewGroupMessagingController(
		whatsappCtrl.GetService(),
		service.NewClueService(),
		service.NewMessageQueueService(db.GetDB()),
		service.NewWhatsAppTemplateService(db.GetDB()),
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
func setupTelegramRoutes(auth *gin.RouterGroup) {
	telegramAccountCtrl := controller.NewTelegramAccountController(service.NewTelegramService(db.GetDB()))
	telegramAccountCtrl.RegisterRoutes(auth)
}

// setupFeishuRoutes 飞书机器人账号管理路由
// 商户在 UI 配置 App ID / App Secret / Verification Token / Encrypt Key，
// 配合 /api/webhook/feishu/{account_id} 自动触发智能体流程
func setupFeishuRoutes(auth *gin.RouterGroup) {
	feishuCtrl := controller.NewFeishuAccountController(service.NewFeishuService(db.GetDB()), service.NewFeishuIntegrationService(db.GetDB()))
	feishuCtrl.RegisterRoutes(auth)
}

// setupWhatsAppCloudRoutes WhatsApp Cloud API 商业账号管理路由
// 商户在 UI 配置 Phone Number ID / WABA ID / Access Token / App Secret，
// 配合 /api/webhook/whatsapp/{account_id} 自动触发智能体流程
func setupWhatsAppCloudRoutes(auth *gin.RouterGroup) {
	waCtrl := controller.NewWhatsAppCloudAccountController(service.NewWhatsAppCloudService(db.GetDB()), service.NewWhatsAppCloudIntegrationService(db.GetDB()))
	waCtrl.RegisterRoutes(auth)
}

// setupTiktokRoutes TikTok 管理路由
func setupTiktokRoutes(auth *gin.RouterGroup) {
	// TikTok 卡片管理
	tiktokCardCtrl := controller.NewTikTokCardController()
	tiktokCardCtrl.RegisterRoutes(auth)

	// TikTok 自动回复管理
	tiktokAutoReplyCtrl := controller.NewTikTokAutoReplyController()
	tiktokAutoReplyCtrl.RegisterRoutes(auth)
}
