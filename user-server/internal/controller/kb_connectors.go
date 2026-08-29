// kb_connectors.go 知识连接器控制器（R40）
package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// KBConnectorController 连接器控制器
type KBConnectorController struct {
	svc *service.KBConnectorService
}

// NewKBConnectorController 构造
func NewKBConnectorController() *KBConnectorController {
	return &KBConnectorController{svc: service.NewKBConnectorService()}
}

// List GET /api/knowledge/connectors
func (c *KBConnectorController) List(ctx *gin.Context) {
	list := c.svc.List(ctx.Request.Context())
	response.Success(ctx, gin.H{"list": list, "total": len(list)}, "ok")
}

// Get GET /api/knowledge/connectors/:source
func (c *KBConnectorController) Get(ctx *gin.Context) {
	response.Success(ctx, c.svc.Get(ctx.Request.Context(), ctx.Param("source")), "ok")
}

// Save PUT /api/knowledge/connectors/:source {enabled, config}
func (c *KBConnectorController) Save(ctx *gin.Context) {
	var req service.SaveConnectorRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}
	if err := c.svc.Save(ctx.Request.Context(), ctx.Param("source"), &req); HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, c.svc.Get(ctx.Request.Context(), ctx.Param("source")), "凭据已保存（读取侧已脱敏）")
}

// Test POST /api/knowledge/connectors/:source/test
func (c *KBConnectorController) Test(ctx *gin.Context) {
	res, err := c.svc.Test(ctx.Request.Context(), ctx.Param("source"))
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, res.Message)
}
