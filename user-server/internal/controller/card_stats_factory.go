package controller

import (
	"strconv"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// CardStatsFactoryController 卡片统计平台路由器
//
// LM-：以前每平台各自维护一套 controller（douyin/kuaishou/xiaohongshu/xianyu）
// + stats 接口，参数命名、返回结构不一致。本 controller 通过
// :platform 路径参数动态选择对应的 service.PlatformCardStatsService，
// 对外暴露统一的 /api/card-stats/:platform/stats/:id 与
// /api/card-stats/:platform/overall 接口。
//
// 支持的平台：douyin / kuaishou / xiaohongshu / xianyu / tiktok
type CardStatsFactoryController struct {
	registry map[string]service.PlatformCardStatsService
}

// NewCardStatsFactoryController 创建平台路由器
func NewCardStatsFactoryController(services ...service.PlatformCardStatsService) *CardStatsFactoryController {
	reg := make(map[string]service.PlatformCardStatsService, len(services))
	for _, s := range services {
		if s == nil {
			continue
		}
		reg[s.Platform()] = s
	}
	return &CardStatsFactoryController{registry: reg}
}

// resolve 解析 platform 路径参数，返回对应 service
func (c *CardStatsFactoryController) resolve(ctx *gin.Context) (service.PlatformCardStatsService, bool) {
	platform := ctx.Param("platform")
	if platform == "" {
		// 也支持 query 参数（兼容旧调用）
		platform = ctx.Query("platform")
	}
	svc, ok := c.registry[platform]
	if !ok {
		response.Error(ctx, 400, "不支持的平台", "platform="+platform)
		return nil, false
	}
	return svc, true
}

// GetCardStats 统一获取单卡片统计
//
// 路由：GET /api/card-stats/:platform/stats/:id
func (c *CardStatsFactoryController) GetCardStats(ctx *gin.Context) {
	svc, ok := c.resolve(ctx)
	if !ok {
		return
	}

	cardIDStr := ctx.Param("id")
	cardID, err := strconv.ParseUint(cardIDStr, 10, 32)
	if err != nil {
		response.Error(ctx, 400, "无效的卡片ID", err.Error())
		return
	}

	req := buildPlatformCardStatsRequest(ctx, uint(cardID), svc.Platform())
	stats, err := svc.GetCardStats(ctx.Request.Context(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取卡片统计失败", err.Error())
		return
	}
	response.Success(ctx, stats, "获取成功")
}

// GetOverallStats 统一获取全部卡片总体统计
//
// 路由：GET /api/card-stats/:platform/overall
func (c *CardStatsFactoryController) GetOverallStats(ctx *gin.Context) {
	svc, ok := c.resolve(ctx)
	if !ok {
		return
	}
	req := buildPlatformCardOverallStatsRequest(ctx, svc.Platform())
	stats, err := svc.GetOverallStats(ctx.Request.Context(), req)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取总体统计失败", err.Error())
		return
	}
	response.Success(ctx, stats, "获取成功")
}

// buildPlatformCardStatsRequest 从 gin.Context 构造统一请求
func buildPlatformCardStatsRequest(ctx *gin.Context, cardID uint, platform string) *dto.PlatformCardStatsRequest {
	groupBy := ctx.DefaultQuery("groupBy", "day")
	startDate := ctx.Query("startDate")
	if startDate == "" {
		startDate = ctx.Query("start_date")
	}
	endDate := ctx.Query("endDate")
	if endDate == "" {
		endDate = ctx.Query("end_date")
	}
	return &dto.PlatformCardStatsRequest{
		Platform:  platform,
		CardID:    cardID,
		StartDate: startDate,
		EndDate:   endDate,
		GroupBy:   groupBy,
	}
}

// buildPlatformCardOverallStatsRequest 从 gin.Context 构造统一总体统计请求
func buildPlatformCardOverallStatsRequest(ctx *gin.Context, platform string) *dto.PlatformCardOverallStatsRequest {
	groupBy := ctx.DefaultQuery("groupBy", "day")
	startDate := ctx.Query("startDate")
	if startDate == "" {
		startDate = ctx.Query("start_date")
	}
	endDate := ctx.Query("endDate")
	if endDate == "" {
		endDate = ctx.Query("end_date")
	}
	limitStr := ctx.DefaultQuery("limit", "10")
	limit, _ := strconv.Atoi(limitStr)
	return &dto.PlatformCardOverallStatsRequest{
		Platform:  platform,
		GroupBy:   groupBy,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     limit,
	}
}
