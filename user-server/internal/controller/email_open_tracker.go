package controller

// email_open_tracker_controller.go 邮件打开率追踪控制器
//
// 五层架构归属: L3 业务层
// 设计依据: docs/核心链路优化.md §13.2 邮件打开率追踪
//
// 路由（公开，邮件客户端直接访问，无 JWT）：
//   - GET /api/email/track/pixel/{token}.png  1×1 透明 PNG 像素
//   - POST /api/email/track/webhook/postmark  Postmark 风格 webhook
//   - POST /api/email/track/webhook/sendcloud 塞邮式 (SendCloud) webhook
//
// 路由（鉴权）：
//   - GET /api/email/track/open-metrics     任务打开率指标
//
// 私域独立部署: 无 merchant_id 字段

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// EmailOpenTrackerController 邮件打开率追踪控制器
type EmailOpenTrackerController struct {
	svc *service.EmailOpenTrackerService
}

// NewEmailOpenTrackerController 创建控制器
func NewEmailOpenTrackerController(svc *service.EmailOpenTrackerService) *EmailOpenTrackerController {
	return &EmailOpenTrackerController{svc: svc}
}

// TrackingPixel GET /api/email/track/pixel/{token}.png
//
// 兼容 .png 后缀的 URL，邮件客户端按 image/png 解析
func (c *EmailOpenTrackerController) TrackingPixel(ctx *gin.Context) {
	if c.svc == nil {
		// 即使服务不可用也返回像素（不影响邮件显示）
		ctx.Data(http.StatusOK, service.EmailOpenPixelContentType, service.EmailOpenPixel)
		return
	}
	token := strings.TrimSuffix(ctx.Param("token"), ".png")
	ip := ctx.ClientIP()
	ua := ctx.Request.UserAgent()
	pixel, contentType, maxAge, err := c.svc.RenderPixel(ctx.Request.Context(), token, ip, ua)
	if err != nil {
		// token 无效也返回像素（邮件客户端过滤）
		ctx.Data(http.StatusOK, service.EmailOpenPixelContentType, service.EmailOpenPixel)
		return
	}
	ctx.Header("Cache-Control", "public, max-age="+strconv.Itoa(maxAge)+", immutable")
	ctx.Data(http.StatusOK, contentType, pixel)
}

// PostmarkWebhook POST /api/email/track/webhook/postmark
func (c *EmailOpenTrackerController) PostmarkWebhook(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "邮件打开追踪服务未初始化")
		return
	}
	var evt service.PostmarkOpenEvent
	if err := ctx.ShouldBindJSON(&evt); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	if err := c.svc.RecordPostmarkEvent(ctx.Request.Context(), &evt); err != nil {
		response.ErrorFromDB(ctx, err, "记录事件失败："+err.Error())
		return
	}
	response.Success(ctx, gin.H{"recorded": true, "type": evt.RecordType}, "ok")
}

// SendCloudWebhook POST /api/email/track/webhook/sendcloud
func (c *EmailOpenTrackerController) SendCloudWebhook(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "邮件打开追踪服务未初始化")
		return
	}
	var evt service.SendCloudOpenEvent
	if err := ctx.ShouldBindJSON(&evt); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}
	if err := c.svc.RecordSendCloudEvent(ctx.Request.Context(), &evt); err != nil {
		response.ErrorFromDB(ctx, err, "记录事件失败："+err.Error())
		return
	}
	response.Success(ctx, gin.H{"recorded": true, "event": evt.Event}, "ok")
}

// GetOpenMetrics GET /api/email/track/open-metrics?job_id=xxx&total_sent=100
func (c *EmailOpenTrackerController) GetOpenMetrics(ctx *gin.Context) {
	if c.svc == nil {
		response.Error(ctx, http.StatusServiceUnavailable, "邮件打开追踪服务未初始化")
		return
	}
	jobID := ctx.Query("job_id")
	if jobID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少 job_id 参数")
		return
	}
	totalSent, _ := strconv.ParseInt(ctx.DefaultQuery("total_sent", "0"), 10, 64)
	m, err := c.svc.GetOpenRateMetrics(ctx.Request.Context(), jobID, totalSent)
	if err != nil {
		response.ErrorFromDB(ctx, err, "查询打开率失败："+err.Error())
		return
	}
	response.Success(ctx, m, "ok")
}

// RegisterRoutes 注册路由
//
// 公开路由：像素 / webhook（邮件客户端或 SMTP 推送，无 JWT）
// 鉴权路由：打开率指标（后台管理）
func (c *EmailOpenTrackerController) RegisterRoutes(public *gin.RouterGroup, authed *gin.RouterGroup) {
	if public != nil {
		public.GET("/email/track/pixel/:token", c.TrackingPixel)
		public.POST("/email/track/webhook/postmark", c.PostmarkWebhook)
		public.POST("/email/track/webhook/sendcloud", c.SendCloudWebhook)
	}
	if authed != nil {
		authed.GET("/email/track/open-metrics", c.GetOpenMetrics)
	}
}
