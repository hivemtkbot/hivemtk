package controller

import (
	"net/http"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// 工具集成配置控制器
//
// 读写客服 Agent 工具依赖的外部集成配置（实时快递轨迹、售后回写电商）。
// 配置统一存数据库 system_config_kv[agent.tool_integrations]（而非环境变量），
// 通过本接口可视化编辑；保存后立即对新工具请求生效，无需重启。
// ============================================================================

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
