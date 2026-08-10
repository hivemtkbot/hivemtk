package controller

import (
	"net/http"
	"strconv"

	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// AIToolConfigController AI工具配置控制器
type AIToolConfigController struct {
	service *service.AIToolConfigService
}

// NewAIToolConfigController 创建AI工具配置控制器
func NewAIToolConfigController(service *service.AIToolConfigService) *AIToolConfigController {
	return &AIToolConfigController{service: service}
}

// ListTools 获取工具列表
// GET /api/ai-tools?category=reach&enabled=true&page=1&page_size=20
func (c *AIToolConfigController) ListTools(ctx *gin.Context) {
	category := ctx.Query("category")
	enabledStr := ctx.Query("enabled")
	pageStr := ctx.DefaultQuery("page", "1")
	pageSizeStr := ctx.DefaultQuery("page_size", "20")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var enabled *bool
	if enabledStr != "" {
		e := enabledStr == "true"
		enabled = &e
	}

	result, err := c.service.ListTools(category, enabled, page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取工具列表失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// GetTool 获取工具详情
// GET /api/ai-tools/:name
func (c *AIToolConfigController) GetTool(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工具名称不能为空"})
		return
	}

	result, err := c.service.GetTool(name)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "工具不存在"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// UpdateToolStatus 更新工具状态
// PUT /api/ai-tools/:name/status
func (c *AIToolConfigController) UpdateToolStatus(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工具名称不能为空"})
		return
	}

	var req struct {
		IsEnabled bool `json:"is_enabled"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	if err := c.service.UpdateStatus(name, req.IsEnabled); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "更新状态失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// BatchUpdateStatus 批量更新工具状态
// POST /api/ai-tools/batch-status
func (c *AIToolConfigController) BatchUpdateStatus(ctx *gin.Context) {
	var req struct {
		Tools     []string `json:"tools"`
		IsEnabled bool     `json:"is_enabled"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	if len(req.Tools) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工具列表不能为空"})
		return
	}

	if err := c.service.BatchUpdateStatus(req.Tools, req.IsEnabled); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "批量更新失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// GetToolAccounts 获取工具绑定的账号
// GET /api/ai-tools/:name/accounts
func (c *AIToolConfigController) GetToolAccounts(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工具名称不能为空"})
		return
	}

	result, err := c.service.GetToolAccounts(name)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "获取绑定账号失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "data": result})
}

// BindAccount 绑定账号到工具
// POST /api/ai-tools/:name/accounts
func (c *AIToolConfigController) BindAccount(ctx *gin.Context) {
	name := ctx.Param("name")
	if name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "工具名称不能为空"})
		return
	}

	var req struct {
		AccountType string `json:"account_type"`
		AccountID   string `json:"account_id"`
		IsPrimary   bool   `json:"is_primary"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "请求参数错误"})
		return
	}

	if req.AccountType == "" || req.AccountID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "账号类型和ID不能为空"})
		return
	}

	if err := c.service.BindAccount(name, req.AccountType, req.AccountID, req.IsPrimary); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "绑定失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}

// UnbindAccount 解绑账号
// DELETE /api/ai-tools/:name/accounts/:account_type/:account_id
func (c *AIToolConfigController) UnbindAccount(ctx *gin.Context) {
	name := ctx.Param("name")
	accountType := ctx.Param("account_type")
	accountID := ctx.Param("account_id")

	if name == "" || accountType == "" || accountID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "参数不完整"})
		return
	}

	if err := c.service.UnbindAccount(name, accountType, accountID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "解绑失败: " + err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"code": 0, "message": "success"})
}
