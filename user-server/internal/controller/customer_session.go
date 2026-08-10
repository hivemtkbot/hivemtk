package controller

import (
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ============================================================================
// CustomerSessionController 客服会话控制器（按子域拆分）
// ----------------------------------------------------------------------------
// 文件拆分（按子域）:
//   - customer_session.go            本文件:核心 CRUD + getUserIDFromContext
//   - customer_session_routing.go    路由:AssignSession / AutoAssign / Transfer
//   - customer_session_takeover.go   接管:Takeover / Release / SwitchHandler
//   - customer_session_blacklist.go  拉黑:Blacklist / Unblacklist / IsUserBlacklisted / ListActiveBlacklist
//   - agent_status_controller.go     客服状态:AgentStatusController
//   - ai_suggestion_controller.go    AI 建议:AISuggestionController
//   - quick_reply_controller.go      快捷回复:QuickReplyController
//   - session_tag_controller.go      会话标签:SessionTagController
//
// 文档:docs/企业级架构优化/坐席实时聊天看板.md
// ============================================================================

// CustomerSessionController 客服会话控制器
type CustomerSessionController struct {
	sessionService *service.CustomerSessionService
}

// NewCustomerSessionController 创建客服会话控制器实例
func NewCustomerSessionController() *CustomerSessionController {
	return &CustomerSessionController{
		sessionService: service.NewCustomerSessionService(),
	}
}

// getUserIDFromContext 从上下文中提取 user_id (JWT 存储为 uint，但部分路径可能使用 float64)
func getUserIDFromContext(ctx *gin.Context) uint {
	if uid, exists := ctx.Get("user_id"); exists {
		switch v := uid.(type) {
		case uint:
			return v
		case float64:
			return uint(v)
		case int:
			return uint(v)
		case int64:
			return uint(v)
		}
	}
	return 0
}

// GetSessions 获取会话列表
func (c *CustomerSessionController) GetSessions(ctx *gin.Context) {
	status := model.SessionStatus(ctx.Query("status"))
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	sessions, total, err := c.sessionService.GetSessions(ctx.Request.Context(), status, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      sessions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetSessionByID 获取会话详情
func (c *CustomerSessionController) GetSessionByID(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	session, err := c.sessionService.GetSessionByID(ctx.Request.Context(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, session, "获取成功")
}

// CreateSession 创建会话
func (c *CustomerSessionController) CreateSession(ctx *gin.Context) {
	var req service.CreateSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	session, err := c.sessionService.CreateSession(ctx.Request.Context(), &req)
	if HandleDBError(ctx, err, "创建会话") {
		return
	}

	response.Success(ctx, session, "创建成功")
}

// GetMessages 获取会话消息
func (c *CustomerSessionController) GetMessages(ctx *gin.Context) {
	// 读取 :id 参数（兼容数字主键 ID 和字符串 session_id）
	idStr := ctx.Param("id")
	if idStr == "" {
		// 兼容旧参数名 session_id
		idStr = ctx.Param("session_id")
	}

	// 尝试解析为数字主键 ID，若成功则通过 ID 查找会话获取字符串 session_id
	sessionID := idStr
	if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
		session, err := c.sessionService.GetSessionByID(ctx.Request.Context(), uint(id))
		if HandleServiceError(ctx, err) {
			return
		}
		sessionID = session.SessionID
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))

	messages, total, err := c.sessionService.GetMessages(ctx.Request.Context(), sessionID, page, pageSize)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, gin.H{
		"list":      messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// SendMessage 发送消息
func (c *CustomerSessionController) SendMessage(ctx *gin.Context) {
	var req service.SendMessageRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	// 安全加固：发送者身份必须从鉴权上下文派生，禁止客户端在请求体伪造 sender_type/sender_id。
	// 访客（仅 app_key 鉴权，上下文含 chat_channel_id）只能以 user 身份发言，杜绝冒充坐席。
	if channelID, ok := ctx.Get("chat_channel_id"); ok {
		req.SenderType = "user"
		switch v := channelID.(type) {
		case uint:
			req.SenderID = strconv.FormatUint(uint64(v), 10)
		case string:
			req.SenderID = v
		default:
			req.SenderID = ""
		}
		req.SenderName = ""
	} else if uid := getUserIDFromContext(ctx); uid != 0 {
		// 坐席/管理员：身份绑定到 JWT 主体，禁止冒充他人
		// 必须显式设置 SenderType = "agent"，否则 service 拿到空值会在
		// 会话统计更新处被当作 "user" 处理，导致 ai_reply_count 错乱 / session.handler_type
		// 不会切回 ai / human。
		req.SenderType = "agent"
		req.SenderID = strconv.FormatUint(uint64(uid), 10)
		if name, ok := ctx.Get("username"); ok {
			if s, ok := name.(string); ok {
				req.SenderName = s
			}
		}
	} else {
		// 既不是访客（chat_channel_id），也不是登录用户（user_id）——不允许发送
		response.Error(ctx, http.StatusUnauthorized, "未登录或无权访问", "missing auth context")
		return
	}

	// 如果 SenderType 仍为空（极少数异常路径），按 user 兜底，避免后续 service 层因空字符串 panic
	if req.SenderType == "" {
		req.SenderType = "user"
	}

	// 如果请求体未提供 session_id，则从 URL 参数 :id 解析
	if req.SessionID == "" {
		idStr := ctx.Param("id")
		if idStr == "" {
			response.Error(ctx, http.StatusBadRequest, "请求参数错误: session_id is required")
			return
		}
		// 兼容数字主键 ID 和字符串 session_id
		sessionID := idStr
		if id, err := strconv.ParseUint(idStr, 10, 32); err == nil {
			session, err := c.sessionService.GetSessionByID(ctx.Request.Context(), uint(id))
			if err != nil {
				response.Error(ctx, http.StatusNotFound, "会话不存在")
				return
			}
			sessionID = session.SessionID
		}
		req.SessionID = sessionID
	}

	message, err := c.sessionService.SendMessage(ctx.Request.Context(), &req)
	if HandleDBError(ctx, err, "发送消息") {
		return
	}

	response.Success(ctx, message, "发送成功")
}

// UpdateSessionStatus 更新会话状态
func (c *CustomerSessionController) UpdateSessionStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Status model.SessionStatus `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.UpdateSessionStatus(ctx.Request.Context(), uint(id), req.Status); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "更新成功")
}

// RateSession 评价会话
func (c *CustomerSessionController) RateSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Comment string `json:"comment"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.RateSession(ctx.Request.Context(), uint(id), req.Rating, req.Comment); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "评价成功")
}

// TagSession 标记会话
func (c *CustomerSessionController) TagSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		Tags []string `json:"tags" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.TagSession(ctx.Request.Context(), uint(id), req.Tags); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "标记成功")
}

// GetPendingSessions 获取待处理会话
func (c *CustomerSessionController) GetPendingSessions(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 20
	}

	sessions, err := c.sessionService.GetPendingSessions(ctx.Request.Context(), page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, "获取待处理会话失败："+err.Error())
		return
	}

	total, _ := c.sessionService.CountPendingSessions(ctx.Request.Context())

	response.Success(ctx, gin.H{
		"list":      sessions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// CloseSession 关闭会话
func (c *CustomerSessionController) CloseSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	if err := c.sessionService.UpdateSessionStatus(ctx.Request.Context(), uint(id), model.SessionStatusClosed); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "关闭成功")
}
