package router

import (
	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"

	"github.com/gin-gonic/gin"
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

