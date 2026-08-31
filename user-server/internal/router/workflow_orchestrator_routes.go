package router

import (
	"context"

	"hivemtk-user/internal/controller"
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/repository"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupWorkflowOrchestratorRoutes 工作流编排路由
func setupWorkflowOrchestratorRoutes(auth *gin.RouterGroup, db *gorm.DB) {
	// 初始化仓库
	versionRepo := repository.NewWorkflowVersionRepository(db)
	execRepo := repository.NewWorkflowExecutionRepository(db)
	nodeExecRepo := repository.NewWorkflowNodeExecutionRepository(db)

	// 初始化服务
	svc := service.NewWorkflowOrchestratorService(versionRepo, execRepo, nodeExecRepo)

	// 初始化节点执行器注册中心 + 调度器，并装配到 service（异步驱动节点执行）
	registry := service.NewWorkflowNodeExecutorRegistry()
	service.RegisterWorkflowNodeExecutors(registry)
	dispatcher := service.NewWorkflowDispatcher(versionRepo, execRepo, nodeExecRepo, registry, nil)
	dispatcher.Start(context.Background())
	svc.SetDispatcher(dispatcher)

	// 初始化控制器
	workflowCtrl := controller.NewWorkflowOrchestratorController(svc)

	// 版本相关路由（写入操作需要管理员权限）
	auth.GET("/workflows/versions", workflowCtrl.ListVersions)
	auth.GET("/workflows/versions/:id", workflowCtrl.GetVersion)

	adminGroup := auth.Group("/workflows/versions", middleware.AdminAuthMiddleware())
	{
		adminGroup.POST("", workflowCtrl.CreateVersion)
		adminGroup.PUT("/:id", workflowCtrl.UpdateVersion)
		adminGroup.DELETE("/:id", workflowCtrl.DeleteVersion)
		adminGroup.POST("/:id/publish", workflowCtrl.PublishVersion)
		adminGroup.POST("/:id/archive", workflowCtrl.ArchiveVersion)
	}

	// 执行相关路由
	//
	// 安全（2026-08-31 P0-25 四轮加固）：POST /workflows/execute 触发工作流执行，
	// 涉及调度器 / 节点执行器 / 可能的外部 API 调用，资源密集型操作，收敛为 admin only。
	// 只读查询（ListExecutions / GetExecution / GetNodeExecutions）保留任意登录。
	auth.GET("/workflows/executions", workflowCtrl.ListExecutions)
	auth.GET("/workflows/executions/:id", workflowCtrl.GetExecution)
	auth.GET("/workflows/executions/:id/nodes", workflowCtrl.GetNodeExecutions)

	execAdmin := auth.Group("", middleware.AdminAuthMiddleware())
	execAdmin.POST("/workflows/execute", workflowCtrl.Execute)
	execAdmin.POST("/workflows/executions/:id/stop", workflowCtrl.StopExecution)
}