package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

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

// Create godoc
// @Summary      创建 SOP 智能体
// @Description  创建一个新的销冠 SOP 智能体，关联场景、动作链、知识库
// @Tags         SOP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  service.CreateRequest  true  "SOP 创建参数"
// @Success      200   {object}  response.Response  "创建成功"
// @Failure      400   {object}  response.Response  "参数错误"
// @Router       /api/sops [post]
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

// Update godoc
// @Summary      更新 SOP 智能体
// @Description  更新指定 ID 的 SOP 智能体配置
// @Tags         SOP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path   int                       true  "SOP ID"
// @Param        body  body   service.CreateRequest  true  "更新参数"
// @Success      200   {object}  response.Response  "更新成功"
// @Router       /api/sops/{id} [put]
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

// Get godoc
// @Summary      获取 SOP 详情
// @Description  根据 ID 返回 SOP 完整定义
// @Tags         SOP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id  path  int  true  "SOP ID"
// @Success      200  {object}  response.Response  "成功"
// @Failure      404  {object}  response.Response  "未找到"
// @Router       /api/sops/{id} [get]
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

// List godoc
// @Summary      SOP 智能体列表
// @Description  按场景分页查询 SOP 智能体
// @Tags         SOP
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        scenario  query  string  false  "场景编码"
// @Param        page      query  int     false  "页码"  default(1)
// @Param        page_size query  int     false  "每页"   default(20)
// @Success      200  {object}  response.Response  "成功"
// @Router       /api/sops [get]
func (c *SOPController) List(ctx *gin.Context) {
	scenario := ctx.Query("scenario")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	list, total, err := c.svc.List(ctx.Request.Context(), scenario, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
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
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.SuccessWithPage(ctx, list, int64(page), int64(pageSize), total)
}

// MatchByIntent 意图匹配
func (c *SOPController) MatchByIntent(ctx *gin.Context) {
	intent := ctx.Query("intent")
	list, err := c.svc.MatchByIntent(ctx.Request.Context(), intent)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}
	response.Success(ctx, list, "查询成功")
}

// Stats 统计
func (c *SOPController) Stats(ctx *gin.Context) {
	stats, err := c.svc.Stats(ctx.Request.Context())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
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
