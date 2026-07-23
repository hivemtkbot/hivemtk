// 独立部署版本：单租户，Controller 仅做参数解析与响应包装
package controller

import (
	"context"
	"net/http"
	"strconv"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// SalesPersonaController 销冠能力画像控制器
type SalesPersonaController struct {
	svc *service.SalesPersonaService
}

// NewSalesPersonaController 创建控制器
func NewSalesPersonaController() *SalesPersonaController {
	return &SalesPersonaController{svc: service.NewSalesPersonaService()}
}

// GetReport 获取员工能力画像
func (c *SalesPersonaController) GetReport(ctx *gin.Context) {
	staffIDStr := ctx.Param("id")
	staffID, err := strconv.ParseUint(staffIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的员工ID")
		return
	}
	rep, err := c.svc.BuildReport(context.Background(), uint(staffID))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "报告生成失败: "+err.Error())
		return
	}
	response.Success(ctx, rep, "报告生成成功")
}

// ListStaffs 列出员工
func (c *SalesPersonaController) ListStaffs(ctx *gin.Context) {
	staffs, err := c.svc.ListStaffs(context.Background(), )
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithList(ctx, staffs, int64(len(staffs)))
}
