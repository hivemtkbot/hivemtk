package controller

import (
	"hivemtk-user/internal/middleware"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// RateQuotaController 限流配额面板控制器
// G11: 竞品标配功能 - 展示各 API 路径的限流配置和当前用量
type RateQuotaController struct{}

// NewRateQuotaController 创建限流配额控制器
func NewRateQuotaController() *RateQuotaController {
	return &RateQuotaController{}
}

// QuotaSnapshot 配额快照（复用 middleware.GetQuotaSnapshots 返回的结构）
type QuotaSnapshot struct {
	Path            string  `json:"path"`
	ConfiguredRPS   float64 `json:"configured_rps"`
	ConfiguredBurst int     `json:"configured_burst"`
	CurrentMinute   int64   `json:"current_minute"`
	Used            int64   `json:"used"`
	Remaining       int64   `json:"remaining"`
	Triggered       int64   `json:"triggered"`
}

// GetRateQuota 获取全局限流配额和当前用量
// GET /api/system/rate-quota
func (c *RateQuotaController) GetRateQuota(ctx *gin.Context) {
	snapshots := middleware.GetQuotaSnapshots()

	data := gin.H{
		"global": gin.H{
			"rps":         1000,
			"bucket_size": 20000,
			"enabled":     true,
		},
		"paths": snapshots,
	}
	response.Success(ctx, data, "获取成功")
}
