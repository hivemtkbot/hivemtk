package router

import (
	"context"

	"marketing/internal/controller"
	"marketing/internal/middleware"
	dbutil "marketing/internal/pkg/utils/db"
	"marketing/internal/service"
	"marketing/internal/websocket"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupChatPublicRoutes 公开 chat API 路由（无 JWT，软解析 AppKey/Channel + 限流）
//
// 路由前缀：/api/chat/public
// 鉴权：AppKeyResolve（软解析：缺失时放行，使用默认 channel "default"）
// 限流：双维度（IP + channel）
//
// 复用：VisitorChatService 内部调用 SmartCSOrchestrator 走 RAG + AI 决策
//
// 私域部署模式（2026-07-17 优化）：用户自己部署本系统后，作为通道嵌入到自有网站，
// AppKey 不再作为强制凭证，仅作为渠道的软标识（用于日志追踪 + 未来多渠道管理）。
func setupChatPublicRoutes(public *gin.RouterGroup, db *gorm.DB, orchestrator *service.SmartCSOrchestrator) {
	channelSvc := service.MustNewChatChannelService(db)
	agentBindingSvc := service.NewChannelAgentBindingService()
	visitorSvc := service.NewVisitorChatService(context.Background(), db, channelSvc, orchestrator, agentBindingSvc)

	// 公开路由组：AppKey 软解析 + 访客限流
	chatPublic := public.Group("/chat/public")
	chatPublic.Use(middleware.AppKeyResolve(channelSvc))
	chatPublic.Use(middleware.VisitorRateLimitMiddleware())

	ctrl := controller.NewChatPublicController(visitorSvc, channelSvc)

	// 渠道信息查询（widget 安装引导的连通性测试用）
	chatPublic.GET("/channel/:app_key/info", ctrl.GetChannelInfoByAppKey)

	// 会话管理
	chatPublic.POST("/sessions", ctrl.OpenSession)
	chatPublic.GET("/sessions/active", ctrl.GetActiveSession)
	chatPublic.GET("/sessions/recent-closed", ctrl.GetRecentClosedSessions)
	chatPublic.GET("/sessions/:session_id/messages", ctrl.GetMessages)
	chatPublic.GET("/sessions/:session_id/offline-messages", ctrl.GetOfflineMessages)
	chatPublic.POST("/sessions/:session_id/messages", ctrl.SendMessage)
	chatPublic.POST("/sessions/:session_id/transfer", ctrl.RequestHumanTransfer)
	chatPublic.POST("/sessions/:session_id/close", ctrl.CloseSession)
	chatPublic.POST("/sessions/:session_id/rate", ctrl.RateSession)

	// 资源查询
	chatPublic.GET("/agents/available", ctrl.CountAvailableAgents)

	// 附件上传（2026-07-17 私域部署：访客直传七牛）
	chatPublic.GET("/upload-token", ctrl.GetUploadToken)
}

// setupChatPublicWebSocket 注册访客 WebSocket 端点
// GET /api/ws/visitor?session_id=xxx&visitor_id=xxx&channel_id=xxx
func setupChatPublicWebSocket(r *gin.Engine) {
	visitorWS := websocket.NewVisitorWSHandler(dbutil.GetDB())
	r.GET("/api/ws/visitor", visitorWS.HandleVisitorWebSocket)
}

// setupChatChannelAdminRoutes B 端渠道管理路由（需要 JWT）
func setupChatChannelAdminRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	channelSvc := service.MustNewChatChannelService(db)
	ctrl := controller.NewChatChannelController(channelSvc)

	g := auth.Group("/chat-channels")
	g.GET("", ctrl.List)
	g.POST("", ctrl.Create)
	g.GET("/:channel_id", ctrl.Get)
	g.PUT("/:channel_id", ctrl.Update)
	g.DELETE("/:channel_id", ctrl.Delete)
	g.POST("/:channel_id/rotate-key", ctrl.RotateAppKey)
	g.POST("/:channel_id/reset-secret", ctrl.ResetAppSecret)
}
