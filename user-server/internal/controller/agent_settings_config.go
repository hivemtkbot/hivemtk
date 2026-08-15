package controller

import (
	"net/http"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)


// AgentSettingsController Agent Loop 运行期调参控制器
type AgentSettingsController struct{}

// NewAgentSettingsController 构造
func NewAgentSettingsController() *AgentSettingsController {
	return &AgentSettingsController{}
}

// GetConfig 读取 Agent Loop 运行期调参（GET /api/agent/settings）
func (c *AgentSettingsController) GetConfig(ctx *gin.Context) {
	cfg, err := service.LoadAgentSettingsConfig(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

// SaveConfig 保存 Agent Loop 运行期调参（PUT /api/agent/settings）
func (c *AgentSettingsController) SaveConfig(ctx *gin.Context) {
	var cfg service.AgentSettingsConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "请求体格式错误: " + err.Error()})
		return
	}
	if cfg.MaxLoopIterations != 0 && cfg.MaxLoopIterations < 2 {
		ctx.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "max_loop_iterations 必须 >= 2（否则工具调用无法产出答案）"})
		return
	}
	if err := service.SaveAgentSettingsConfig(ctx.Request.Context(), &cfg); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"success": true, "data": cfg})
}

