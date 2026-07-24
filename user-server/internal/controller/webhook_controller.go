package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"marketing/internal/service"
)

// WebhookController 多渠道 Webhook 控制器
// 对应 P0-14 多渠道 Webhook
// 提供 9 个渠道的回调入口
type WebhookController struct {
	svc        *service.WebhookService
	waCloudSvc *service.WhatsAppCloudService
	dtAppSvc   *service.DingTalkAppService
}

// NewWebhookController 创建 Webhook 控制器
func NewWebhookController(svc *service.WebhookService) *WebhookController {
	return &WebhookController{
		svc: svc,
	}
}

// SetWhatsAppCloudService 注入 WhatsApp Cloud 账号服务（用于回调 URL 验证）
func (c *WebhookController) SetWhatsAppCloudService(svc *service.WhatsAppCloudService) {
	c.waCloudSvc = svc
}

// SetDingTalkAppService 注入钉钉应用账号服务（用于回调验签 + 入站收消息）
func (c *WebhookController) SetDingTalkAppService(svc *service.DingTalkAppService) {
	c.dtAppSvc = svc
}

// Stop 关闭后台 worker（用于测试或优雅退出）
func (c *WebhookController) Stop() {
	if c.svc != nil {
		c.svc.Stop(context.Background())
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
		// WhatsApp Cloud GET 验证（Meta 回调 URL 挑战）
		wh.GET("/whatsapp/:account_id", c.WhatsAppVerify)
		// 钉钉企业内部应用 GET 验证 + POST 收消息
		wh.GET("/dingtalk/:account_id", c.DingTalkVerify)
		wh.POST("/dingtalk/:account_id", c.DingTalkReceive)
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
	token, aesKey, err := c.svc.GetWeComSecrets(context.Background(), accountID)
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

// WhatsAppVerify 处理 Meta 回调 URL 验证挑战（GET）
// 路由: GET /api/webhook/whatsapp/{account_id}
// Meta 在「Webhook 配置」中填入回调 URL 后会发送 GET 校验：
//
//	hub.mode=subscribe & hub.verify_token=<配置的 token> & hub.challenge=<随机串>
//
// 仅当 hub.verify_token 与账号存储的 VerifyToken 一致时才回显 hub.challenge。
func (c *WebhookController) WhatsAppVerify(ctx *gin.Context) {
	accountIDStr := ctx.Param("account_id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		ctx.String(http.StatusBadRequest, "invalid account_id")
		return
	}
	if c.waCloudSvc == nil {
		ctx.String(http.StatusServiceUnavailable, "whatsapp cloud service not configured")
		return
	}
	acc, err := c.waCloudSvc.GetAccount(ctx.Request.Context(), uint(accountID))
	if err != nil || acc == nil {
		ctx.String(http.StatusNotFound, "account not found")
		return
	}
	mode := ctx.Query("hub.mode")
	token := ctx.Query("hub.verify_token")
	challenge := ctx.Query("hub.challenge")
	if mode != "subscribe" || token != acc.VerifyToken {
		ctx.String(http.StatusForbidden, "verification failed")
		return
	}
	ctx.String(http.StatusOK, challenge)
}

// DingTalkVerify 钉钉回调 URL 验证（GET）
// 路由: GET /api/webhook/dingtalk/{account_id}?signature=...&timestamp=...&nonce=...&echostr=...
// 钉钉对配置的回调地址发起 GET，携带 signature/timestamp/nonce/echostr。
// 本地用 token 计算 signature 比对；一致则回显 echostr（配置了 AESKey 时先解密）。
func (c *WebhookController) DingTalkVerify(ctx *gin.Context) {
	accountIDStr := ctx.Param("account_id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		ctx.String(http.StatusBadRequest, "invalid account_id")
		return
	}
	if c.dtAppSvc == nil {
		ctx.String(http.StatusServiceUnavailable, "dingtalk app service not configured")
		return
	}
	signature := ctx.Query("signature")
	timestamp := ctx.Query("timestamp")
	nonce := ctx.Query("nonce")
	echostr := ctx.Query("echostr")
	if signature == "" || timestamp == "" || nonce == "" || echostr == "" {
		ctx.String(http.StatusBadRequest, "missing signature params")
		return
	}
	plain, err := c.dtAppSvc.VerifyCallback(ctx.Request.Context(), uint(accountID), signature, timestamp, nonce, echostr)
	if err != nil {
		ctx.String(http.StatusUnauthorized, "verify failed: "+err.Error())
		return
	}
	ctx.String(http.StatusOK, plain)
}

// DingTalkReceive 钉钉回调收消息（POST）
// 路由: POST /api/webhook/dingtalk/{account_id}
// 请求体为 {"encrypt":"..."}（配置 AESKey 时）或明文消息 JSON。
// 解析后入消息中台并经统一 AI 派发管线触发智能体。
func (c *WebhookController) DingTalkReceive(ctx *gin.Context) {
	accountIDStr := ctx.Param("account_id")
	accountID, err := strconv.ParseUint(accountIDStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"accepted": false, "reason": "invalid account_id"})
		return
	}
	if c.dtAppSvc == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"accepted": false, "reason": "dingtalk app service not configured"})
		return
	}
	body, err := service.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"accepted": false, "reason": "read body: " + err.Error()})
		return
	}
	if err := c.dtAppSvc.ReceiveMessage(ctx.Request.Context(), uint(accountID), body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"accepted": false, "reason": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"accepted": true})
}

// Stats 统计
func (c *WebhookController) Stats(ctx *gin.Context) {
	pending := c.svc.PendingCount(context.Background())
	queueLen := c.svc.QueueLen(context.Background())
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
		c.svc.SetSalesEngine(context.Background(), e)
	}
}

// SetSmartOrchestrator 注入智能体统一编排器（在 router 中调用）
// 注入后 Webhook 入站消息会走 SmartCSOrchestrator.HandleIncoming 9 步编排
func (c *WebhookController) SetSmartOrchestrator(o *service.SmartCSOrchestrator) {
	if c.svc != nil {
		c.svc.SetSmartOrchestrator(context.Background(), o)
	}
}

// SetAgentBindingService 注入渠道智能体绑定服务（在 router 中调用）
// 注入后 triggerSalesEngine/triggerSmartOrchestrator 会先按 (channel_type, account_id)
// 加载绑定的智能体上下文，再调用 SalesEngine.HandleWithAgent 按智能体配置执行
func (c *WebhookController) SetAgentBindingService(svc *service.ChannelAgentBindingService) {
	if c.svc != nil {
		c.svc.SetAgentBindingService(context.Background(), svc)
	}
}
