package router

import (
	"context"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	dbutil "hivemtk-user/internal/pkg/db"
	"hivemtk-user/internal/service"
	"hivemtk-user/internal/service/translation"
	"hivemtk-user/internal/websocket"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupChatPublicRoutes 公开 chat API 路由（无 JWT，软解析 AppKey/Channel + 限流）
//
// 路由前缀：/api/chat/public
// 鉴权：AppKeyResolve（软解析：缺失时放行，使用默认 channel "default"）
// 限流：双维度（IP + channel）
// 多语言：LangResolverMiddleware（v1.2 出海方案，按 channel_id 解析双语言）
//
// 复用：VisitorChatService 内部调用 SmartCSOrchestrator 走 RAG + AI 决策
//
// 私域部署模式：用户自己部署本系统后，作为通道嵌入到自有网站，
// AppKey 不再作为强制凭证，仅作为渠道的软标识（用于日志追踪 + 未来多渠道管理）。
func setupChatPublicRoutes(public *gin.RouterGroup, db *gorm.DB, orchestrator *service.SmartCSOrchestrator, langResolver *translation.LangConfigResolver) {
	channelSvc := service.MustNewChatChannelService(db)
	agentBindingSvc := service.NewChannelAgentBindingService()
	visitorSvc := service.NewVisitorChatService(context.Background(), db, channelSvc, orchestrator, agentBindingSvc)

	chatPublic := public.Group("/chat/public")
	chatPublic.Use(middleware.AppKeyResolve(chatChannelResolver{svc: channelSvc}))
	chatPublic.Use(middleware.LangResolverMiddleware(langResolver))
	chatPublic.Use(middleware.VisitorRateLimitMiddleware())
	// 安全修复：添加 SanitizeMiddleware 对请求体做 PII 脱敏 + 内容清洗
	chatPublic.Use(middleware.SanitizeMiddleware())

	ctrl := controller.NewChatPublicController(visitorSvc, channelSvc)

	chatPublic.GET("/channel/:app_key/info", ctrl.GetChannelInfoByAppKey)

	chatPublic.POST("/sessions", ctrl.OpenSession)
	chatPublic.GET("/sessions/active", ctrl.GetActiveSession)
	chatPublic.GET("/sessions/recent-closed", ctrl.GetRecentClosedSessions)
	chatPublic.GET("/sessions/:session_id/messages", ctrl.GetMessages)
	chatPublic.GET("/sessions/:session_id/offline-messages", ctrl.GetOfflineMessages)
	chatPublic.POST("/sessions/:session_id/messages", ctrl.SendMessage)
	chatPublic.POST("/sessions/:session_id/transfer", ctrl.RequestHumanTransfer)
	chatPublic.POST("/sessions/:session_id/close", ctrl.CloseSession)
	chatPublic.POST("/sessions/:session_id/rate", ctrl.RateSession)

	chatPublic.GET("/agents/available", ctrl.CountAvailableAgents)

	chatPublic.GET("/upload-token", ctrl.GetUploadToken)
}

// setupChatPublicWebSocket 注册访客 WebSocket 端点
// GET /api/ws/visitor?session_id=xxx&visitor_id=xxx&channel_id=xxx
func setupChatPublicWebSocket(r *gin.Engine, langResolver *translation.LangConfigResolver) {
	visitorWS := websocket.NewVisitorWSHandler(dbutil.GetDB())
	visitorWS.SetLangResolver(langResolver)
	r.GET("/api/ws/visitor", visitorWS.HandleVisitorWebSocket)
}

// setupChatChannelAdminRoutes B 端渠道管理路由
//
// 权限分级（2026-08-18 多角度审计修复）：
//   - 渠道 CRUD / rotate-key / reset-secret 全部 admin only（含 app_key/app_secret 凭据）
//   - List/Get 任意登录用户（业务展示）
func setupChatChannelAdminRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	channelSvc := service.MustNewChatChannelService(db)
	ctrl := controller.NewChatChannelController(channelSvc)

	auth.GET("/chat-channels", ctrl.List)
	auth.GET("/chat-channels/:channel_id", ctrl.Get)
	admin := auth.Group("/chat-channels", middleware.AdminAuthMiddleware())
	{
		admin.POST("", ctrl.Create)
		admin.PUT("/:channel_id", ctrl.Update)
		admin.DELETE("/:channel_id", ctrl.Delete)
		admin.POST("/:channel_id/rotate-key", ctrl.RotateAppKey)
		admin.POST("/:channel_id/reset-secret", ctrl.ResetAppSecret)
	}
}


// setupWeComRoutes 企业微信管理路由
//
// 权限分级（2026-08-18）：
//   - 写操作（Create/Update/Delete/Sync/Send/Refresh）必须 admin（corp_secret 等敏感凭据）
//   - 读操作（List/Get/Customers/Groups/Messages/Tags）任意登录用户
func setupWeComRoutes(auth *gin.RouterGroup, gormDB *gorm.DB) {
	wecomCtrl := controller.NewWeComController(service.NewWeComServiceWithDB(gormDB))

	auth.GET("/wecom/accounts", wecomCtrl.GetAccountList)
	auth.GET("/wecom/accounts/:id", wecomCtrl.GetAccountByID)
	auth.GET("/wecom/customers", wecomCtrl.GetCustomerList)
	auth.GET("/wecom/groups", wecomCtrl.GetGroupList)
	auth.GET("/wecom/messages", wecomCtrl.GetMessageList)
	auth.GET("/wecom/tags", wecomCtrl.GetTagList)
	admin := auth.Group("/wecom", middleware.AdminAuthMiddleware())
	{
		admin.POST("/accounts", wecomCtrl.CreateAccount)
		admin.PUT("/accounts/:id", wecomCtrl.UpdateAccount)
		admin.DELETE("/accounts/:id", wecomCtrl.DeleteAccount)
		admin.POST("/accounts/:id/sync-customers", wecomCtrl.SyncCustomers)
		admin.POST("/accounts/:id/sync-groups", wecomCtrl.SyncGroups)
		admin.POST("/accounts/:id/send-message", wecomCtrl.SendMessage)
		admin.POST("/accounts/:id/refresh", wecomCtrl.RefreshAccount)
		admin.POST("/accounts/:id/sync-tags", wecomCtrl.SyncTags)
	}
}


func setupCardShareRoutes(r *gin.Engine, gormDB *gorm.DB) {
	douyinCardCtrl := controller.NewDouyinCardController(service.NewDouyinCardService(gormDB))
	r.GET("/share/douyin/:id", douyinCardCtrl.SharePage)

	kuaishouCardCtrl := controller.NewKuaishouCardController(service.NewKuaishouCardService(gormDB))
	r.GET("/share/kuaishou/:id", kuaishouCardCtrl.SharePage)

	xiaohongshuCardCtrl := controller.NewXiaohongshuCardController(service.NewXiaohongshuCardService(gormDB))
	r.GET("/share/xiaohongshu/:id", xiaohongshuCardCtrl.SharePage)

	xianyuCardCtrl := controller.NewXianyuCardController(service.NewXianyuCardService(gormDB), service.NewXianyuCardStatsService(gormDB))
	r.GET("/share/xianyu/:id", xianyuCardCtrl.SharePage)
}

