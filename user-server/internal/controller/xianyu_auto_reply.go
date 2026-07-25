package controller

import (
	"context"
	"marketing/internal/aiagent/agent/browser"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ragStack 由 router 在启动时通过 SetRAGStack 注入；为 nil 时（如测试场景）
// 调用方应避免触发依赖 RAGStack 的 Start 流程。
var ragStack *knowledgesvc.RAGStack

// SetRAGStack 注入全局 RAGStack 实例（由 router 调用，避免 controller 直连 db）
func SetRAGStack(s *knowledgesvc.RAGStack) {
	ragStack = s
}

// getRAGStack 返回 router 注入的 RAGStack 实例
func getRAGStack() *knowledgesvc.RAGStack {
	return ragStack
}

type XianyuAutoReplyController struct {
	svc      *service.XianyuAutoReplyService
	manager  *browser.AutoReplyManager
	infra    *browser.AutoReplyInfra
	ragStack *knowledgesvc.RAGStack
}

func NewXianyuAutoReplyController(svc *service.XianyuAutoReplyService, ragStack *knowledgesvc.RAGStack) *XianyuAutoReplyController {
	// 测试场景下 svc 为 nil 时自动构造默认服务（依赖全局 DB，由 SetTestDB 设置）
	if svc == nil {
		svc = service.NewXianyuAutoReplyService(nil)
	}
	return &XianyuAutoReplyController{
		svc:      svc,
		manager:  GetAutoReplyManager(),
		infra:    browser.GetAutoReplyInfra(),
		ragStack: ragStack,
	}
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

// 启动登录流程：创建账号记录并打开浏览器到咸鱼页面，登录后自动保存 Cookie
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
	item := &model.AutoReplyAccount{UserID: userID, Platform: "xianyu", Username: req.Username, IsActive: true, Headless: headless, LoginAt: &now}
	if err := c.svc.UpsertAccount(context.Background(), item); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	// 启动浏览器登录流程（后台异步）
	c.svc.StartLoginBrowser(context.Background(), userID, req.Username, item.ID, headless)
	response.Success(ctx, gin.H{"started": true, "accountId": item.ID, "headless": headless}, "ok")
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
				// 咸鱼平台关键Cookie严格判定（仅登录后出现的Cookie）
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

	item := &model.AutoReplyAccount{UserID: userID, Platform: "xianyu", Username: req.Username, Cookie: req.Cookie, IsActive: true, Headless: headless, LoginAt: &loginAt}
	if err := c.svc.UpsertAccount(context.Background(), item); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": item.ID, "headless": headless}, "ok")
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
	// R8 修复：校验 :id 为正整数 + 透传 userID 做 IDOR 防护
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
	// R8 修复：校验 :id 为正整数
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
	err := c.svc.SaveRule(context.Background(), &model.AutoReplyRule{UserID: userID, Platform: "xianyu", Keywords: req.Keywords, ReplyContent: req.ReplyContent, Frequency: req.Frequency, DailyLimit: req.DailyLimit, IsActive: req.IsActive})
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
	browserPlatform := browser.Platform(platform)

	// 确定是否使用 WS 模式
	useWS := account.WsMode
	headless := account.Headless

	c.manager.SetHeadless(platform, headless)

	if err := c.manager.StartBot(browserPlatform, account.Username, account.ID, account.Cookie); err != nil {
		HandleServiceError(ctx, err)
		return
	}

	bot, err := c.manager.GetBot(browserPlatform)
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}

	dedup := browser.NewInMemoryDedup(5 * time.Minute)
	bot.SetDedup(dedup)
	bot.SetReplyHandler(browser.NewIntegrationReplyHandler(
		c.ragStack.Integration,
		c.ragStack.Customer,
		c.ragStack.Retrieval,
		c.svc,
	))

	if useWS {
		if err := c.svc.StartWSBot(context.Background(), bot, c.svc, userID, c.infra.RateLimiter, c.infra.SliderSolver); err != nil {
			logger.Warnf("[闲鱼] WS 模式启动失败，降级为轮询: %v", err)
			bot.Start(c.svc, userID)
			useWS = false
		} else {
			// 记录 WS 连接成功时间（下沉到 service，controller 不直连 DB）
			if err := c.svc.MarkWSConnected(context.Background(), account.ID); err != nil {
				logger.Errorf("[闲鱼] 记录 WS 连接时间失败: %v", err)
			}
		}
	}
	response.Success(ctx, gin.H{
		"started":    true,
		"platform":   platform,
		"account_id": account.ID,
		"headless":   headless,
		"ws_mode":    useWS,
	}, "闲鱼机器人启动成功")
}

func (c *XianyuAutoReplyController) Stop(ctx *gin.Context) {
	platform := "xianyu"
	browserPlatform := browser.Platform(platform)
	if err := c.manager.StopBot(browserPlatform); err != nil {
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
	bot, botErr := c.manager.GetBot(browser.Platform(platform))
	rateKey := "xianyu_" + platform

	status := gin.H{
		"platform":    platform,
		"bot_running": botErr == nil && bot.IsRunning(),
		"rate_limit":  c.infra.RateLimiter.Stats(rateKey),
	}

	if botErr == nil && bot != nil {
		status["headless"] = bot.IsHeadless()
	}

	response.Success(ctx, status, "ok")
}
