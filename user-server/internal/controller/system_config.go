package controller

import (
	"context"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
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
	config, err := c.svc.GetConfig(context.Background())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
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
	config, err := c.svc.SaveConfig(context.Background(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, config, "success")
}
