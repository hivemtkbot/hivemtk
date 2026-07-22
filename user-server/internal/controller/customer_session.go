package controller

import (
	"marketing/internal/model"
	dbutil "marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CustomerSessionController 客服会话控制器
type CustomerSessionController struct {
	sessionService *service.CustomerSessionService
	agentService   *service.AgentStatusService
}

// NewCustomerSessionController 创建客服会话控制器实例
func NewCustomerSessionController() *CustomerSessionController {
	return &CustomerSessionController{
		sessionService: service.NewCustomerSessionService(),
		agentService:   service.NewAgentStatusService(),
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

	sessions, total, err := c.sessionService.GetSessions(status, page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
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

	session, err := c.sessionService.GetSessionByID(uint(id))
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

	session, err := c.sessionService.CreateSession(&req)
	if HandleDBError(ctx, err, "创建会话") {
		return
	}

	response.Success(ctx, session, "创建成功")
}

// AssignSession 分配会话
func (c *CustomerSessionController) AssignSession(ctx *gin.Context) {
	var req service.AssignSessionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.AssignSession(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
}

// AutoAssignSession 自动分配会话
func (c *CustomerSessionController) AutoAssignSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	if err := c.sessionService.AutoAssign(uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "分配成功")
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
		session, err := c.sessionService.GetSessionByID(uint(id))
		if HandleServiceError(ctx, err) {
			return
		}
		sessionID = session.SessionID
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "50"))

	messages, total, err := c.sessionService.GetMessages(sessionID, page, pageSize)
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
		// 2026-07-21 修复：必须显式设置 SenderType = "agent"，否则 service 拿到空值会在
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
			session, err := c.sessionService.GetSessionByID(uint(id))
			if err != nil {
				response.Error(ctx, http.StatusNotFound, "会话不存在")
				return
			}
			sessionID = session.SessionID
		}
		req.SessionID = sessionID
	}

	message, err := c.sessionService.SendMessage(&req)
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

	if err := c.sessionService.UpdateSessionStatus(uint(id), req.Status); err != nil {
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

	if err := c.sessionService.RateSession(uint(id), req.Rating, req.Comment); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "评价成功")
}

// TransferSession 转接会话
func (c *CustomerSessionController) TransferSession(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}

	var req struct {
		NewAgentID uint `json:"new_agent_id" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.sessionService.TransferSession(uint(id), req.NewAgentID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "转接成功")
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

	if err := c.sessionService.TagSession(uint(id), req.Tags); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "标记成功")
}

// Takeover 坐席接管 AI 会话
//
//	POST /api/customer-sessions/:id/takeover
//	body: {"reason": "AI 答非所问"}
func (c *CustomerSessionController) Takeover(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = ctx.ShouldBindJSON(&req)

	// agent_id 从登录态推导（与 SendMessage 一致，禁止客户端伪造）
	agentID := getUserIDFromContext(ctx)
	if agentID == 0 {
		response.Error(ctx, http.StatusUnauthorized, "未登录或无权操作", "missing user_id")
		return
	}
	if err := c.sessionService.TakeoverByAgent(ctx.Request.Context(), &service.TakeoverRequest{
		SessionID: uint(id),
		AgentID:   agentID,
		Reason:    req.Reason,
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"session_id":   id,
		"handler_type": "human",
	}, "接管成功")
}

// Release 坐席释放会话回 AI
//
//	POST /api/customer-sessions/:id/release
func (c *CustomerSessionController) Release(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}
	agentID := getUserIDFromContext(ctx)
	if agentID == 0 {
		response.Error(ctx, http.StatusUnauthorized, "未登录或无权操作", "missing user_id")
		return
	}
	if err := c.sessionService.ReleaseToAI(ctx.Request.Context(), &service.ReleaseToAIRequest{
		SessionID: uint(id),
		AgentID:   agentID,
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"session_id":   id,
		"handler_type": "ai",
	}, "释放成功")
}

// SwitchHandler 统一 AI/人工切换接口
//
//	POST /api/customer-sessions/:id/switch-handler
//	body: {"handler_type": "human" | "ai", "reason": "..."}
//
// 传 handler_type=human → 等价 Takeover；handler_type=ai → 等价 Release。
// 前端只需要一个按钮调一个接口，根据当前 handler_type 反向选择目标。
func (c *CustomerSessionController) SwitchHandler(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}
	var req struct {
		HandlerType string `json:"handler_type" binding:"required,oneof=ai human"`
		Reason      string `json:"reason"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	agentID := getUserIDFromContext(ctx)
	if req.HandlerType == "human" && agentID == 0 {
		response.Error(ctx, http.StatusUnauthorized, "切人工时必须登录", "missing user_id")
		return
	}
	if err := c.sessionService.SwitchHandler(ctx.Request.Context(), &service.SwitchHandlerRequest{
		SessionID:   uint(id),
		AgentID:     agentID,
		HandlerType: model.HandlerType(req.HandlerType),
		Reason:      req.Reason,
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"session_id":   id,
		"handler_type": req.HandlerType,
	}, "切换成功")
}

