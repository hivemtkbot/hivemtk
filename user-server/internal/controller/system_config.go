package controller

import (
	"marketing/internal/model"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SystemConfigController 系统配置控制器
type SystemConfigController struct {
	svc *service.SystemConfigService
}

// NewSystemConfigController 创建系统配置控制器实例
func NewSystemConfigController() *SystemConfigController {
	return &SystemConfigController{svc: service.NewSystemConfigService()}
}

// GetConfig 获取系统配置
func (c *SystemConfigController) GetConfig(ctx *gin.Context) {
	config, err := c.svc.GetConfig()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, config, "success")
}

// SaveConfig 保存系统配置
func (c *SystemConfigController) SaveConfig(ctx *gin.Context) {
	var req *model.SystemConfig
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	config, err := c.svc.SaveConfig(req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, config, "success")
}

// ResetSystem 重置系统
func (c *SystemConfigController) ResetSystem(ctx *gin.Context) {
	err := c.svc.ResetSystem()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, nil, "系统重置成功")
}
