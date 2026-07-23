package controller

import (
	"context"
	"net/http"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// LLMRoutingController LLM 多模型路由控制器
type LLMRoutingController struct {
	svc *service.LLMRoutingService
}

// NewLLMRoutingController 创建 LLM 路由控制器
func NewLLMRoutingController() *LLMRoutingController {
	return &LLMRoutingController{svc: service.NewLLMRoutingService()}
}

// ListModels 模型列表
func (c *LLMRoutingController) ListModels(ctx *gin.Context) {
	list := c.svc.ListModels(context.Background(), )
	response.SuccessWithList(ctx, list, int64(len(list)))
}

// CreateModel 添加模型
func (c *LLMRoutingController) CreateModel(ctx *gin.Context) {
	var req service.CreateModelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	model, err := c.svc.AddModel(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, model, "添加成功")
}

// UpdateModel 更新模型
func (c *LLMRoutingController) UpdateModel(ctx *gin.Context) {
	name := ctx.Param("id")
	var req service.CreateModelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	model, err := c.svc.UpdateModel(context.Background(), name, &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, model, "更新成功")
}

// DeleteModel 删除模型
func (c *LLMRoutingController) DeleteModel(ctx *gin.Context) {
	name := ctx.Param("id")
	if err := c.svc.DeleteModel(context.Background(), name); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"name": name}, "删除成功")
}

// TestModel 测试模型
func (c *LLMRoutingController) TestModel(ctx *gin.Context) {
	name := ctx.Param("id")
	var req service.TestModelRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	result, err := c.svc.TestModel(ctx.Request.Context(), name, &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, result, "测试完成")
}

// ListStrategies 路由策略列表
func (c *LLMRoutingController) ListStrategies(ctx *gin.Context) {
	list := c.svc.ListStrategies(context.Background(), )
	response.SuccessWithList(ctx, list, int64(len(list)))
}

// UpdateStrategies 更新路由策略
func (c *LLMRoutingController) UpdateStrategies(ctx *gin.Context) {
	var req service.UpdateStrategiesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	list, err := c.svc.UpdateStrategies(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"routes": list}, "更新成功")
}

// Stats 调用统计
func (c *LLMRoutingController) Stats(ctx *gin.Context) {
	stats := c.svc.GetStats(context.Background(), )
	response.SuccessWithList(ctx, stats, int64(len(stats)))
}

// Usage 用量查询
func (c *LLMRoutingController) Usage(ctx *gin.Context) {
	summary := c.svc.GetUsage(context.Background(), )
	response.Success(ctx, summary, "查询成功")
}
