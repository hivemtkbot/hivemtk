package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// ReachPipelineController 触达 Pipeline 控制器

// reachHTTPStatus 将触达 service 错误映射到合适的 HTTP 状态码
//
// 遵循项目约定（response.ErrorFromDB 语义）：not found -> 404，可预期校验错误 -> 400，
// 其余（数据库 / 未知错误）-> 500。避免把校验错误一律按 400、也不把 not found 误判为 500。
func reachHTTPStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrReachPipelineNotFound),
		errors.Is(err, service.ErrReachJobNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrReachInvalidChannel),
		errors.Is(err, service.ErrReachInvalidSteps),
		errors.Is(err, service.ErrReachInvalidPayload),
		errors.Is(err, service.ErrReachJobNotPending),
		errors.Is(err, service.ErrReachRateLimited):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
type ReachPipelineController struct {
	svc *service.ReachPipelineService
}

// NewReachPipelineController 创建触达 Pipeline 控制器
func NewReachPipelineController(svc *service.ReachPipelineService) *ReachPipelineController {
	return &ReachPipelineController{svc: svc}
}

// CreatePipeline 创建 Pipeline
func (c *ReachPipelineController) CreatePipeline(ctx *gin.Context) {
	var req service.CreatePipelineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if req.RetryPolicy.MaxRetries == 0 {
		req.RetryPolicy = service.DefaultRetryPolicy()
	}
	if req.RateLimit.QPS == 0 {
		req.RateLimit = service.DefaultRateLimit()
	}
	pipe, err := c.svc.CreatePipeline(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, pipe, "创建成功")
}

// UpdatePipeline 更新
func (c *ReachPipelineController) UpdatePipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	var req service.CreatePipelineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	pipe, err := c.svc.UpdatePipeline(ctx.Request.Context(), uint(id), &req)
	if err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, pipe, "更新成功")
}

// GetPipeline 详情
func (c *ReachPipelineController) GetPipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	pipe, err := c.svc.GetPipeline(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "Pipeline 不存在")
		return
	}
	response.Success(ctx, pipe, "查询成功")
}

// ListPipelines 列表
func (c *ReachPipelineController) ListPipelines(ctx *gin.Context) {
	channel := ctx.Query("channel")
	status := ctx.Query("status")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc.ListPipelines(ctx.Request.Context(), channel, status, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// DeletePipeline 删除
func (c *ReachPipelineController) DeletePipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.DeletePipeline(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "删除成功")
}

// PausePipeline 暂停
func (c *ReachPipelineController) PausePipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.PausePipeline(ctx.Request.Context(), uint(id)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "暂停成功")
}

// ResumePipeline 恢复
func (c *ReachPipelineController) ResumePipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.ResumePipeline(ctx.Request.Context(), uint(id)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "恢复成功")
}

// ArchivePipeline 归档
func (c *ReachPipelineController) ArchivePipeline(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.ArchivePipeline(ctx.Request.Context(), uint(id)); err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "归档成功")
}

// EnqueueJob 入队任务
func (c *ReachPipelineController) EnqueueJob(ctx *gin.Context) {
	var req service.EnqueueJobRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if req.RunAt == nil {
		now := time.Now()
		req.RunAt = &now
	}
	job, err := c.svc.EnqueueJob(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, job, "入队成功")
}

// GetJob 任务详情
func (c *ReachPipelineController) GetJob(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	job, err := c.svc.GetJob(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "任务不存在")
		return
	}
	response.Success(ctx, job, "查询成功")
}

// ListJobs 任务列表
func (c *ReachPipelineController) ListJobs(ctx *gin.Context) {
	channel := ctx.Query("channel")
	state := ctx.Query("state")
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	list, total, err := c.svc.ListJobs(ctx.Request.Context(), channel, state, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// CancelJob 取消任务
func (c *ReachPipelineController) CancelJob(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.CancelJob(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "取消成功")
}

// RetryJob 重试任务
func (c *ReachPipelineController) RetryJob(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.RetryJob(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "重试成功")
}

// ExecuteJob 立即执行
func (c *ReachPipelineController) ExecuteJob(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	job, err := c.svc.ExecuteJob(ctx.Request.Context(), uint(id))
	if err != nil {
		// rate_limited 仍返回结果
		if err == service.ErrReachRateLimited && job != nil {
			response.Success(ctx, job, "任务被限流")
			return
		}
		response.Error(ctx, reachHTTPStatus(err), err.Error())
		return
	}
	response.Success(ctx, job, "执行成功")
}

// Stats 统计
func (c *ReachPipelineController) Stats(ctx *gin.Context) {
	stats, err := c.svc.Stats(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, stats, "查询成功")
}

// ResetRateLimit 重置限流
func (c *ReachPipelineController) ResetRateLimit(ctx *gin.Context) {
	channel := ctx.Query("channel")
	if channel == "" {
		response.Error(ctx, http.StatusBadRequest, "channel 必填")
		return
	}
	c.svc.ResetRateLimit(context.Background(), channel)
	response.Success(ctx, gin.H{"channel": channel}, "重置成功")
}
