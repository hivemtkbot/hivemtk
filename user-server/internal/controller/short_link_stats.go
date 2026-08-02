package controller

import (
	"context"
	"marketing/internal/dto"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ShortLinkStatsController 短链统计控制器
type ShortLinkStatsController struct {
	shortLinkService service.ShortLinkService
}

// NewShortLinkStatsController 创建短链统计控制器实例
func NewShortLinkStatsController(shortLinkService service.ShortLinkService) *ShortLinkStatsController {
	return &ShortLinkStatsController{
		shortLinkService: shortLinkService,
	}
}

// GetStats 获取短链统计
func (c *ShortLinkStatsController) GetStats(ctx *gin.Context) {
	var req dto.ShortLinkStatsRequest

	// 获取短链ID参数
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, http.StatusBadRequest, "短链ID不能为空")
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "短链ID格式错误")
		return
	}
	req.ID = uint(id)

	// 绑定查询参数
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: ")
		return
	}

	// 调用服务
	stats, err := c.shortLinkService.GetStats(context.Background(), &req)
	if HandleDBError(ctx, err, "获取短链统计") {
		return
	}

	response.Success(ctx, stats, "获取短链统计成功")
}

// GetAllStats 获取所有短链统计
func (c *ShortLinkStatsController) GetAllStats(ctx *gin.Context) {
	var req dto.AllShortLinksStatsRequest

	// 绑定查询参数
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误: ")
		return
	}

	// 调用服务
	stats, err := c.shortLinkService.GetAllStats(context.Background(), &req)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, stats, "获取所有短链统计成功")
}

// ShareShortLink 分享短链
func (c *ShortLinkStatsController) ShareShortLink(ctx *gin.Context) {
	var req dto.ShareShortLinkRequest

	// 获取短链ID参数
	idStr := ctx.Param("id")
	if idStr == "" {
		response.Error(ctx, http.StatusBadRequest, "短链ID不能为空")
		return
	}

	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "短链ID格式错误")
		return
	}
	req.ID = uint(id)

	// 调用服务
	share, err := c.shortLinkService.ShareShortLink(context.Background(), &req)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, share, "获取短链分享信息成功")
}
