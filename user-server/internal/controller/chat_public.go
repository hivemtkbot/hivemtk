package controller

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"marketing/internal/middleware"
	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
)

// ChatPublicController 公开 chat API 控制器（访客端，无 JWT）
//
// 通过软解析 AppKeyResolve 鉴权后提供：
//   - 会话打开/续接
//   - 消息发送
//   - 历史消息查询
//   - 转人工 / 关闭 / 评分
//   - 离线消息拉取
//   - 拉取离线期间坐席/AI 回复（连接 WebSocket 时）
//
// 路由前缀：/api/chat/public
//
// 私域部署模式（2026-07-17 优化）：不再强制要求 X-Chat-App-Key Header。
// 渠道 ID 软解析顺序：ctx.chat_channel_id > body.channel_id > X-Chat-Channel-Id > 默认 "default"。
type ChatPublicController struct {
	visitorSvc *service.VisitorChatService
	channelSvc *service.ChatChannelService
}

// NewChatPublicController 构造控制器
func NewChatPublicController(visitorSvc *service.VisitorChatService, channelSvc *service.ChatChannelService) *ChatPublicController {
	return &ChatPublicController{visitorSvc: visitorSvc, channelSvc: channelSvc}
}

// GetChannelInfoByAppKey 根据 app_key 查询渠道公开信息（用于 widget 安装引导的连通性测试）
// GET /api/chat/public/channel/:app_key/info
func (ctrl *ChatPublicController) GetChannelInfoByAppKey(c *gin.Context) {
	appKey := c.Param("app_key")
	if appKey == "" {
		response.Error(c, http.StatusBadRequest, "app_key 不能为空")
		return
	}
	channel, err := ctrl.channelSvc.GetByAppKey(appKey)
	if err != nil {
		response.NotFoundError(c, "渠道")
		return
	}
	// 只返回公开字段（不暴露 AppSecretHash）
	c.JSON(http.StatusOK, gin.H{
		"code":    "SUCCESS",
		"message": "ok",
		"data": gin.H{
			"channel_id":           channel.ChannelID,
			"channel_name":         channel.ChannelName,
			"status":               channel.Status,
			"widget_title":         channel.WidgetTitle,
			"widget_color":         channel.WidgetColor,
			"widget_position":      channel.WidgetPosition,
			"welcome_message":      channel.WelcomeMessage,
			"auto_assign":          channel.AutoAssign,
			"confidence_threshold": channel.ConfidenceThreshold,
		},
	})
}

// resolveChannelID 软解析 channel_id：ctx > body > header > 默认
func resolveChannelID(c *gin.Context, reqBody *service.VisitorOpenSessionRequest) string {
	if v, ok := c.Get("chat_channel_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return middleware.GetChatChannelID(c)
}

// OpenSession 打开（或续接）会话
// POST /api/chat/public/sessions
func (ctrl *ChatPublicController) OpenSession(c *gin.Context) {
	var req service.VisitorOpenSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	// 渠道 ID 软解析（中间件已注入；如果缺失则用 body 里的）
	if req.ChannelID == "" {
		req.ChannelID = resolveChannelID(c, &req)
	}
	// 2026-07-18 私域部署修复：visitor_id 也走软解析，从 X-Chat-Visitor-Id header / query 兜底
	// 这样访客 SDK 只需发 header，不必在 body 里传 visitor_id。
	if req.VisitorID == "" {
		req.VisitorID = c.GetHeader("X-Chat-Visitor-Id")
		if req.VisitorID == "" {
			req.VisitorID = c.Query("visitor_id")
		}
	}

	result, err := ctrl.visitorSvc.OpenSession(&req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, result, "会话已打开")
}

// GetMessages 获取历史消息
// GET /api/chat/public/sessions/:session_id/messages
func (ctrl *ChatPublicController) GetMessages(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, "session_id 必填")
		return
	}
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" {
		response.Error(c, http.StatusBadRequest, "X-Chat-Visitor-Id 必填")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}

	messages, total, err := ctrl.visitorSvc.GetMessages(channelID, visitorID, sessionID, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.SuccessWithList(c, messages, total)
}

