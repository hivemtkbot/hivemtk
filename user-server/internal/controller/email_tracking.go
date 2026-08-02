package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// EmailTrackingController 邮件追踪控制器
//
// 路由（公开，邮件客户端直接访问，无 JWT）：
//   - GET /api/email/track/open/{token}.png  追踪像素（1x1 透明 PNG）
//   - GET /api/email/track/click/{token}     链接重定向（302）
//
// 路由（鉴权）：
//   - GET /api/email/track/jobs/:id/metrics      任务指标
//   - GET /api/email/track/jobs/:id/events       任务事件列表
//   - GET /api/email/track/metrics               区间聚合指标
type EmailTrackingController struct {
	svc *service.EmailTrackingService
}

// NewEmailTrackingController 创建邮件追踪控制器
func NewEmailTrackingController(svc *service.EmailTrackingService) *EmailTrackingController {
	return &EmailTrackingController{svc: svc}
}

// transparent1x1PNG 1x1 透明 PNG 字节
// 用于追踪像素：邮件客户端加载图片时触发 GET 请求
var transparent1x1PNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4, // RGBA
	0x89, 0x00, 0x00, 0x00, 0x0D, 0x49, 0x44, 0x41, // IDAT chunk
	0x54, 0x78, 0x9C, 0x62, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, // IEND chunk
	0x42, 0x60, 0x82,
}

// TrackingPixel GET /api/email/track/open/{token}.png
//
// 路由路径形如 /api/email/track/open/abc.def.png
// 控制器从 path 中剥离 .png 后缀获取 token
func (c *EmailTrackingController) TrackingPixel(ctx *gin.Context) {
	token := ctx.Param("token")
	// 兼容直接带 .png 后缀的情况
	token = strings.TrimSuffix(token, ".png")
	if token == "" {
		ctx.Data(http.StatusOK, "image/png", transparent1x1PNG)
		return
	}

	ip := ctx.ClientIP()
	ua := ctx.Request.UserAgent()
	// 异常不影响图片返回（邮件客户端对错误响应敏感）
	_ = c.svc.RecordOpenEvent(ctx.Request.Context(), token, ip, ua)

	ctx.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	ctx.Header("Pragma", "no-cache")
	ctx.Data(http.StatusOK, "image/png", transparent1x1PNG)
}

// ClickRedirect GET /api/email/track/click/{token}?url=xxx
//
// 记录点击事件后 302 跳转到目标 URL
// 优先使用 token 内 target；缺失时取 query 参数 url
func (c *EmailTrackingController) ClickRedirect(ctx *gin.Context) {
	token := ctx.Param("token")
	if token == "" {
		ctx.String(http.StatusBadRequest, "缺少 token 参数")
		return
	}

	ip := ctx.ClientIP()
	ua := ctx.Request.UserAgent()
	target, err := c.svc.RecordClickEvent(ctx.Request.Context(), token, ip, ua)
	if err != nil {
		ctx.String(http.StatusBadRequest, "追踪链接无效或已过期：%s", err.Error())
		return
	}

	if target == "" {
		target = ctx.Query("url")
	}
	if target == "" {
		ctx.String(http.StatusBadRequest, "缺少跳转目标 URL")
		return
	}

	ctx.Redirect(http.StatusFound, target)
}

// GetJobMetrics GET /api/email/track/jobs/:id/metrics
func (c *EmailTrackingController) GetJobMetrics(ctx *gin.Context) {
	jobID := ctx.Param("id")
	if jobID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少 job_id")
		return
	}

	metric, err := c.svc.GetJobMetrics(context.Background(), jobID)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取指标失败："+err.Error())
		return
	}

	response.Success(ctx, metric, "success")
}

// ListJobEvents GET /api/email/track/jobs/:id/events
func (c *EmailTrackingController) ListJobEvents(ctx *gin.Context) {
	jobID := ctx.Param("id")
	if jobID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少 job_id")
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "20"))

	events, total, err := c.svc.ListJobEvents(context.Background(), jobID, page, limit)
	if err != nil {
		response.ErrorFromDB(ctx, err, "查询事件失败："+err.Error())
		return
	}

	response.SuccessWithPage(ctx, events, int64(page), int64(limit), total)
}

// GetRangeMetricsRequest 区间指标请求
type GetRangeMetricsRequest struct {
	Start string `form:"start" binding:"required"`
	End   string `form:"end" binding:"required"`
}

// GetRangeMetrics GET /api/email/track/metrics?start=...&end=...
//
// 时间格式： 或 RFC3339
func (c *EmailTrackingController) GetRangeMetrics(ctx *gin.Context) {
	var req GetRangeMetricsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	start, err := parseFlexibleTime(req.Start)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "start 时间格式错误")
		return
	}
	end, err := parseFlexibleTime(req.End)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "end 时间格式错误")
		return
	}

	metric, err := c.svc.GetEmailMetrics(context.Background(), start, end)
	if err != nil {
		response.ErrorFromDB(ctx, err, "聚合指标失败："+err.Error())
		return
	}

	response.Success(ctx, metric, "success")
}

// RegisterRoutes 注册路由
//
// 公开路由：追踪像素 / 点击重定向（邮件客户端访问，无 JWT）
// 鉴权路由：指标查询（后台管理）
func (c *EmailTrackingController) RegisterRoutes(public *gin.RouterGroup, authed *gin.RouterGroup) {
	if public != nil {
		public.GET("/email/track/open/:token", c.TrackingPixel)
		public.GET("/email/track/click/:token", c.ClickRedirect)
	}
	if authed != nil {
		authed.GET("/email/track/jobs/:id/metrics", c.GetJobMetrics)
		authed.GET("/email/track/jobs/:id/events", c.ListJobEvents)
		authed.GET("/email/track/metrics", c.GetRangeMetrics)
	}
}

// parseFlexibleTime 解析多种时间格式
//
// 支持：

// Z07:00 (RFC3339)

func parseFlexibleTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time")
	}
	formats := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("unsupported time format")
}
