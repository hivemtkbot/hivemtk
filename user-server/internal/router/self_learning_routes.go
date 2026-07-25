package router

// self_learning_routes.go 对话驱动自我学习三位一体机制路由装配
//
// 五层架构归属: L1 网关层
// 设计依据: docs/企业级架构优化/对话驱动自我学习机制.md (v1.1) §7.4
//
// 职责：
//   1. 装配 SelfLearningService（依赖 SelfLearningComponents）
//   2. 装配 SelfLearningController
//   3. 注册路由到 /api/self-learning/*
//
// 装配链路：
//   main.go → InitSelfLearningComponents() → SelfLearningComponents
//            → SetGlobalSelfLearningComponents() （全局单例）
//   router.Setup → setupSelfLearningRoutes(auth) → 读取全局单例
//                → NewSelfLearningService(components, db)
//                → NewSelfLearningController(svc)
//                → ctrl.Register(auth)

import (
	"github.com/gin-gonic/gin"

	"marketing/internal/controller"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"
)

// 全局单例（由 main.go 启动阶段调用 SetGlobalSelfLearningComponents 注入）
var globalSelfLearningComponents *service.SelfLearningComponents

// SetGlobalSelfLearningComponents 注册全局自我学习组件（main.go 启动时调用一次）
func SetGlobalSelfLearningComponents(c *service.SelfLearningComponents) {
	globalSelfLearningComponents = c
}

// GetGlobalSelfLearningComponents 获取全局自我学习组件
func GetGlobalSelfLearningComponents() *service.SelfLearningComponents {
	return globalSelfLearningComponents
}

// setupSelfLearningRoutes 自我学习三位一体机制路由注册
//
// 路由前缀：/api/self-learning/*（兼容 /api/v1/self-learning/*）
func setupSelfLearningRoutes(auth *gin.RouterGroup) {
	components := GetGlobalSelfLearningComponents()
	if components == nil {
		// 未初始化时跳过路由注册（不阻塞其他服务启动）
		return
	}
	// 构造 L3 门面服务（注入查询仓储）
	svc := service.NewSelfLearningService(components, db.GetDB())
	ctrl := controller.NewSelfLearningController(svc)

	// 兼容 /api/v1 与 /api 两种前缀
	for _, g := range []*gin.RouterGroup{
		auth.Group("/v1"),
		auth,
	} {
		ctrl.Register(g)
	}
}
