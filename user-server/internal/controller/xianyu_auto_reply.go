package controller

import (
	"context"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type XianyuAutoReplyController struct {
	svc *service.XianyuAutoReplyService
}

func NewXianyuAutoReplyController(svc *service.XianyuAutoReplyService) *XianyuAutoReplyController {
	// 测试场景下 svc 为 nil 时自动构造默认服务（依赖全局 DB，由 SetTestDB 设置）
	if svc == nil {
		svc = service.NewXianyuAutoReplyService(nil)
	}
	return &XianyuAutoReplyController{svc: svc}
}

type xianyuUpsertAccountReq struct {
	Username string `json:"username"`
	Cookie   string `json:"cookie"`
	Headless *bool  `json:"headless,omitempty"`
}

type xianyuSaveRuleReq struct {
	Keywords     string `json:"keywords"`
	ReplyContent string `json:"reply_content"`
	Frequency    int    `json:"frequency"`
	DailyLimit   int    `json:"daily_limit"`
	IsActive     bool   `json:"is_active"`
}

// 启动登录流程：创建账号记录并打开浏览器到闲鱼页面，登录后自动保存 Cookie
func (c *XianyuAutoReplyController) StartLogin(ctx *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Headless *bool  `json:"headless,omitempty"` // 无头模式，可选参数
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	// 设置默认无头模式为false（可视化模式）
	headless := false
	if req.Headless != nil {
		headless = *req.Headless
	}

	now := time.Now()
	accountID, err := c.svc.UpsertAccountDTO(context.Background(), service.XianyuAccountCreateReq{
		UserID: userID, Username: req.Username, IsActive: true, Headless: headless, LoginAt: &now,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	// 启动浏览器登录流程（后台异步）
	c.svc.StartLoginBrowser(context.Background(), userID, req.Username, accountID, headless)
	response.Success(ctx, gin.H{"started": true, "accountId": accountID, "headless": headless}, "ok")
}

// 查询登录状态：根据是否存在 Cookie 判定，并返回 Cookie（如已获取）
func (c *XianyuAutoReplyController) LoginStatus(ctx *gin.Context) {
	username := ctx.Query("username")

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	items, err := c.svc.ListAccounts(context.Background(), userID)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	status := "waiting"
	var accountId uint
	var cookie string
	for _, a := range items {
		if a.Username == username {
			accountId = a.ID
			if a.Cookie != "" {
				// 闲鱼平台关键Cookie严格判定（仅登录后出现的Cookie）
				// 优先检测 xianyu_sid cookie值
				if strings.Contains(a.Cookie, "xianyu_sid=") {
					status = "logged_in"
					cookie = a.Cookie
				}
				// 备选检测 session_token cookie值
				if strings.Contains(a.Cookie, "session_token=") {
					status = "logged_in"
					cookie = a.Cookie
				}
			}
			break
		}
	}
	response.Success(ctx, gin.H{"status": status, "accountId": accountId, "cookie": cookie}, "ok")
}

func (c *XianyuAutoReplyController) ListAccounts(ctx *gin.Context) {
	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	items, err := c.svc.ListAccounts(context.Background(), userID)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"list": items}, "ok")
}

func (c *XianyuAutoReplyController) UpsertAccount(ctx *gin.Context) {
	var req xianyuUpsertAccountReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	loginAt := time.Now()

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	// 设置默认无头模式为true（后台运行）
	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	accountID, err := c.svc.UpsertAccountDTO(context.Background(), service.XianyuAccountCreateReq{
		UserID: userID, Username: req.Username, Cookie: req.Cookie, IsActive: true, Headless: headless, LoginAt: &loginAt,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": accountID, "headless": headless}, "ok")
}

func (c *XianyuAutoReplyController) SaveCookies(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var payload struct {
		Cookie string `json:"cookie"`
	}
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	// 校验 :id 为正整数 + 透传 userID 做 IDOR 防护
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		response.Error(ctx, 400, "id 参数非法")
		return
	}
	userID := getUserIDFromContext(ctx)
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}
	if err := c.svc.SaveCookies(context.Background(), uint(id64), payload.Cookie, userID); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": id64}, "ok")
}

func (c *XianyuAutoReplyController) DeleteAccount(ctx *gin.Context) {
	idStr := ctx.Param("id")
	// 校验 :id 为正整数
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		response.Error(ctx, 400, "id 参数非法")
		return
	}

	userID := getUserIDFromContext(ctx)
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}
	if err := c.svc.DeleteAccount(context.Background(), uint(id64), userID); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": id64}, "ok")
}

func (c *XianyuAutoReplyController) GetRule(ctx *gin.Context) {
	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	rule, err := c.svc.GetRule(context.Background(), userID)
	if err != nil {
		response.Success(ctx, gin.H{"rule": nil}, "ok")
		return
	}
	response.Success(ctx, gin.H{"rule": rule}, "ok")
}

func (c *XianyuAutoReplyController) SaveRule(ctx *gin.Context) {
	var req xianyuSaveRuleReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	err := c.svc.SaveRuleDTO(context.Background(), service.XianyuRuleSaveReq{
		UserID: userID, Keywords: req.Keywords, ReplyContent: req.ReplyContent,
		Frequency: req.Frequency, DailyLimit: req.DailyLimit, IsActive: req.IsActive,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"ok": true}, "ok")
}

func (c *XianyuAutoReplyController) ListLogs(ctx *gin.Context) {
	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID uint
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	// 类型断言获取uint类型的用户ID
	if id, ok := userIDInterface.(uint); ok {
		userID = id
	}
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := c.svc.ListRecentLogs(context.Background(), userID, page, pageSize)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"list": items, "total": total, "page": page, "page_size": pageSize}, "ok")
}

func (c *XianyuAutoReplyController) Start(ctx *gin.Context) {
	platform := "xianyu"
	userID := c.extractUserID(ctx)
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	accounts, err := c.svc.ListAccounts(context.Background(), userID)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	if len(accounts) == 0 {
		response.Error(ctx, 404, "未找到可用的闲鱼账号，请先绑定账号")
		return
	}

	account := accounts[0]

	// 启动编排下沉 service：装配 bot + RAG handler → WS 模式（失败降级轮询）
	useWS, err := c.svc.StartXianyuBot(context.Background(), account, userID)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{
		"started":    true,
		"platform":   platform,
		"account_id": account.ID,
		"headless":   account.Headless,
		"ws_mode":    useWS,
	}, "闲鱼机器人启动成功")
}

func (c *XianyuAutoReplyController) Stop(ctx *gin.Context) {
	platform := "xianyu"
	if err := service.GetAutoReplyBotManager().StopBot(platform); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{
		"stopped":  true,
		"platform": platform,
	}, "闲鱼机器人停止成功")
}

// extractUserID 从 gin context 提取 userID
func (c *XianyuAutoReplyController) extractUserID(ctx *gin.Context) uint {
	v, exists := ctx.Get("user_id")
	if !exists {
		return 0
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

// Health 返回自动回复服务健康状态
func (c *XianyuAutoReplyController) Health(ctx *gin.Context) {
	platform := "xianyu"
	mgr := service.GetAutoReplyBotManager()
	botStatus := mgr.GetBotStatus(platform)
	rateKey := "xianyu_" + platform

	status := gin.H{
		"platform":    platform,
		"bot_running": botStatus.Running,
		"rate_limit":  mgr.RateLimitStats(rateKey),
		"headless":    botStatus.Headless,
	}

	response.Success(ctx, status, "ok")
}
