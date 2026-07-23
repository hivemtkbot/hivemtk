package controller

import (
	"context"
	"marketing/internal/cache"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// MessageHubController 消息中台控制器
type MessageHubController struct {
	svc *service.MessageHubService
}

// NewMessageHubController 创建消息中台控制器
func NewMessageHubController(svc *service.MessageHubService) *MessageHubController {
	return &MessageHubController{
		svc: svc,
	}
}

// NewMessageHubControllerWithCache 带缓存创建
// Deprecated: 请使用 NewMessageHubController，router 层负责组装带缓存的 service
func NewMessageHubControllerWithCache(svc *service.MessageHubService, _ cache.Cache) *MessageHubController {
	return &MessageHubController{
		svc: svc,
	}
}

// Push 推送消息
func (c *MessageHubController) Push(ctx *gin.Context) {
	var req service.PushMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	msg, err := c.svc.Push(ctx.Request.Context(), &req)
	if err != nil {
		if err == service.ErrMessageHubIdempotent {
			response.Success(ctx, gin.H{"duplicate": true}, "消息已存在(幂等)")
			return
		}
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, msg, "推送成功")
}

// PushBatch 批量推送
func (c *MessageHubController) PushBatch(ctx *gin.Context) {
	var reqs []service.PushMessageRequest
	if err := ctx.ShouldBindJSON(&reqs); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	results, errs := c.svc.PushBatch(ctx.Request.Context(), reqs)
	resp := gin.H{
		"success": 0,
		"failed":  0,
		"items":   results,
		"errors":  errs,
	}
	for _, e := range errs {
		if e == nil {
			resp["success"] = resp["success"].(int) + 1
		}
	}
	response.Success(ctx, resp, "批量推送完成")
}

// List 列表
func (c *MessageHubController) List(ctx *gin.Context) {
	_ = ctx
	q := service.ListQuery{

		Platform:       ctx.Query("platform"),
		AccountID:      ctx.Query("account_id"),
		ConversationID: ctx.Query("conversation_id"),
		SenderID:       ctx.Query("sender_id"),
		Direction:      ctx.Query("direction"),
		MsgType:        ctx.Query("msg_type"),
		Keyword:        ctx.Query("keyword"),
		OrderBy:        ctx.Query("order_by"),
	}
	if v := ctx.Query("is_read"); v != "" {
		b := v == "true" || v == "1"
		q.IsRead = &b
	}
	if v := ctx.Query("is_group"); v != "" {
		b := v == "true" || v == "1"
		q.IsGroup = &b
	}
	if v := ctx.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.StartTime = &t
		}
	}
	if v := ctx.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			q.EndTime = &t
		}
	}
	{
		p, ps, err := pagination.Parse(ctx)
		if err != nil {
			response.Error(ctx, http.StatusBadRequest, err.Error())
			return
		}
		q.Page = p
		q.PageSize = ps
	}
	list, total, err := c.svc.List(ctx.Request.Context(), q)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(q.Page), int64(q.PageSize), total)
}

// GetByID 详情
func (c *MessageHubController) GetByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的ID")
		return
	}
	msg, err := c.svc.GetByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if msg == nil {
		response.NotFound(ctx, "消息不存在")
		return
	}
	response.Success(ctx, msg, "获取成功")
}

// MarkRead 标记已读
func (c *MessageHubController) MarkRead(ctx *gin.Context) {
	var req struct {
		IDs []uint `json:"ids" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if err := c.svc.MarkRead(ctx.Request.Context(), req.IDs); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"updated": len(req.IDs)}, "标记已读成功")
}

// Stats 统计
func (c *MessageHubController) Stats(ctx *gin.Context) {
	var start, end *time.Time
	if v := ctx.Query("start_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			start = &t
		}
	}
	if v := ctx.Query("end_time"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			end = &t
		}
	}
	stats, err := c.svc.GetStats(ctx.Request.Context(), start, end)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "统计成功")
}

// Platforms 列出支持平台
func (c *MessageHubController) Platforms(ctx *gin.Context) {
	response.Success(ctx, gin.H{
		"platforms": service.ListPlatforms(),
		"msg_types": service.ListMsgTypes(),
	}, "获取成功")
}

// PushFromChannel 从渠道原始消息推送
func (c *MessageHubController) PushFromChannel(ctx *gin.Context) {
	var raw service.RawChannelMessage
	if err := ctx.ShouldBindJSON(&raw); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	req := c.svc.ConvertFromChannel(context.Background(), &raw)
	msg, err := c.svc.Push(ctx.Request.Context(), req)
	if err != nil {
		if err == service.ErrMessageHubIdempotent {
			response.Success(ctx, gin.H{"duplicate": true}, "消息已存在(幂等)")
			return
		}
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, msg, "推送成功")
}
