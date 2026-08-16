package controller

import (
	"hivemtk-user/internal/ops/service"
	"hivemtk-user/internal/pkg/errhttp"
	"hivemtk-user/internal/pkg/utils/response"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// DashboardScreenController 数据大屏控制器
type DashboardScreenController struct {
	screenService *service.DashboardScreenService
}

// NewDashboardScreenController 创建数据大屏控制器实例
func NewDashboardScreenController() *DashboardScreenController {
	return &DashboardScreenController{
		screenService: service.NewDashboardScreenService(),
	}
}

// CreateScreen 创建大屏
func (c *DashboardScreenController) CreateScreen(ctx *gin.Context) {

	userID, _ := ctx.Get("user_id")
	var req service.CreateScreenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	screen, err := c.screenService.CreateScreen(userID.(uint), &req)
	if errhttp.HandleDBError(ctx, err, "创建大屏") {
		return
	}

	response.Success(ctx, screen, "创建成功")
}

// GetScreenList 获取大屏列表
func (c *DashboardScreenController) GetScreenList(ctx *gin.Context) {

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	screens, total, err := c.screenService.GetScreenList(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      screens,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetScreenByID 获取大屏详情
func (c *DashboardScreenController) GetScreenByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的大屏 ID")
		return
	}

	screen, err := c.screenService.GetScreenByID(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, screen, "获取成功")
}

// UpdateScreen 更新大屏
func (c *DashboardScreenController) UpdateScreen(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的大屏 ID")
		return
	}

	var req service.UpdateScreenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误："+err.Error())
		return
	}

	screen, err := c.screenService.UpdateScreen(uint(id), &req)
	if errhttp.HandleDBError(ctx, err, "更新大屏") {
		return
	}

	response.Success(ctx, screen, "更新成功")
}

// DeleteScreen 删除大屏
func (c *DashboardScreenController) DeleteScreen(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的大屏 ID")
		return
	}

	if errhttp.HandleDBError(ctx, c.screenService.DeleteScreen(uint(id)), "删除大屏") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// PublicViewScreen 公开访问大屏
func (c *DashboardScreenController) PublicViewScreen(ctx *gin.Context) {
	code := ctx.Param("code")

	screen, err := c.screenService.GetPublicScreen(code)
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	widgets, _ := c.screenService.GetScreenWidgets(screen.ID)

	response.Success(ctx, gin.H{
		"screen":  screen,
		"widgets": widgets,
	}, "获取成功")
}

// GetDashboardData 获取仪表板大屏实时聚合数据
// 该接口汇总线索、客户、订单、消息、抖音/小红书卡片等多源真实数据
// 用于前端大屏可视化展示,绝不返回模拟数据
// 架构修复：通过 Service 层访问 DB，不再直接调 db.GetDB
func (c *DashboardScreenController) GetDashboardData(ctx *gin.Context) {
	data, err := c.screenService.AggregateDashboardData()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "聚合大屏数据失败: "+err.Error())
		return
	}
	response.Success(ctx, data, "获取仪表板数据成功")
}

// GetRealtimeActivities 获取实时活动
// 架构修复：通过 Service 层访问 DB
func (c *DashboardScreenController) GetRealtimeActivities(ctx *gin.Context) {
	activities, err := c.screenService.FetchRealtimeActivities(20)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取实时活动失败: "+err.Error())
		return
	}
	response.Success(ctx, activities, "获取实时活动成功")
}







