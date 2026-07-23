package router

// system_user_routes.go 系统用户管理路由（人员管理）
//
// 五层架构归属：L1 路由层
// 路由：/api/system/users/*（仅 admin 可见）
//
// 阶段 4 范围：
//   - GET    /api/system/users       列表
//   - GET    /api/system/users/:id   详情
//   - POST   /api/system/users       创建
//   - PUT    /api/system/users/:id   更新
//   - DELETE /api/system/users/:id   删除
//
// 受 RequireAdminMiddleware 保护（在路由组上声明，子路由全部继承）。

import (
	"github.com/gin-gonic/gin"

	"marketing/internal/controller"
	"marketing/internal/middleware"
)

// setupSystemUserRoutes 注册 /api/system/users/* 路由
func setupSystemUserRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewSystemUserAdminController()
	admin := auth.Group("/system/users", middleware.RequireAdminMiddleware())
	{
		admin.GET("", ctrl.GetUsers)
		admin.GET("/:id", ctrl.GetByID)
		admin.POST("", ctrl.Create)
		admin.PUT("/:id", ctrl.Update)
		admin.DELETE("/:id", ctrl.Delete)
	}
}
