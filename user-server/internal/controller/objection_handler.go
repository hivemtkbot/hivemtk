// 独立部署版本：单租户，Controller 仅做参数解析与响应包装
package controller

import (
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// ObjectionHandlerController 异议处理控制器
type ObjectionHandlerController struct {
	svc *service.ObjectionHandlerService
}

// NewObjectionHandlerController 创建控制器
func NewObjectionHandlerController() *ObjectionHandlerController {
	return &ObjectionHandlerController{svc: service.NewObjectionHandlerService()}
}

// Handle 处理异议
func (c *ObjectionHandlerController) Handle(ctx *gin.Context) {
	var req service.HandleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	resp, err := c.svc.Handle(ctx.Request.Context(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "处理失败: "+err.Error())
		return
	}
	response.Success(ctx, resp, "处理成功")
}

// Classify 分类
func (c *ObjectionHandlerController) Classify(ctx *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	cat, name := c.svc.Classify(ctx.Request.Context(), req.Text)
	response.Success(ctx, gin.H{
		"category":      cat,
		"category_name": name,
	}, "分类成功")
}

// ListCategories 列出类别
func (c *ObjectionHandlerController) ListCategories(ctx *gin.Context) {
	cats := c.svc.ListCategories(ctx.Request.Context())
	response.SuccessWithList(ctx, cats, int64(len(cats)))
}

// RecordUsage 记录使用（需登录，与同目录其他 controller 鉴权惯例对齐）
func (c *ObjectionHandlerController) RecordUsage(ctx *gin.Context) {
	if _, ok := extractActorID(ctx); !ok {
		return
	}
	var req struct {
		TemplateID uint `json:"template_id"`
		Success    bool `json:"success"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := c.svc.RecordUsage(ctx.Request.Context(), req.TemplateID, req.Success); err != nil {
		response.ErrorFromDB(ctx, err, "记录失败: "+err.Error())
		return
	}
	response.Success(ctx, nil, "记录成功")
}
