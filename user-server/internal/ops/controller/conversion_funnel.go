// 独立部署版本：单租户，Controller 仅做参数解析与响应包装
package controller

import (
	"net/http"
	"time"

	"hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// ConversionFunnelController 转化漏斗控制器
type ConversionFunnelController struct {
	svc *service.ConversionFunnelService
}

// NewConversionFunnelController 创建控制器
func NewConversionFunnelController() *ConversionFunnelController {
	return &ConversionFunnelController{svc: service.NewConversionFunnelService()}
}

// GetFunnel 获取漏斗
func (c *ConversionFunnelController) GetFunnel(ctx *gin.Context) {
	start, end := parseTimeRange(ctx)
	report, err := c.svc.BuildFunnel(start, end)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "漏斗生成失败: "+err.Error())
		return
	}
	response.Success(ctx, report, "漏斗生成成功")
}

// GetStageDetails 阶段详情
func (c *ConversionFunnelController) GetStageDetails(ctx *gin.Context) {
	stage := ctx.Query("stage")
	start, end := parseTimeRange(ctx)
	det, err := c.svc.GetStageDetails(stage, start, end)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "阶段详情失败: "+err.Error())
		return
	}
	response.Success(ctx, det, "查询成功")
}

// parseTimeRange 公共时间范围解析
func parseTimeRange(ctx *gin.Context) (time.Time, time.Time) {
	var start, end time.Time
	if v := ctx.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = t
		}
	}
	if v := ctx.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = t
		}
	}
	return start, end
}

