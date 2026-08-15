package controller

import (
	"net/http"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)


// ToolIntegrationConfigController 工具集成配置控制器
type ToolIntegrationConfigController struct{}

// NewToolIntegrationConfigController 构造
func NewToolIntegrationConfigController() *ToolIntegrationConfigController {
	return &ToolIntegrationConfigController{}
}

// GetConfig 读取工具集成配置（GET /api/agent/tool-integrations）
func (c *ToolIntegrationConfigController) GetConfig(ctx *gin.Context) {
	cfg, err := service.LoadToolIntegrationConfig(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// SaveConfig 保存工具集成配置（PUT /api/agent/tool-integrations）
func (c *ToolIntegrationConfigController) SaveConfig(ctx *gin.Context) {
	var cfg service.ToolIntegrationConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求体格式错误: " + err.Error()})
		return
	}
	if err := service.SaveToolIntegrationConfig(ctx.Request.Context(), &cfg); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

