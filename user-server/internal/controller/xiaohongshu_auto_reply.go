package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"marketing/internal/aiagent/agent/browser"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

type XiaohongshuAutoReplyController struct {
	svc		*service.XiaohongshuAutoReplyService
	manager		*browser.AutoReplyManager
	infra		*browser.AutoReplyInfra
	ragStack	*knowledgesvc.RAGStack
}

func NewXiaohongshuAutoReplyController(svc *service.XiaohongshuAutoReplyService, ragStack *knowledgesvc.RAGStack) *XiaohongshuAutoReplyController {
	return &XiaohongshuAutoReplyController{
		svc:		svc,
		manager:	GetAutoReplyManager(),
		infra:		browser.GetAutoReplyInfra(),
		ragStack:	ragStack,
	}
}

type xhsAccountReq struct {
	Username	string	`json:"username"`
	Cookie		string	`json:"cookie"`
	Headless	*bool	`json:"headless,omitempty"`
}

type xhsRuleReq struct {
	Keywords	string	`json:"keywords"`
	ReplyContent	string	`json:"reply_content"`
	Frequency	int	`json:"frequency"`
	DailyLimit	int	`json:"daily_limit"`
	IsActive	bool	`json:"is_active"`
}

func (c *XiaohongshuAutoReplyController) extractUserID(ctx *gin.Context) uint {
	v, exists := ctx.Get("user_id")
	if !exists {
		return 0
	}
	if id, ok := v.(uint); ok {
		return id
	}
	return 0
}

func (c *XiaohongshuAutoReplyController) StartLogin(ctx *gin.Context) {
	var req struct {
		Username	string	`json:"username"`
		Headless	*bool	`json:"headless,omitempty"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	userID := c.extractUserID(ctx, )
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	headless := false
	if req.Headless != nil {
		headless = *req.Headless
	}
	now := time.Now()
	item := &model.AutoReplyAccount{
		UserID:	userID, Platform: "xiaohongshu", Username: req.Username,
		IsActive:	true, Headless: headless, LoginAt: &now,
	}
	if err := c.svc.UpsertAccount(context.Background(), item); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	c.svc.StartLoginBrowser(context.Background(), userID, req.Username, item.ID, headless)
	response.Success(ctx, gin.H{"started": true, "accountId": item.ID}, "ok")
}

func (c *XiaohongshuAutoReplyController) LoginStatus(ctx *gin.Context) {
	username := ctx.Query("username")
	userID := c.extractUserID(ctx, )
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}
	items, _ := c.svc.ListAccounts(context.Background(), userID)
	status := "waiting"
	var accountID uint
	var cookie string
	for _, a := range items {
		if a.Username == username {
			accountID = a.ID
			if strings.Contains(a.Cookie, "web_session=") {
				status = "logged_in"
				cookie = a.Cookie
			}
			break
		}
	}
	response.Success(ctx, gin.H{"status": status, "accountId": accountID, "cookie": cookie}, "ok")
}

func (c *XiaohongshuAutoReplyController) ListAccounts(ctx *gin.Context) {
	userID := c.extractUserID(ctx, )
	items, _ := c.svc.ListAccounts(context.Background(), userID)
	response.Success(ctx, gin.H{"list": items}, "ok")
}

func (c *XiaohongshuAutoReplyController) UpsertAccount(ctx *gin.Context) {
	var req xhsAccountReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	userID := c.extractUserID(ctx, )
	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}
	now := time.Now()
	item := &model.AutoReplyAccount{
		UserID:	userID, Platform: "xiaohongshu", Username: req.Username,
		Cookie:	req.Cookie, IsActive: true, Headless: headless, LoginAt: &now,
	}
	if err := c.svc.UpsertAccount(context.Background(), item); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": item.ID}, "ok")
}

func (c *XiaohongshuAutoReplyController) SaveCookies(ctx *gin.Context) {
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
	userID := c.extractUserID(ctx, )
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

func (c *XiaohongshuAutoReplyController) DeleteAccount(ctx *gin.Context) {
	idStr := ctx.Param("id")
	// R8 修复：校验 :id 为正整数
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		response.Error(ctx, 400, "id 参数非法")
		return
	}
	userID := c.extractUserID(ctx, )
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

func (c *XiaohongshuAutoReplyController) GetRule(ctx *gin.Context) {
	userID := c.extractUserID(ctx, )
	rule, err := c.svc.GetRule(context.Background(), userID)
	if err != nil {
		response.Success(ctx, gin.H{"rule": nil}, "ok")
		return
	}
	response.Success(ctx, gin.H{"rule": rule}, "ok")
}

func (c *XiaohongshuAutoReplyController) SaveRule(ctx *gin.Context) {
	var req xhsRuleReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	userID := c.extractUserID(ctx, )
	rule := &model.AutoReplyRule{
		UserID:	userID, Platform: "xiaohongshu", Keywords: req.Keywords,
		ReplyContent:	req.ReplyContent, Frequency: req.Frequency,
		DailyLimit:	req.DailyLimit, IsActive: req.IsActive,
	}
	if err := c.svc.SaveRule(context.Background(), rule); err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"ok": true}, "ok")
}

func (c *XiaohongshuAutoReplyController) ListLogs(ctx *gin.Context) {
	userID := c.extractUserID(ctx, )
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

func (c *XiaohongshuAutoReplyController) Start(ctx *gin.Context) {
	platform := "xiaohongshu"
	userID := c.extractUserID(ctx, )
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	accounts, err := c.svc.ListAccounts(context.Background(), userID)
	if err != nil || len(accounts) == 0 {
		response.Error(ctx, 404, "未找到可用的小红书账号，请先绑定")
		return
	}

	account := accounts[0]
	bp := browser.Platform(platform)
	c.manager.SetHeadless(platform, account.Headless)

	if err := c.manager.StartBot(bp, account.Username, account.ID, account.Cookie); err != nil {
		HandleServiceError(ctx, err)
		return
	}

	bot, err := c.manager.GetBot(bp)
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

	if err := bot.Start(c.svc, userID); err != nil {
		HandleServiceError(ctx, err)
		return
	}

	response.Success(ctx, gin.H{
		"started":	true,
		"platform":	platform,
		"account_id":	account.ID,
		"headless":	account.Headless,
	}, "小红书机器人启动成功")
}

func (c *XiaohongshuAutoReplyController) Stop(ctx *gin.Context) {
	bp := browser.Platform("xiaohongshu")
	if err := c.manager.StopBot(bp); err != nil {
		logger.Errorf("[小红书] 停止失败: %v", err)
	}
	response.Success(ctx, gin.H{"stopped": true, "platform": "xiaohongshu"}, "ok")
}

func (c *XiaohongshuAutoReplyController) Health(ctx *gin.Context) {
	platform := "xiaohongshu"
	bot, botErr := c.manager.GetBot(browser.Platform(platform))
	status := gin.H{
		"platform":	platform,
		"bot_running":	botErr == nil && bot.IsRunning(),
		"rate_limit":	c.infra.RateLimiter.Stats("xiaohongshu_" + platform),
	}
	if botErr == nil && bot != nil {
		status["headless"] = bot.IsHeadless()
	}
	response.Success(ctx, status, "ok")
}
