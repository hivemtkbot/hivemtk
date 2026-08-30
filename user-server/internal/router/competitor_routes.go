package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
)

// setupCompetitorFeatureRoutes 注册 6 个竞品标配功能的路由端点
//
// G1  Agent Co-Pilot 自动执行
// G2  智能路由 & 技能匹配
// G3  会话锁定/编辑冲突检测（乐观锁）—— 纯 Model + Repo 变更，无独立端点
// G4  RAG 自动评估 Pipeline
// G5  Agent 绩效归因
// G6  GDPR DSAR 数据导出
func setupCompetitorFeatureRoutes(auth *gin.RouterGroup) {
	// === G1: Agent Co-Pilot 自动执行 ===
	coPilotCtrl := controller.NewAgentCoPilotController()
	auth.GET("/agent/co-pilot/config", coPilotCtrl.GetConfig)
	auth.PUT("/agent/co-pilot/config", coPilotCtrl.SaveConfig)
	auth.POST("/agent/co-pilot/evaluate", coPilotCtrl.Evaluate)

	// === G2: 智能路由 & 技能匹配 ===
	smartRouterCtrl := controller.NewSmartRouterController()
	auth.POST("/smart-router/select", smartRouterCtrl.SelectAgent)

	// === G4: RAG 自动评估 Pipeline ===
	ragEvalCtrl := controller.NewRagEvalController()
	auth.POST("/rag-eval/run", ragEvalCtrl.Run)
	auth.GET("/rag-eval/runs", ragEvalCtrl.List)
	auth.GET("/rag-eval/runs/:id", ragEvalCtrl.Get)

	// === G5: Agent 绩效归因 ===
	agentAttrCtrl := controller.NewAgentAttributionController()
	auth.GET("/agent-attribution/performance", agentAttrCtrl.GetPerformance)

	// === G6: GDPR DSAR 数据导出 ===
	dataExportCtrl := controller.NewDataExportController()
	auth.GET("/gdpr/export/:customer_id", dataExportCtrl.Export)
}
