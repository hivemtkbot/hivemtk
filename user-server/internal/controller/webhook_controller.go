package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"marketing/internal/pkg/utils/db"
	"marketing/internal/service"
)

// WebhookController 多渠道 Webhook 控制器
// 对应 P0-14 多渠道 Webhook
// 提供 9 个渠道的回调入口
type WebhookController struct {
	svc *service.WebhookService
}

// NewWebhookController 创建 Webhook 控制器
func NewWebhookController() *WebhookController {
	return &WebhookController{
		svc: service.NewWebhookService(db.GetDB()),
	}
}

// Stop 关闭后台 worker（用于测试或优雅退出）
func (c *WebhookController) Stop() {
	if c.svc != nil {
		c.svc.Stop()
	}
}

// RegisterRoutes 注册路由（公开，不需要鉴权）
func (c *WebhookController) RegisterRoutes(r *gin.Engine) {
	// 多渠道统一入口
	wh := r.Group("/api/webhook")
	{
		// 公开渠道
		wh.POST("/:channel/:account_id", c.Receive)
		wh.POST("/:channel", c.ReceiveWithoutAccount)
		// 业务查询
		wh.GET("/stats", c.Stats)
		wh.GET("/health", c.Health)
		// 企微 GET 验证（URL 验证挑战）
		wh.GET("/wecom/:account_id", c.WeComVerify)
		// 飞书 GET 验证
		wh.GET("/feishu/:account_id", c.FeishuVerify)
	}
}

// Receive 接收渠道回调（需要 accountID）
// 路由: POST /api/webhook/{channel}/{account_id}
func (c *WebhookController) Receive(ctx *gin.Context) {
	channel := service.WebhookChannel(strings.ToLower(ctx.Param("channel")))
	accountID := ctx.Param("account_id")

	body, err := service.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"accepted": false,
			"reason":   "read body: " + err.Error(),
		})
		return
	}

	req := &service.ReceiveRequest{
		Channel:   channel,
		AccountID: accountID,
		Body:      body,
		Headers:   extractHeaders(ctx),
		SourceIP:  ctx.ClientIP(),
		Query:     extractQuery(ctx),
	}

	result, _ := c.svc.Receive(ctx.Request.Context(), req)
	status := http.StatusOK
	if !result.Accepted {
		status = http.StatusBadRequest
		if result.VerifyFail {
			status = http.StatusUnauthorized
		}
		if result.RateLimit {
			status = http.StatusTooManyRequests
		}
	}
	ctx.JSON(status, result)
}

// ReceiveWithoutAccount 不带 account_id 的入口
// 路由: POST /api/webhook/{channel}
func (c *WebhookController) ReceiveWithoutAccount(ctx *gin.Context) {
	channel := service.WebhookChannel(strings.ToLower(ctx.Param("channel")))

	body, err := service.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"accepted": false,
			"reason":   "read body: " + err.Error(),
		})
		return
	}

	// 从 body 解析 account_id（如果提供）
	accountID := extractAccountID(body)

	req := &service.ReceiveRequest{
		Channel:   channel,
		AccountID: accountID,
		Body:      body,
		Headers:   extractHeaders(ctx),
		SourceIP:  ctx.ClientIP(),
		Query:     extractQuery(ctx),
	}

	result, _ := c.svc.Receive(ctx.Request.Context(), req)
	status := http.StatusOK
	if !result.Accepted {
		status = http.StatusBadRequest
	}
	ctx.JSON(status, result)
}

// WeComVerify 企微 URL 验证（GET 挑战）
// 路由: GET /api/webhook/wecom/{account_id}?msg_signature=...&timestamp=...&nonce=...&echostr=...
func (c *WebhookController) WeComVerify(ctx *gin.Context) {
	accountID := ctx.Param("account_id")
	sig := ctx.Query("msg_signature")
	ts := ctx.Query("timestamp")
	nonce := ctx.Query("nonce")
	echo := ctx.Query("echostr")

	if sig == "" || ts == "" || nonce == "" || echo == "" {
		ctx.String(http.StatusBadRequest, "missing signature params")
		return
	}

	// 从 wecom_accounts 读取 token + EncodingAESKey
	token, aesKey, err := c.svc.GetWeComSecrets(accountID)
	if err != nil || token == "" {
		ctx.String(http.StatusUnauthorized, "account not found or token missing")
		return
	}
	plain, err := service.VerifyURL(token, aesKey, sig, ts, nonce, echo)
	if err != nil {
		ctx.String(http.StatusUnauthorized, "verify failed: "+err.Error())
		return
	}
	ctx.String(http.StatusOK, plain)
}

