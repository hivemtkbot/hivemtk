package router

import (
	"hivemtk-user/internal/controller"

	"github.com/gin-gonic/gin"
)

func setupCompetitorFeatureRoutes(auth *gin.RouterGroup) {

	coPilotCtrl := controller.NewAgentCoPilotController()
	auth.GET("/agent/co-pilot/config", coPilotCtrl.GetConfig)
	auth.PUT("/agent/co-pilot/config", coPilotCtrl.SaveConfig)
	auth.POST("/agent/co-pilot/evaluate", coPilotCtrl.Evaluate)

	smartRouterCtrl := controller.NewSmartRouterController()
	auth.POST("/smart-router/select", smartRouterCtrl.SelectAgent)

	ragEvalCtrl := controller.NewRagEvalController()
	auth.POST("/rag-eval/run", ragEvalCtrl.Run)
	auth.GET("/rag-eval/runs", ragEvalCtrl.List)
	auth.GET("/rag-eval/runs/:id", ragEvalCtrl.Get)

	agentAttrCtrl := controller.NewAgentAttributionController()
	auth.GET("/agent-attribution/performance", agentAttrCtrl.GetPerformance)

	dataExportCtrl := controller.NewDataExportController()
	auth.GET("/gdpr/export/:customer_id", dataExportCtrl.Export)
}
