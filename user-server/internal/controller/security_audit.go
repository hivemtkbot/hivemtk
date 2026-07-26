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

// SecurityAuditController 安全审计控制器
type SecurityAuditController struct {
	svc *service.SecurityAuditService
}

// NewSecurityAuditController 创建控制器
func NewSecurityAuditController() *SecurityAuditController {
	return &SecurityAuditController{svc: service.NewSecurityAuditService()}
}

// RunAudit 启动审计
func (c *SecurityAuditController) RunAudit(ctx *gin.Context) {
	var req service.AuditRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// 允许空 body
		req.AuditName = "default_audit"
	}
	record, err := c.svc.RunAudit(ctx.Request.Context(), req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "审计失败: "+err.Error())
		return
	}
	response.Success(ctx, record, "审计完成")
}

// GetResult 审计结果
func (c *SecurityAuditController) GetResult(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	r, err := c.svc.GetResult(context.Background(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, r, "查询成功")
}

// ListResults 审计历史
func (c *SecurityAuditController) ListResults(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc.ListResults(context.Background(), page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}
