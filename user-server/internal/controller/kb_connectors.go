// kb_connectors.go 知识连接器控制器（R40）
package controller

import (
	"fmt"
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

// Pull POST /api/knowledge/connectors/:source/pull {product_id, query, max_pages}
//
// R42: 一键拉取导入（notion 完整实现；其余源返回明确 not_implemented 契约）。
func (c *KBConnectorController) Pull(ctx *gin.Context) {
	// 注意: Gin body 只能读一次 → 单结构体合并绑定(product_id + 拉取参数)
	var body struct {
		service.ConnectorPullRequest
		ProductID string `json:"product_id"`
	}
	_ = ctx.ShouldBindJSON(&body)
	productID := body.ProductID
	if productID == "" {
		productID = ctx.Query("product_id")
	}
	res, err := c.svc.Pull(ctx.Request.Context(), ctx.Param("source"), productID, &body.ConnectorPullRequest)
	if HandleServiceError(ctx, err) {
		return
	}
	response.Success(ctx, res, fmt.Sprintf("拉取完成: 成功 %d / 失败 %d / 跳过 %d", res.Imported, res.Failed, res.Skipped))
}
