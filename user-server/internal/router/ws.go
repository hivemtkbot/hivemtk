// Package router - ws.go WebSocket 路由
// ============================================================================
// 5 层架构归属: L2 Router 层（路由层）
//   - 5 层架构: controller → service → repository → model → dto
//   - 不做业务逻辑；仅绑定路径与方法
//
// 设计依据: AI 智能体性能优化 WebSocket 流式路由
//
// 路由:
//   - GET /ws/chat?session_id=xxx&customer_id=xxx - WebSocket 流式对话入口
//
// 不破坏其他路由：RegisterWSRoutes 只在传入的 gin.Engine 上注册 /ws 路径；
// 其它路由（/api/chat/public、/api/auth 等）完全独立。
package router

import (
	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/service"
)

// RegisterWSRoutes 注册 WebSocket 路由（不破坏其他路由）
//
// 调用方: Setup() 中调用；建议在所有 auth 路由之后注册，便于与公开路由解耦。
//
// 参数:
//   - engine: gin.Engine 实例（不能为 nil）
//   - hub: ChatWSHub（必填，用于管理 WS 连接）
//   - engine_: *service.SalesEngine 实例（必填，提供 HandleStream）
//
// 设计说明:
//   - 仅绑定 /ws 前缀，不影响 /api/* 等其它路径
//   - 鉴权由 controller.HandleChatWS 内部按 query param 校验，不在此处加 middleware
//   - 不使用 auth group（WebSocket 浏览器端无法携带 Authorization header）
func RegisterWSRoutes(engine *gin.Engine, hub *controller.ChatWSHub, engine_ *service.SalesEngine) {
	if engine == nil {
		return
	}
	if hub == nil {
		return
	}
	if engine_ == nil {
		return
	}

	ctrl := controller.NewChatWSController(hub, engine_)

	engine.GET("/ws/chat", ctrl.HandleChatWS)
}
