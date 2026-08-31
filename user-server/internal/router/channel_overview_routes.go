package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupChannelOverviewRoutes 13 渠道统一概览 + 客户渠道绑定管理
//
// 2026-08-16 严肃化：解决"用户找不到配置入口"问题。
// 提供统一入口列出所有 13 渠道的当前状态、配置 URL、必填字段，
// 以及客户渠道绑定管理（手动补全客户的渠道身份）。
//
// P0-23 权限分级（2026-08-31 四轮加固）：
//   - GET channels/overview + GET customer channels：任意登录用户
//   - POST channels/bind：admin only（手动改客户渠道身份属于敏感数据操作）
func setupChannelOverviewRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	svc := service.NewChannelOverviewService(repository.NewChannelOverviewRepository(db))
	ov := controller.NewChannelOverviewController(svc)

	// 渠道总览（dashboard 风格）
	auth.GET("/channels/overview", ov.Overview)

	// 客户渠道绑定管理：读 auth，写 admin
	auth.GET("/channels/customer/:customer_id", ov.ListCustomerChannels)
	admin := auth.Group("", middleware.AdminAuthMiddleware())
	admin.POST("/channels/bind", ov.BindChannel)
}
