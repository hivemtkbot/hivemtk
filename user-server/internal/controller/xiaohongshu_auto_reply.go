package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

type XiaohongshuAutoReplyController struct {
	svc *service.XiaohongshuAutoReplyService
}

func NewXiaohongshuAutoReplyController(svc *service.XiaohongshuAutoReplyService) *XiaohongshuAutoReplyController {
	return &XiaohongshuAutoReplyController{svc: svc}
}

type xhsAccountReq struct {
	Username string `json:"username"`
	Cookie   string `json:"cookie"`
	Headless *bool  `json:"headless,omitempty"`
}

type xhsRuleReq struct {
	Keywords     string `json:"keywords"`
	ReplyContent string `json:"reply_content"`
	Frequency    int    `json:"frequency"`
	DailyLimit   int    `json:"daily_limit"`
	IsActive     bool   `json:"is_active"`
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
		Username string `json:"username"`
		Headless *bool  `json:"headless,omitempty"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	userID := c.extractUserID(ctx)
	if userID == 0 {
		response.Error(ctx, 401, "用户未认证")
		return
	}

	headless := false
	if req.Headless != nil {
		headless = *req.Headless
	}
	now := time.Now()
	accountID, err := c.svc.UpsertAccountDTO(context.Background(), service.XiaohongshuAccountCreateReq{
		UserID: userID, Username: req.Username, IsActive: true, Headless: headless, LoginAt: &now,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	c.svc.StartLoginBrowser(context.Background(), userID, req.Username, accountID, headless)
	response.Success(ctx, gin.H{"started": true, "accountId": accountID}, "ok")
}

func (c *XiaohongshuAutoReplyController) LoginStatus(ctx *gin.Context) {
	username := ctx.Query("username")
	userID := c.extractUserID(ctx)
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
	userID := c.extractUserID(ctx)
	items, _ := c.svc.ListAccounts(context.Background(), userID)
	response.Success(ctx, gin.H{"list": items}, "ok")
}

func (c *XiaohongshuAutoReplyController) UpsertAccount(ctx *gin.Context) {
	var req xhsAccountReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	userID := c.extractUserID(ctx)
	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}
	now := time.Now()
	accountID, err := c.svc.UpsertAccountDTO(context.Background(), service.XiaohongshuAccountCreateReq{
		UserID: userID, Username: req.Username, Cookie: req.Cookie, IsActive: true, Headless: headless, LoginAt: &now,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"id": accountID}, "ok")
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
	// 校验 :id 为正整数 + 透传 userID 做 IDOR 防护
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		response.Error(ctx, 400, "id 参数非法")
		return
	}
	userID := c.extractUserID(ctx)
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
	// 校验 :id 为正整数
	id64, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id64 == 0 {
		response.Error(ctx, 400, "id 参数非法")
		return
	}
	userID := c.extractUserID(ctx)
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
	userID := c.extractUserID(ctx)
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
	userID := c.extractUserID(ctx)
	err := c.svc.SaveRuleDTO(context.Background(), service.XiaohongshuRuleSaveReq{
		UserID: userID, Keywords: req.Keywords, ReplyContent: req.ReplyContent,
		Frequency: req.Frequency, DailyLimit: req.DailyLimit, IsActive: req.IsActive,
	})
	if err != nil {
		HandleServiceError(ctx, err)
		return
	}
	response.Success(ctx, gin.H{"ok": true}, "ok")
}

func (c *XiaohongshuAutoReplyController) ListLogs(ctx *gin.Context) {
	userID := c.extractUserID(ctx)
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
	userID := c.extractUserID(ctx)
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

	// 启动编排下沉 service：装配 bot + RAG handler → 启动轮询
	if err := c.svc.StartXiaohongshuBot(context.Background(), account, userID); err != nil {
		HandleServiceError(ctx, err)
		return
	}

	response.Success(ctx, gin.H{
		"started":    true,
		"platform":   platform,
		"account_id": account.ID,
		"headless":   account.Headless,
	}, "小红书机器人启动成功")
}

func (c *XiaohongshuAutoReplyController) Stop(ctx *gin.Context) {
	if err := service.GetAutoReplyBotManager().StopBot("xiaohongshu"); err != nil {
		logger.Errorf("[小红书] 停止失败: %v", err)
	}
	response.Success(ctx, gin.H{"stopped": true, "platform": "xiaohongshu"}, "ok")
}

func (c *XiaohongshuAutoReplyController) Health(ctx *gin.Context) {
	platform := "xiaohongshu"
	mgr := service.GetAutoReplyBotManager()
	botStatus := mgr.GetBotStatus(platform)
	status := gin.H{
		"platform":    platform,
		"bot_running": botStatus.Running,
		"rate_limit":  mgr.RateLimitStats("xiaohongshu_" + platform),
		"headless":    botStatus.Headless,
	}
	response.Success(ctx, status, "ok")
}