// SendMessage 发送访客消息
// POST /api/chat/public/sessions/:session_id/messages
func (ctrl *ChatPublicController) SendMessage(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, "session_id 必填")
		return
	}
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" {
		response.Error(c, http.StatusBadRequest, "X-Chat-Visitor-Id 必填")
		return
	}

	var body struct {
		Content     string `json:"content" binding:"required"`
		ContentType string `json:"content_type"`
		// 2026-07-17: 附件支持（访客直传七牛后带 CDN URL 发消息）
		MediaURL  string `json:"media_url"`
		MediaType string `json:"media_type"`
		MediaName string `json:"media_name"`
		MediaSize int64  `json:"media_size"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	req := &service.VisitorSendMessageRequest{
		ChannelID:   channelID,
		VisitorID:   visitorID,
		SessionID:   sessionID,
		Content:     body.Content,
		ContentType: body.ContentType,
		MediaURL:    body.MediaURL,
		MediaType:   body.MediaType,
		MediaName:   body.MediaName,
		MediaSize:   body.MediaSize,
	}
	result, err := ctrl.visitorSvc.SendMessage(req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, result, "消息已发送")
}

// GetActiveSession 获取访客最近活跃会话（用于离线消息续接）
// GET /api/chat/public/sessions/active
func (ctrl *ChatPublicController) GetActiveSession(c *gin.Context) {
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" {
		response.Error(c, http.StatusBadRequest, "X-Chat-Visitor-Id 必填")
		return
	}

	session, err := ctrl.visitorSvc.GetLatestActiveSession(channelID, visitorID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if session == nil {
		response.Success(c, nil, "无活跃会话")
		return
	}
	response.Success(c, session, "ok")
}

// GetRecentClosedSessions 获取最近 7 天已结束会话
// GET /api/chat/public/sessions/recent-closed
func (ctrl *ChatPublicController) GetRecentClosedSessions(c *gin.Context) {
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" {
		response.Error(c, http.StatusBadRequest, "X-Chat-Visitor-Id 必填")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	sessions, err := ctrl.visitorSvc.GetRecentClosedSessions(channelID, visitorID, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithList(c, sessions, int64(len(sessions)))
}

// GetOfflineMessages 拉取访客离线期间的坐席/AI 回复消息
// GET /api/chat/public/sessions/:session_id/offline-messages
// 用于访客重新连接 WebSocket 时，一次性拉取离线期间的所有回复。
// 标记消息为已投递（通过 message.delivered_at 字段）。
func (ctrl *ChatPublicController) GetOfflineMessages(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		response.Error(c, http.StatusBadRequest, "session_id 必填")
		return
	}
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" {
		response.Error(c, http.StatusBadRequest, "X-Chat-Visitor-Id 必填")
		return
	}

	messages, err := ctrl.visitorSvc.GetOfflineMessages(channelID, visitorID, sessionID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.SuccessWithList(c, messages, int64(len(messages)))
}

// RequestHumanTransfer 访客主动转人工
// POST /api/chat/public/sessions/:session_id/transfer
func (ctrl *ChatPublicController) RequestHumanTransfer(c *gin.Context) {
	sessionID := c.Param("session_id")
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" || sessionID == "" {
		response.Error(c, http.StatusBadRequest, "参数不完整")
		return
	}

	var body struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)

	if err := ctrl.visitorSvc.RequestHumanTransfer(channelID, visitorID, sessionID, body.Reason); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil, "已为您转接人工客服")
}

// CloseSession 访客关闭会话
// POST /api/chat/public/sessions/:session_id/close
func (ctrl *ChatPublicController) CloseSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" || sessionID == "" {
		response.Error(c, http.StatusBadRequest, "参数不完整")
		return
	}

	if err := ctrl.visitorSvc.CloseSession(channelID, visitorID, sessionID); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil, "会话已关闭")
}

// RateSession 访客评分
// POST /api/chat/public/sessions/:session_id/rate
func (ctrl *ChatPublicController) RateSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	channelID := resolveChannelID(c, nil)
	visitorID := c.GetHeader("X-Chat-Visitor-Id")
	if visitorID == "" {
		visitorID = c.Query("visitor_id")
	}
	if visitorID == "" || sessionID == "" {
		response.Error(c, http.StatusBadRequest, "参数不完整")
		return
	}

	var body struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, http.StatusBadRequest, "评分必须在 1-5 之间")
		return
	}

	if err := ctrl.visitorSvc.RateSession(channelID, visitorID, sessionID, body.Rating, body.Comment); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil, "感谢您的评价")
}

// CountAvailableAgents 公开可用坐席数（用于显示"当前 X 位客服在线"）
// GET /api/chat/public/agents/available
func (ctrl *ChatPublicController) CountAvailableAgents(c *gin.Context) {
	count, err := ctrl.visitorSvc.CountAvailableAgents()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"available": count}, "ok")
}

// ============================================================================
// 附件上传（2026-07-17）
// ============================================================================
//
// 设计：访客 → 后端拿上传 token → 访客直传七牛 → 拿到 CDN URL → 发消息带 media_url
//   - 不经过后端中转文件流，节省带宽
//   - 后端只签发带策略的 token（限定 key 前缀、有效期、文件大小）
//   - 上传地址由 config.yaml 的 storage.qiniu.upload_domain 配置
// ============================================================================

// GetUploadToken 生成七牛上传凭证
// GET /api/chat/public/upload-token?file_type=image&ext=jpg&size=102400
//
// 返回：
//   - upload_url: 七牛华东上传入口
//   - token: 上传凭证
//   - key: 预生成的文件 key（chat/yyyy/MM/<uuid>.ext）
//   - public_url: 上传后可直接访问的 CDN URL
//   - expires_in: token 有效期（秒）
func (ctrl *ChatPublicController) GetUploadToken(c *gin.Context) {
	cfg := config.GetAppConfig()
	if cfg.Storage.Type != "qiniu" || cfg.Storage.Qiniu.AccessKey == "" {
		response.Error(c, http.StatusServiceUnavailable, "对象存储未配置")
		return
	}

	fileType := c.DefaultQuery("file_type", "file")
	ext := c.DefaultQuery("ext", "")
	sizeStr := c.DefaultQuery("size", "0")
	size, _ := strconv.ParseInt(sizeStr, 10, 64)

	// 1. 限制文件大小（默认 20MB）
	const maxSize = 20 * 1024 * 1024
	if size > maxSize {
		response.Error(c, http.StatusRequestEntityTooLarge, "文件大小超出 20MB 限制")
		return
	}

	// 2. 限制文件类型
	allowedExts := map[string][]string{
		"image": {"jpg", "jpeg", "png", "gif", "webp"},
		"file":  {"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "zip", "rar"},
		"audio": {"mp3", "wav", "ogg", "m4a"},
		"video": {"mp4", "webm"},
	}
	extLower := strings.ToLower(ext)
	if ext == "" {
		response.Error(c, http.StatusBadRequest, "缺少 ext 参数")
		return
	}
	if list, ok := allowedExts[fileType]; ok {
		valid := false
		for _, e := range list {
			if e == extLower {
				valid = true
				break
			}
		}
		if !valid {
			response.Error(c, http.StatusBadRequest, "文件类型不被允许: "+ext)
			return
		}
	}

	// 3. 构造 key: chat/yyyy/MM/<uuid>.<ext>
	now := time.Now()
	uid := uuid.NewString()
	key := fmt.Sprintf("chat/%04d/%02d/%s.%s", now.Year(), now.Month(), uid, extLower)

	// 4. 构造上传策略（base64 url-encoded JSON）
	expires := now.Add(1 * time.Hour).Unix()
	policy := map[string]any{
		"scope":      cfg.Storage.Qiniu.Bucket + ":" + key,
		"deadline":   expires,
		"fsizeLimit": maxSize,
		"returnBody": `{"key":"$(key)","hash":"$(etag)","fsize":$(fsize),"fname":"$(fname)"}`,
	}
	policyBytes, _ := json.Marshal(policy)
	// 2026-07-17: 七牛 token 格式严格要求 AK:Signature:Policy（签名在前，策略在后）
	//   - 反例：AK:Policy:Signature → 七牛返回 BadToken
	//   - 参考：qiniu/go-sdk Mac.UploadToken 拼接顺序
	policyEncoded := base64.URLEncoding.EncodeToString(policyBytes)

	// 5. 用 SecretKey 对 policy 签名（HMAC-SHA1）
	mac := hmac.New(sha1.New, []byte(cfg.Storage.Qiniu.SecretKey))
	mac.Write([]byte(policyEncoded))
	signature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	// 七牛 token 拼接顺序：AccessKey : Signature : PolicyEncoded
	token := cfg.Storage.Qiniu.AccessKey + ":" + signature + ":" + policyEncoded

	// 6. 构造返回
	uploadDomain := cfg.Storage.Qiniu.UploadDomain
	if uploadDomain == "" {
		uploadDomain = "up-z2.qiniup.com"
	}
	uploadURL := fmt.Sprintf("https://%s", uploadDomain)
	// 2026-07-17: 返回完整 URL（含 https://），方便前端直接使用，避免遗漏协议
	publicURL := "https://" + strings.TrimSuffix(cfg.Storage.Qiniu.Domain, "/") + "/" + key

	response.Success(c, gin.H{
		"upload_url": uploadURL,
		"token":      token,
		"key":        key,
		"public_url": publicURL,
		"expires_in": 3600,
	}, "ok")
}
