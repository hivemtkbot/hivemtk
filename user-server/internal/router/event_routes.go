package router

import (
	"marketing/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupEventRoutes 客户事件追踪(CDP)路由
// 提供 8 个事件端点 + 历史查询/统计
func setupEventRoutes(auth *gin.RouterGroup) {
	eventCtrl := controller.NewCustomerEventController()
	auth.POST("/events/track", eventCtrl.TrackEvent)
	auth.GET("/events/customer/:id", eventCtrl.GetEventHistory)
	auth.DELETE("/events/customer/:id", eventCtrl.DeleteEvent)
	auth.GET("/events/stats", eventCtrl.GetEventStats)
	auth.POST("/events/pageview", eventCtrl.TrackPageView)
	auth.POST("/events/click", eventCtrl.TrackClick)
	auth.POST("/events/purchase", eventCtrl.TrackPurchase)
	auth.POST("/events/signup", eventCtrl.TrackSignup)
	auth.POST("/events/login", eventCtrl.TrackLogin)
	auth.POST("/events/add-to-cart", eventCtrl.TrackAddToCart)
}
