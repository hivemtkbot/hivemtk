package router

// role_routes.go 角色管理路由
//
// 五层架构归属：L1 路由层
// 路由：/api/system/roles/*（仅 admin 可见）
//
// 阶段 5 范围：
//   - GET    /api/system/roles                列出 3 档系统角色 + 成员数
//   - GET    /api/system/roles/:code          单个角色详情
//   - GET    /api/system/roles/:code/members  角色下成员列表（分页）
//
// 受 RequireAdminMiddleware 保护（在路由组上声明，子路由全部继承）。

import (
	"github.com/gin-gonic/gin"

	"marketing/internal/controller"
	"marketing/internal/middleware"
)

// setupRoleRoutes 注册 /api/system/roles/* 路由
func setupRoleRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewRoleController()
	admin := auth.Group("/system/roles", middleware.RequireAdminMiddleware())
	{
		admin.GET("", ctrl.ListRoles)
		admin.GET("/:code", ctrl.GetRole)
		admin.GET("/:code/members", ctrl.ListMembers)
	}
}
