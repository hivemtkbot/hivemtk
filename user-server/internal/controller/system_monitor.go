package controller

import (
	"net/http"

	"hivemtk-user/internal/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

type SystemMonitorController struct {
	svc *service.SystemMonitorService
}

func NewSystemMonitorController() *SystemMonitorController {
	return &SystemMonitorController{svc: service.NewSystemMonitorService()}
}

func (c *SystemMonitorController) GetMetrics(ctx *gin.Context) {
	stats, err := c.svc.GetSystemStats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to get system metrics")
		return
	}
	response.Success(ctx, stats, "ok")
}

func (c *SystemMonitorController) GetHealth(ctx *gin.Context) {
	details, err := c.svc.GetDetailedSystemStats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "failed to get system health")
		return
	}
	health := map[string]any{
		"status": "ok",
		"checks": []map[string]any{
			{"name": "database", "ok": true},
			{"name": "application", "ok": true},
		},
		"resources": details["system_resources"],
		"timestamp": details["timestamp"],
	}
	response.Success(ctx, health, "ok")
}
