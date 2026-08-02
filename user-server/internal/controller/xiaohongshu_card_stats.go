package controller

import (
	"context"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type XiaohongshuCardStatsController struct {
	statsService service.XiaohongshuCardStatsService
}

func NewXiaohongshuCardStatsController(statsService service.XiaohongshuCardStatsService) *XiaohongshuCardStatsController {
	return &XiaohongshuCardStatsController{
		statsService: statsService,
	}
}

// GetCardStats 获取单个小红书卡片的统计数据
func (c *XiaohongshuCardStatsController) GetCardStats(ctx *gin.Context) {
	cardIDStr := ctx.Param("id")
	cardID, err := strconv.ParseUint(cardIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, 400, "无效的卡片ID", err.Error())
		return
	}

	// 构建请求
	req := &dto.XiaohongshuCardStatsRequest{
		CardID:    uint(cardID),
		StartDate: ctx.Query("start_date"),
		EndDate:   ctx.Query("end_date"),
		GroupBy:   ctx.Query("group_by"),
	}

	// 设置默认时间范围（最近7天）
	if req.StartDate == "" || req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
		req.StartDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}

	// 设置默认分组方式
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}

	stats, err := c.statsService.GetCardStats(context.Background(), req)
	if HandleDBError(ctx, err, "获取小红书卡片统计") {
		return
	}

	response.Success(ctx, stats, "获取统计数据成功")
}

// GetOverallStats 获取小红书卡片的总体统计数据
func (c *XiaohongshuCardStatsController) GetOverallStats(ctx *gin.Context) {
	// 构建请求
	req := &dto.XiaohongshuCardOverallStatsRequest{
		GroupBy:   ctx.Query("group_by"),
		StartDate: ctx.Query("start_date"),
		EndDate:   ctx.Query("end_date"),
	}

	// 设置默认时间范围（最近30天）
	if req.StartDate == "" || req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
		req.StartDate = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	}

	// 设置默认分组方式
	if req.GroupBy == "" {
		req.GroupBy = "day"
	}

	stats, err := c.statsService.GetOverallStats(context.Background(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取统计数据失败", err.Error())
		return
	}

	response.Success(ctx, stats, "获取统计数据成功")
}
