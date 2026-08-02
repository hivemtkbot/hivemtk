package controller

// rag_health_controller.go RAG 健康度评估控制器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md §14.6.4 RAG 健康度
//
// 路由（全部鉴权）：
//   - GET /api/rag/health           综合健康度评分（带 30 秒缓存）
//   - GET /api/rag/health/refresh   强制刷新（不走缓存）

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// RagHealthController RAG 健康度控制器
type RagHealthController struct {
	svc *service.RagHealthService
}

// NewRagHealthController 创建控制器
func NewRagHealthController(svc *service.RagHealthService) *RagHealthController {
	return &RagHealthController{svc: svc}
}

// GetHealth godoc
// @Summary      RAG 系统健康度评分
// @Description  30 秒内复用缓存；窗口可通过 window_seconds 自定义
// @Tags         RAG Health
// @Produce      json
// @Param        window_seconds  query  int  false  "评估窗口（秒），默认 3600"
// @Success      200  {object}  service.RagHealthReport
// @Router       /api/rag/health [get]
func (c *RagHealthController) GetHealth(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "RAG 健康度服务未初始化")
		return
	}
	windowSec, _ := strconv.Atoi(ctx.DefaultQuery("window_seconds", "0"))
	window := time.Duration(windowSec) * time.Second
	report, err := c.svc.GetHealthCached(ctx.Request.Context(), window)
	if err != nil {
		response.ErrorFromDB(ctx, err, "健康度评估失败："+err.Error())
		return
	}
	response.Success(ctx, report, "ok")
}

// RefreshHealth godoc
// @Summary      强制刷新健康度评分
// @Description  不使用 30 秒缓存，立即重新计算
// @Tags         RAG Health
// @Produce      json
// @Param        window_seconds  query  int  false  "评估窗口（秒），默认 3600"
// @Success      200  {object}  service.RagHealthReport
// @Router       /api/rag/health/refresh [get]
func (c *RagHealthController) RefreshHealth(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "RAG 健康度服务未初始化")
		return
	}
	windowSec, _ := strconv.Atoi(ctx.DefaultQuery("window_seconds", "0"))
	window := time.Duration(windowSec) * time.Second
	report, err := c.svc.GetHealth(ctx.Request.Context(), window)
	if err != nil {
		response.ErrorFromDB(ctx, err, "健康度评估失败："+err.Error())
		return
	}
	response.Success(ctx, report, "ok")
}

// RegisterRoutes 注册路由
func (c *RagHealthController) RegisterRoutes(auth *gin.RouterGroup) {
	group := auth.Group("/rag/health")
	{
		group.GET("", c.GetHealth)
		group.GET("/refresh", c.RefreshHealth)
	}
}
