package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// SOPController SOP 智能体控制器
type SOPController struct {
	svc *service.SOPService
}

// NewSOPController 创建 SOP 控制器
func NewSOPController(svc *service.SOPService) *SOPController {
	return &SOPController{svc: svc}
}

// Create 创建 SOP
func (c *SOPController) Create(ctx *gin.Context) {
	var req service.CreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	agent, err := c.svc.Create(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, agent, "创建成功")
}

// Update 更新 SOP
func (c *SOPController) Update(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	var req service.CreateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agent, err := c.svc.Update(ctx.Request.Context(), uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, agent, "更新成功")
}

// Get 详情
func (c *SOPController) Get(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	agent, err := c.svc.Get(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "SOP 不存在")
		return
	}
	response.Success(ctx, agent, "查询成功")
}

// List 列表
func (c *SOPController) List(ctx *gin.Context) {
	scenario := ctx.Query("scenario")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	list, total, err := c.svc.List(ctx.Request.Context(), scenario, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// Delete 删除
func (c *SOPController) Delete(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	if err := c.svc.Delete(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "删除成功")
}

// Activate 启用
func (c *SOPController) Activate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	if err := c.svc.Activate(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "启用成功")
}

// Deactivate 停用
func (c *SOPController) Deactivate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	if err := c.svc.Deactivate(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "停用成功")
}

// Execute 启动执行
func (c *SOPController) Execute(ctx *gin.Context) {
	var req dto.ExecuteRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	exec, err := c.svc.Execute(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, exec, "执行成功")
}

// Step 单步推进
func (c *SOPController) Step(ctx *gin.Context) {
	var req dto.StepRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	exec, err := c.svc.Step(ctx.Request.Context(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, exec, "推进成功")
}

// Pause 暂停
func (c *SOPController) Pause(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.Pause(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "暂停成功")
}

// Resume 恢复
func (c *SOPController) Resume(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.Resume(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "恢复成功")
}

// Cancel 取消
func (c *SOPController) Cancel(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	if err := c.svc.Cancel(ctx.Request.Context(), uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"id": id}, "取消成功")
}

// GetExecution 详情
func (c *SOPController) GetExecution(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 ID")
		return
	}
	exec, err := c.svc.GetExecution(ctx.Request.Context(), uint(id))
	if err != nil {
		response.NotFound(ctx, "执行不存在")
		return
	}
	response.Success(ctx, exec, "查询成功")
}

// ListExecutions 列表
func (c *SOPController) ListExecutions(ctx *gin.Context) {
	customerID := ctx.Query("customer_id")
	status := ctx.Query("status")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	list, total, err := c.svc.ListExecutions(ctx.Request.Context(), customerID, status, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// MatchByIntent 意图匹配
func (c *SOPController) MatchByIntent(ctx *gin.Context) {
	intent := ctx.Query("intent")
	list, err := c.svc.MatchByIntent(ctx.Request.Context(), intent)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// Stats 统计
func (c *SOPController) Stats(ctx *gin.Context) {
	stats, err := c.svc.Stats(ctx.Request.Context())
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, stats, "查询成功")
}

// GetABTestStats 查询 SOP 的 A/B 测试 variant 统计
func (c *SOPController) GetABTestStats(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	stats, err := c.svc.GetABTestStats(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, stats, "查询成功")
}

// UpdateABTestConfig 更新 SOP 的 A/B 测试配置
func (c *SOPController) UpdateABTestConfig(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的 SOP ID")
		return
	}
	var cfg service.SOPABTestConfig
	if err := ctx.ShouldBindJSON(&cfg); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agent, err := c.svc.UpdateABTestConfig(ctx.Request.Context(), uint(id), cfg)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, agent, "更新成功")
}
