package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"

	"github.com/gin-gonic/gin"
)

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

// ============================================================================
// 以下内容合并自 role_routes.go（P1-2 router 文件数收敛）
// ============================================================================

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

// ============================================================================
// 以下内容合并自 permission_routes.go（P1-2 router 文件数收敛）
// ============================================================================

// permission_routes.go 授权管理路由
//
// 五层架构归属：L1 路由层
// 路由：/api/system/permissions/*（仅 admin 可见）
//
// 阶段 6 范围：
//   - GET /api/system/permissions/audit-logs     操作审计日志
//   - PUT /api/system/permissions/:id/enabled    启停账号
//   - PUT /api/system/permissions/:id/password   重置密码
//
// 受 RequireAdminMiddleware 保护（在路由组上声明，子路由全部继承）。

// setupPermissionRoutes 注册 /api/system/permissions/* 路由
func setupPermissionRoutes(auth *gin.RouterGroup) {
	ctrl := controller.NewPermissionController()
	admin := auth.Group("/system/permissions", middleware.RequireAdminMiddleware())
	{
		admin.GET("/audit-logs", ctrl.ListAuditLogs)
		admin.PUT("/:id/enabled", ctrl.SetEnabled)
		admin.PUT("/:id/password", ctrl.ResetPassword)
	}
}