// FeishuVerify 飞书 URL 验证（GET 挑战）
// 路由: GET /api/webhook/feishu/{account_id}?challenge=...&token=...&type=url_verification
// 飞书 URL 验证只是 JSON 回显 challenge
func (c *WebhookController) FeishuVerify(ctx *gin.Context) {
	challenge := ctx.Query("challenge")
	if challenge == "" {
		ctx.String(http.StatusBadRequest, "missing challenge")
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"challenge": challenge})
}

// Stats 统计
func (c *WebhookController) Stats(ctx *gin.Context) {
	pending := c.svc.PendingCount()
	queueLen := c.svc.QueueLen()
	ctx.JSON(http.StatusOK, gin.H{
		"pending_events": pending,
		"queue_length":   queueLen,
	})
}

// Health 健康检查
func (c *WebhookController) Health(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// extractHeaders 提取所需 headers
func extractHeaders(ctx *gin.Context) map[string]string {
	headers := make(map[string]string)
	for _, k := range []string{
		"X-Signature", "Signature", "X-Hub-Signature-256",
		"X-Douyin-Signature", "X-Lark-Signature",
		"X-Wechat-Timestamp", "X-Wechat-Nonce", "X-Wechat-Signature",
		// Telegram Bot API 通过此头传递 webhook secret（setWebhook 时配置 secret_token 后必带）
		"X-Telegram-Bot-Api-Secret-Token",
	} {
		if v := ctx.GetHeader(k); v != "" {
			headers[k] = v
		}
	}
	return headers
}

// extractQuery 提取 query 参数
func extractQuery(ctx *gin.Context) map[string]string {
	q := make(map[string]string)
	for _, k := range []string{"msg_signature", "timestamp", "nonce", "echostr", "challenge", "token", "type"} {
		if v := ctx.Query(k); v != "" {
			q[k] = v
		}
	}
	return q
}

// extractAccountID 从 body 简单提取 account_id
func extractAccountID(body []byte) string {
	s := string(body)
	idx := strings.Index(s, "\"account_id\"")
	if idx < 0 {
		idx = strings.Index(s, "\"accountId\"")
	}
	if idx < 0 {
		return ""
	}
	rest := s[idx:]
	colon := strings.Index(rest, ":")
	if colon < 0 {
		return ""
	}
	rest = rest[colon+1:]
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, "\"") {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// SetSalesEngine 注入 智能体引擎（在 main.go 中调用）
func (c *WebhookController) SetSalesEngine(e *service.SalesEngine) {
	if c.svc != nil {
		c.svc.SetSalesEngine(e)
	}
}

// SetSmartOrchestrator 注入智能体统一编排器（在 router 中调用）
// 注入后 Webhook 入站消息会走 SmartCSOrchestrator.HandleIncoming 9 步编排
func (c *WebhookController) SetSmartOrchestrator(o *service.SmartCSOrchestrator) {
	if c.svc != nil {
		c.svc.SetSmartOrchestrator(o)
	}
}

// SetAgentBindingService 注入渠道智能体绑定服务（在 router 中调用）
// 注入后 triggerSalesEngine/triggerSmartOrchestrator 会先按 (channel_type, account_id)
// 加载绑定的智能体上下文，再调用 SalesEngine.HandleWithAgent 按智能体配置执行
func (c *WebhookController) SetAgentBindingService(svc *service.ChannelAgentBindingService) {
	if c.svc != nil {
		c.svc.SetAgentBindingService(svc)
	}
}

// EnsureDB 防止 import unused
var _ = gorm.ErrRecordNotFound
