package controller

import (
	"context"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

// KuaishouCardStatsController 快手卡片统计控制器
type KuaishouCardStatsController struct {
	statsService *service.KuaishouCardStatsService
}

// NewKuaishouCardStatsController 创建快手卡片统计控制器实例
func NewKuaishouCardStatsController(statsService *service.KuaishouCardStatsService) *KuaishouCardStatsController {
	return &KuaishouCardStatsController{
		statsService: statsService,
	}
}

// GetCardStats 获取单个快手卡片的统计数据
func (c *KuaishouCardStatsController) GetCardStats(ctx *gin.Context) {
	var req dto.KuaishouCardStatsRequest

	// 解析卡片ID
	cardIDStr := ctx.Param("id")
	cardID, err := strconv.ParseUint(cardIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, 400, "无效的卡片ID", err.Error())
		return
	}
	req.CardID = uint(cardID)

	// 解析查询参数
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}

	// 获取统计数据
	stats, err := c.statsService.GetCardStats(context.Background(), &req)
	if HandleDBError(ctx, err, "获取快手卡片统计") {
		return
	}

	response.Success(ctx, stats, "获取统计数据成功")
}

// GetOverallStats 获取快手卡片总体统计数据
func (c *KuaishouCardStatsController) GetOverallStats(ctx *gin.Context) {
	var req dto.KuaishouCardOverallStatsRequest

	// 解析查询参数
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, 400, "参数错误", err.Error())
		return
	}

	// 获取统计数据
	stats, err := c.statsService.GetOverallStats(context.Background(), &req)
	if err != nil {
		response.Error(ctx, 500, "获取总体统计数据失败", err.Error())
		return
	}

	response.Success(ctx, stats, "获取统计数据成功")
}
