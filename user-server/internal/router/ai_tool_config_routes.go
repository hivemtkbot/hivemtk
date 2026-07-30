package router

import (
	"marketing/internal/controller"
	"marketing/internal/repository"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAIToolConfigRoutes 注册AI工具配置路由
func setupAIToolConfigRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 创建依赖
	toolRepo := repository.NewAIToolConfigRepository(db)
	bindingRepo := repository.NewAIToolAccountBindingRepository(db)
	svc := service.NewAIToolConfigService(toolRepo, bindingRepo)
	ctrl := controller.NewAIToolConfigController(svc)

	// 注册路由
	g := auth.Group("/ai-tools")
	{
		// 工具配置
		g.GET("", ctrl.ListTools)
		g.GET("/:name", ctrl.GetTool)
		g.PUT("/:name/status", ctrl.UpdateToolStatus)
		g.POST("/batch-status", ctrl.BatchUpdateStatus)

		// 工具-账号绑定
		g.GET("/:name/accounts", ctrl.GetToolAccounts)
		g.POST("/:name/accounts", ctrl.BindAccount)
		g.DELETE("/:name/accounts/:account_type/:account_id", ctrl.UnbindAccount)
	}
}