package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupChannelOverviewRoutes 13 渠道统一概览 + 客户渠道绑定管理
//
// 2026-08-16 严肃化：解决"用户找不到配置入口"问题。
// 提供统一入口列出所有 13 渠道的当前状态、配置 URL、必填字段，
// 以及客户渠道绑定管理（手动补全客户的渠道身份）。
func setupChannelOverviewRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	ov := controller.NewChannelOverviewController(db)

	// 渠道总览（dashboard 风格）
	auth.GET("/channels/overview", ov.Overview)

	// 客户渠道绑定管理
	auth.POST("/channels/bind", ov.BindChannel)
	auth.GET("/channels/customer/:customer_id", ov.ListCustomerChannels)
}
