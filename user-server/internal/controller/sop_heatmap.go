package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// SopHeatmapController SOP 节点级转化热力图控制器
type SopHeatmapController struct {
	svc *service.SopHeatmapService
}

// NewSopHeatmapController 创建热力图控制器
func NewSopHeatmapController(svc *service.SopHeatmapService) *SopHeatmapController {
	return &SopHeatmapController{svc: svc}
}

// GetHeatmap godoc
// @Summary      SOP 节点转化热力图
// @Description  返回指定 SOP 的每个节点 entered/completed/drop_rate/avg_duration，
//
//	支持 variant 筛选（A/B 测试），limit 控制拉取执行数上限
//
// @Tags         SOP
// @Produce      json
// @Security     BearerAuth
// @Param        id      path   int    true  "SOP ID"
// @Param        variant query  string false "Variant 名称筛选（A/B/...）"
// @Param        limit   query  int    false "拉取执行数上限（默认 200，最大 1000）"
// @Success      200     {object}  response.Response{data=service.SopHeatmapReport}
// @Failure      400     {object}  response.Response
// @Router       /api/sop/{id}/heatmap [get]
func (c *SopHeatmapController) GetHeatmap(ctx *gin.Context) {
	sopID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	variant := ctx.Query("variant")
	limit := 200
	if l, err := strconv.Atoi(ctx.Query("limit")); err == nil && l > 0 && l <= 1000 {
		limit = l
	}

	rpt, err := c.svc.GenerateHeatmapForSOP(ctx.Request.Context(), uint(sopID), variant, limit)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, rpt, "查询成功")
}
