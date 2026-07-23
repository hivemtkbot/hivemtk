package router

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

import (
	"github.com/gin-gonic/gin"

	"marketing/internal/controller"
	"marketing/internal/middleware"
)

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
