package controller

import (
	"marketing/internal/pkg/shared/service"
	"net/http"

	"marketing/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

type SystemInfoController struct {
	service *service.SystemStatsService
}

func NewSystemInfoController() *SystemInfoController {
	return &SystemInfoController{
		service: service.NewSystemStatsService(),
	}
}

// GetSystemInfo 获取系统信息
// @Summary 获取系统信息
// @Description 获取系统基本信息，包括版本、运行环境等
// @Tags 系统
// @Produce json
// @Success 200 {object} object{data=service.SystemInfo} "获取成功"
// @Router /api/system/info [get]
func (c *SystemInfoController) GetSystemInfo(ctx *gin.Context) {
	info, err := c.service.GetSystemInfo()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取系统信息失败")
		return
	}

	response.Success(ctx, info, "获取系统信息成功")
}