// Blacklist 拉黑当前会话对应的访客
//
//	POST /api/customer-sessions/:id/blacklist
//	body: {"reason": "辱骂客服", "ttl_hours": 0}
func (c *CustomerSessionController) Blacklist(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的会话ID")
		return
	}
	var req struct {
		Reason   string `json:"reason"`
		TTLHours int    `json:"ttl_hours"`
	}
	_ = ctx.ShouldBindJSON(&req)

	agentID := getUserIDFromContext(ctx)
	operatorName := ""
	if name, ok := ctx.Get("username"); ok {
		if s, ok := name.(string); ok {
			operatorName = s
		}
	}

	if err := c.sessionService.BlacklistUser(&service.BlacklistRequest{
		SessionID:    uint(id),
		Reason:       req.Reason,
		OperatorID:   agentID,
		OperatorName: operatorName,
		TTLHours:     req.TTLHours,
	}); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"session_id":  id,
		"blacklisted": true,
	}, "拉黑成功")
}

// Unblacklist 解除拉黑
//
//	POST /api/customer-sessions/blacklist/remove
//	body: {"user_id": "u_123", "platform": "web"}
//
// 鉴权：要求登录态（JWT 中间件已保证），但操作者必须存在（agentID > 0），
// 避免未登录态/匿名 token 误调。
func (c *CustomerSessionController) Unblacklist(ctx *gin.Context) {
	var req struct {
		UserID   string         `json:"user_id" binding:"required"`
		Platform model.Platform `json:"platform"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}
	if getUserIDFromContext(ctx) == 0 {
		response.Error(ctx, http.StatusUnauthorized, "未登录或无权操作", "missing user_id")
		return
	}
	if err := c.sessionService.UnblacklistUser(req.UserID, req.Platform); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(ctx, nil, "解除拉黑成功")
}

// IsUserBlacklisted 判断访客是否在黑名单
//
//	GET /api/customer-sessions/blacklist/check?user_id=u_123&platform=web
func (c *CustomerSessionController) IsUserBlacklisted(ctx *gin.Context) {
	userID := ctx.Query("user_id")
	if userID == "" {
		response.Error(ctx, http.StatusBadRequest, "user_id 必填")
		return
	}
	platform := model.Platform(ctx.DefaultQuery("platform", "web"))
	ok, err := c.sessionService.IsUserBlacklisted(userID, platform)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{"blacklisted": ok}, "查询成功")
}

// ListBlacklist 分页查询生效中的黑名单
//
//	GET /api/customer-sessions/blacklist?page=1&page_size=20
func (c *CustomerSessionController) ListBlacklist(ctx *gin.Context) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))
	rows, total, err := c.sessionService.ListBlacklist(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(ctx, gin.H{
		"list":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
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

	sessions, err := c.sessionService.GetPendingSessions(page, pageSize)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取待处理会话失败："+err.Error())
		return
	}

	total, _ := c.sessionService.CountPendingSessions()

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

	if err := c.sessionService.UpdateSessionStatus(uint(id), model.SessionStatusClosed); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "关闭成功")
}

// AgentStatusController 客服状态控制器
type AgentStatusController struct {
	agentService    *service.AgentStatusService
	customerService *service.CustomerServiceAgentService
}

// NewAgentStatusController 创建客服状态控制器实例
func NewAgentStatusController() *AgentStatusController {
	db := dbutil.GetDB()
	return &AgentStatusController{
		agentService:    service.NewAgentStatusService(),
		customerService: service.NewCustomerServiceAgentService(db, service.NewAIAgentService(db)),
	}
}

// CreateAgent 创建客服
func (c *AgentStatusController) CreateAgent(ctx *gin.Context) {
	var req service.CreateAgentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	agent, err := c.agentService.CreateAgent(&req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, agent, "创建成功")
}

// GetAgentStatus 获取客服状态
func (c *AgentStatusController) GetAgentStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的客服ID")
		return
	}

	agent, err := c.agentService.GetAgentStatus(uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, agent, "获取成功")
}

// GetOnlineAgents 获取在线客服列表
func (c *AgentStatusController) GetOnlineAgents(ctx *gin.Context) {
	agents, err := c.agentService.GetOnlineAgents()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, agents, "获取成功")
}

// GetMyAgent 获取当前登录用户对应的坐席身份（接入登录态，杜绝"列表首位猜测"）
// GET /api/agents/me
func (c *AgentStatusController) GetMyAgent(ctx *gin.Context) {
	uidVal, ok := ctx.Get("user_id")
	if !ok {
		response.Error(ctx, http.StatusUnauthorized, "未登录", "缺少 user_id")
		return
	}
	var userID uint
	switch v := uidVal.(type) {
	case uint:
		userID = v
	case float64:
		userID = uint(v)
	case int:
		userID = uint(v)
	default:
		response.Error(ctx, http.StatusUnauthorized, "无效的用户标识", "")
		return
	}
	nameVal, _ := ctx.Get("username")
	name, _ := nameVal.(string)
	if name == "" {
		name = "user_" + strconv.FormatUint(uint64(userID), 10)
	}
	st, err := c.customerService.GetOrCreateAgentStatusByUserID(userID, name)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取坐席身份失败", err.Error())
		return
	}
	response.Success(ctx, st, "获取成功")
}

// ListAllAgents 列出全部客服（监管控制台）
func (c *AgentStatusController) ListAllAgents(ctx *gin.Context) {
	agents, err := c.agentService.ListAllAgents()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, agents, "获取成功")
}

// UpdateAgentStatus 更新客服状态
func (c *AgentStatusController) UpdateAgentStatus(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的客服ID")
		return
	}

	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	if err := c.agentService.UpdateAgentStatus(uint(id), req.Status); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "更新成功")
}

// GoOnline 客服上线
func (c *AgentStatusController) GoOnline(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的客服ID")
		return
	}

	if err := c.agentService.GoOnline(uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "上线成功")
}

// GoOffline 客服下线
func (c *AgentStatusController) GoOffline(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的客服ID")
		return
	}

	if err := c.agentService.GoOffline(uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "下线成功")
}

// GetAgentSessions 获取客服的会话
func (c *AgentStatusController) GetAgentSessions(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的客服ID")
		return
	}

	sessions, err := c.agentService.GetAgentSessions(uint(id))
	if HandleDBError(ctx, err, "获取客服会话") {
		return
	}

	response.Success(ctx, sessions, "获取成功")
}

// QuickReplyController 快捷回复控制器
type QuickReplyController struct {
	replyService *service.QuickReplyService
}

// NewQuickReplyController 创建快捷回复控制器实例
func NewQuickReplyController() *QuickReplyController {
	return &QuickReplyController{
		replyService: service.NewQuickReplyService(),
	}
}

// GetReplies 获取快捷回复列表
func (c *QuickReplyController) GetReplies(ctx *gin.Context) {
	category := ctx.Query("category")
	replies, err := c.replyService.GetReplies(category)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, replies, "获取成功")
}

// CreateReply 创建快捷回复
func (c *QuickReplyController) CreateReply(ctx *gin.Context) {
	createdBy := getUserIDFromContext(ctx)

	var req service.CreateReplyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	reply, err := c.replyService.CreateReply(createdBy, &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, reply, "创建成功")
}

// UpdateReply 更新快捷回复
func (c *QuickReplyController) UpdateReply(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的快捷回复ID")
		return
	}

	var req service.CreateReplyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	reply, err := c.replyService.UpdateReply(uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, reply, "更新成功")
}

// DeleteReply 删除快捷回复
func (c *QuickReplyController) DeleteReply(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的快捷回复ID")
		return
	}

	if err := c.replyService.DeleteReply(uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// GetReplyCategories 获取快捷回复分类
func (c *QuickReplyController) GetReplyCategories(ctx *gin.Context) {
	categories, err := c.replyService.GetCategories()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, categories, "获取成功")
}

// SessionTagController 会话标签控制器
type SessionTagController struct {
	tagService *service.SessionTagService
}

// NewSessionTagController 创建会话标签控制器实例
func NewSessionTagController() *SessionTagController {
	return &SessionTagController{
		tagService: service.NewSessionTagService(),
	}
}

// GetTags 获取标签列表
func (c *SessionTagController) GetTags(ctx *gin.Context) {
	tags, err := c.tagService.GetTags()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, err.Error())
		return
	}

	response.Success(ctx, tags, "获取成功")
}

// CreateTag 创建标签
func (c *SessionTagController) CreateTag(ctx *gin.Context) {
	var req service.CreateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	tag, err := c.tagService.CreateTag(&req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, tag, "创建成功")
}

// UpdateTag 更新标签
func (c *SessionTagController) UpdateTag(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的标签ID")
		return
	}

	var req service.CreateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	tag, err := c.tagService.UpdateTag(uint(id), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, tag, "更新成功")
}

// DeleteTag 删除标签
func (c *SessionTagController) DeleteTag(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的标签ID")
		return
	}

	if err := c.tagService.DeleteTag(uint(id)); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// AISuggestionController AI建议控制器
type AISuggestionController struct {
	suggestionService *service.AISuggestionService
}

// NewAISuggestionController 创建AI建议控制器实例
func NewAISuggestionController() *AISuggestionController {
	return &AISuggestionController{
		suggestionService: service.NewAISuggestionService(),
	}
}

// GetSuggestions 获取AI建议
func (c *AISuggestionController) GetSuggestions(ctx *gin.Context) {
	sessionID := ctx.Param("session_id")
	suggestions, err := c.suggestionService.GetSuggestions(sessionID)
	if HandleDBError(ctx, err, "获取AI建议") {
		return
	}

	response.Success(ctx, suggestions, "获取成功")
}

// UseSuggestion 使用AI建议
func (c *AISuggestionController) UseSuggestion(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的建议ID")
		return
	}

	agentID := getUserIDFromContext(ctx)

	if err := c.suggestionService.UseSuggestion(uint(id), agentID); err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, nil, "使用成功")
}
