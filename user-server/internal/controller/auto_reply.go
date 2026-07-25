package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"marketing/internal/aiagent/agent/browser"
	knowledgesvc "marketing/internal/aiagent/knowledge/service"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/pagination"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/pkg/utils/urlguard"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// 辅助函数：获取无头模式描述
func getHeadlessDescription(headless bool) string {
	if headless {
		return "后台运行模式（无界面）"
	}
	return "可视化模式（显示浏览器）"
}

type AutoReplyController struct {
	svc      *service.AutoReplyService
	manager  *browser.AutoReplyManager // 自动回复管理器
	ragStack *knowledgesvc.RAGStack
}

// AutoReplyManagerSingleton 单例管理器
type AutoReplyManagerSingleton struct {
	manager *browser.AutoReplyManager
	once    sync.Once
}

var singleton *AutoReplyManagerSingleton

// GetAutoReplyManager 返回单例的自动回复管理器实例
func GetAutoReplyManager() *browser.AutoReplyManager {
	if singleton == nil {
		once := sync.Once{}
		once.Do(func() {
			if singleton == nil {
				singleton = &AutoReplyManagerSingleton{}
				singleton.init()
			}
		})
	}
	return singleton.GetManager()
}

// GetManager 获取管理器实例
func (s *AutoReplyManagerSingleton) GetManager() *browser.AutoReplyManager {
	if s.manager == nil {
		s.once.Do(func() {
			if s.manager == nil {
				s.init()
			}
		})
	}
	return s.manager
}

// init 初始化管理器
func (s *AutoReplyManagerSingleton) init() {
	// 默认所有平台都使用无头模式
	defaultHeadless := map[string]bool{
		"douyin":      true,
		"kuaishou":    true,
		"xiaohongshu": true,
		"xianyu":      true,
	}
	s.manager = browser.NewAutoReplyManager(defaultHeadless)
}

func NewAutoReplyController(svc *service.AutoReplyService, ragStack *knowledgesvc.RAGStack) *AutoReplyController {
	return &AutoReplyController{
		svc:      svc,
		manager:  GetAutoReplyManager(), // 使用单例管理器
		ragStack: ragStack,
	}
}

type upsertAccountReq struct {
	Platform string `json:"platform"`
	Username string `json:"username"`
	Cookie   string `json:"cookie"`
	Headless *bool  `json:"headless,omitempty"` // 无头模式，可选参数
}

type saveRuleReq struct {
	Platform     string  `json:"platform"`
	Keywords     string  `json:"keywords"`
	ReplyContent string  `json:"reply_content"`
	Frequency    int     `json:"frequency"`
	DailyLimit   int     `json:"daily_limit"`
	StartTime    *string `json:"start_time,omitempty"` // 开始时间 (HH:MM格式)
	EndTime      *string `json:"end_time,omitempty"`   // 结束时间 (HH:MM格式)
	IsActive     bool    `json:"is_active"`
	IsRagEnabled bool    `json:"is_rag_enabled"`
	RagProductID *string `json:"rag_product_id,omitempty"`
}

// 启动登录流程：创建账号记录并打开浏览器到抖音页面，登录后自动保存 Cookie
func (c *AutoReplyController) StartLogin(ctx *gin.Context) {
	var req struct {
		Platform string `json:"platform"`
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
	now := time.Now()

	// 设置默认无头模式为true（后台运行）
	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	accountID, err := c.svc.UpsertAutoReplyAccount(context.Background(), service.AutoReplyAccountCreateReq{
		UserID:   userID,
		Platform: req.Platform,
		Username: req.Username,
		IsActive: true,
		Headless: headless, // 添加无头模式设置
		LoginAt:  &now,
	})
	if err != nil {
		response.Error(ctx, 500, "启动失败")
		return
	}
	// 启动浏览器登录流程（后台异步）
	c.svc.StartLoginBrowser(context.Background(), userID, req.Platform, req.Username, accountID, headless)
	response.Success(ctx, gin.H{"started": true, "accountId": accountID, "headless": headless}, "ok")
}

// 查询登录状态：根据是否存在 Cookie 判定，并返回 Cookie（如已获取）
func (c *AutoReplyController) LoginStatus(ctx *gin.Context) {
	platform := ctx.Query("platform")
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
	items, err := c.svc.ListAccounts(context.Background(), platform, userID)
	if err != nil {
		response.Error(ctx, 500, "查询失败")
		return
	}
	status := "waiting"
	var accountId uint
	var cookie string
	for _, a := range items {
		if a.Username == username {
			accountId = a.ID
			if a.Cookie != "" {
				switch platform {
				case "douyin":
					if strings.Contains(a.Cookie, "sessionid=") {
						status = "logged_in"
						cookie = a.Cookie
					}
				case "kuaishou":
					// 仅当包含 sid 或 session 关键字时视为登录
					lc := strings.ToLower(a.Cookie)
					if strings.Contains(lc, "sid=") || strings.Contains(lc, "session=") {
						status = "logged_in"
						cookie = a.Cookie
					}
				case "xiaohongshu":
					if strings.Contains(a.Cookie, "web_session=") {
						status = "logged_in"
						cookie = a.Cookie
					}
				}
			}
			break
		}
	}
	response.Success(ctx, gin.H{"status": status, "accountId": accountId, "cookie": cookie}, "ok")
}

func (c *AutoReplyController) ListAccounts(ctx *gin.Context) {
	platform := ctx.Query("platform")

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
	items, err := c.svc.ListAccounts(context.Background(), platform, userID)
	if err != nil {
		response.Error(ctx, 500, "查询失败")
		return
	}
	response.Success(ctx, gin.H{"list": items}, "ok")
}

func (c *AutoReplyController) UpsertAccount(ctx *gin.Context) {
	var req upsertAccountReq
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
	loginAt := time.Now()

	// 设置默认无头模式为true（后台运行）
	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	accountID, err := c.svc.UpsertAutoReplyAccount(context.Background(), service.AutoReplyAccountCreateReq{
		UserID:   userID,
		Platform: req.Platform,
		Username: req.Username,
		Cookie:   req.Cookie,
		IsActive: true,
		Headless: headless, // 添加无头模式设置
		LoginAt:  &loginAt,
	})
	if err != nil {
		response.Error(ctx, 500, "保存失败")
		return
	}
	response.Success(ctx, gin.H{"id": accountID, "headless": headless}, "ok")
}

func (c *AutoReplyController) SaveCookies(ctx *gin.Context) {
	idStr := ctx.Param("id")
	var payload struct {
		Cookie string `json:"cookie"`
	}
	if err := ctx.ShouldBindJSON(&payload); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}
	// R8 修复：原实现忽略 parse 错误，且 SaveCookies 不校验账号所有权（IDOR）。
	// 现校验 :id 必须为正整数，并从 JWT 取 userID 传给 service 做所有权校验。
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
	if HandleDBError(ctx, c.svc.SaveCookies(context.Background(), uint(id64), payload.Cookie, userID), "保存Cookies") {
		return
	}
	response.Success(ctx, gin.H{"id": id64}, "ok")
}

func (c *AutoReplyController) DeleteAccount(ctx *gin.Context) {
	idStr := ctx.Param("id")
	// R8 修复：校验 :id 必须为正整数
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
	if HandleDBError(ctx, c.svc.DeleteAccount(context.Background(), uint(id64), userID), "删除自动回复账号") {
		return
	}
	response.Success(ctx, gin.H{"id": id64}, "ok")
}

func (c *AutoReplyController) GetRule(ctx *gin.Context) {
	platform := ctx.Query("platform")

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID int
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	} else {
		// 类型断言获取uint类型的用户ID
		if id, ok := userIDInterface.(uint); ok {
			userID = int(id)
		}
	}

	rule, err := c.svc.GetRule(context.Background(), platform, uint(userID))
	if err != nil {
		response.Success(ctx, gin.H{"rule": nil}, "ok")
		return
	}
	response.Success(ctx, gin.H{"rule": rule}, "ok")
}

func (c *AutoReplyController) SaveRule(ctx *gin.Context) {
	var req saveRuleReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	// 从上下文中获取当前用户ID
	userIDInterface, exists := ctx.Get("user_id")
	var userID int
	if !exists {
		response.Error(ctx, 401, "用户未认证")
		return
	} else {
		// 类型断言获取uint类型的用户ID
		if id, ok := userIDInterface.(uint); ok {
			userID = int(id)
		}
	}

	err := c.svc.SaveRuleDTO(context.Background(), service.AutoReplyRuleSaveReq{
		UserID:       uint(userID),
		Platform:     req.Platform,
		Keywords:     req.Keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		IsActive:     req.IsActive,
		IsRagEnabled: req.IsRagEnabled,
		RagProductID: req.RagProductID,
	})
	if err != nil {
		response.Error(ctx, 500, "保存失败")
		return
	}
	response.Success(ctx, gin.H{"ok": true}, "ok")
}

func (c *AutoReplyController) ListLogs(ctx *gin.Context) {
	platform := ctx.Query("platform")

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
	items, total, err := c.svc.ListRecentLogs(context.Background(), platform, userID, page, pageSize)
	if err != nil {
		response.Error(ctx, 500, "查询失败")
		return
	}
	response.Success(ctx, gin.H{"list": items, "total": total, "page": page, "page_size": pageSize}, "ok")
}

func (c *AutoReplyController) Start(ctx *gin.Context) {
	var req struct {
		Platform  string `json:"platform" binding:"required"`
		AccountID uint   `json:"account_id" binding:"required"`
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
	// 获取商户账户的无头模式设置（下沉到 service，controller 不直连 DB）
	settings, err := c.svc.GetMerchantHeadlessSettings(context.Background())
	if err != nil {
		response.Error(ctx, 500, "获取商户账户信息失败")
		return
	}

	// 获取账号信息
	accounts, err := c.svc.ListAccounts(context.Background(), req.Platform, userID)
	if err != nil {
		response.Error(ctx, 500, "获取账号信息失败")
		return
	}

	var found bool
	var accUsername, accCookie string
	var accID uint
	for _, acc := range accounts {
		if acc.ID == req.AccountID {
			accUsername, accCookie, accID = acc.Username, acc.Cookie, acc.ID
			found = true
			break
		}
	}

	if !found {
		response.Error(ctx, 404, "账号不存在")
		return
	}

	// 更新管理器的无头模式设置
	headlessSettings := map[string]bool{
		"douyin":      settings.Douyin,
		"kuaishou":    settings.Kuaishou,
		"xiaohongshu": settings.Xiaohongshu,
		"xianyu":      settings.Xianyu,
	}

	// 根据平台获取对应的无头模式设置
	platformHeadless := true // 默认使用无头模式
	if v, ok := headlessSettings[req.Platform]; ok {
		platformHeadless = v
	}

	// 使用单例管理器并更新其设置
	manager := GetAutoReplyManager()
	for platform, headless := range headlessSettings {
		manager.SetHeadless(platform, headless)
	}
	c.manager = manager

	// 使用管理器创建机器人实例
	platform := browser.Platform(req.Platform)
	if err := c.manager.StartBot(platform, accUsername, accID, accCookie); err != nil {
		response.Error(ctx, 500, fmt.Sprintf("创建机器人失败: %v", err))
		return
	}

	// 获取机器人实例并启动
	bot, err := c.manager.GetBot(platform)
	if err != nil {
		response.Error(ctx, 500, fmt.Sprintf("获取机器人实例失败: %v", err))
		return
	}

	dedup := browser.NewInMemoryDedup(5 * time.Minute)
	bot.SetDedup(dedup)

	replyService := service.NewAutoReplyServiceAuto()

	bot.SetReplyHandler(browser.NewIntegrationReplyHandler(
		c.ragStack.Integration,
		c.ragStack.Customer,
		c.ragStack.Retrieval,
		replyService,
	))

	// 启动机器人，传入规则匹配器和用户ID
	if err := bot.Start(replyService, userID); err != nil {
		response.Error(ctx, 500, fmt.Sprintf("启动机器人失败: %v", err))
		return
	}

	response.Success(ctx, gin.H{
		"started":    true,
		"platform":   req.Platform,
		"account_id": req.AccountID,
		"headless":   platformHeadless,
	}, "机器人启动成功")
}

func (c *AutoReplyController) Stop(ctx *gin.Context) {
	var req struct {
		Platform string `json:"platform" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	platform := browser.Platform(req.Platform)
	if err := c.manager.StopBot(platform); err != nil {
		response.Error(ctx, 500, fmt.Sprintf("停止机器人失败: %v", err))
		return
	}

	response.Success(ctx, gin.H{
		"stopped":  true,
		"platform": req.Platform,
	}, "机器人停止成功")
}

// GetHeadlessMode 获取无头模式设置
func (c *AutoReplyController) GetHeadlessMode(ctx *gin.Context) {
	// 获取商户账户的无头模式设置（下沉到 service，不存在则返回默认 true）
	settings, err := c.svc.GetMerchantHeadlessSettings(context.Background())
	if err != nil {
		response.Error(ctx, 500, "获取商户账户信息失败")
		return
	}

	response.Success(ctx, gin.H{
		"headless_settings": gin.H{
			"douyin":      settings.Douyin,
			"kuaishou":    settings.Kuaishou,
			"xiaohongshu": settings.Xiaohongshu,
			"xianyu":      settings.Xianyu,
		},
		"descriptions": gin.H{
			"douyin":      getHeadlessDescription(settings.Douyin),
			"kuaishou":    getHeadlessDescription(settings.Kuaishou),
			"xiaohongshu": getHeadlessDescription(settings.Xiaohongshu),
			"xianyu":      getHeadlessDescription(settings.Xianyu),
		},
	}, "获取无头模式设置成功")
}

// SetHeadlessMode 设置无头模式（通过商户账户）
func (c *AutoReplyController) SetHeadlessMode(ctx *gin.Context) {
	var req struct {
		Platform string `json:"platform" binding:"required"`
		Headless bool   `json:"headless" binding:"required"`
	}

	// 读取请求体
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		logger.Errorf("读取请求体失败: %v", err)
		response.Error(ctx, 400, fmt.Sprintf("读取请求体失败: %v", err))
		return
	}

	// 敏感数据保护（R4 修复）：原实现直接打印原始请求体，可能包含凭证/Token。
	// 改为仅记录结构化字段，并对 headless 值做日志输出。
	// 同时复用 body 给后续 Unmarshal，避免重复读取 ctx.Request.Body。

	// 解析JSON
	if err := json.Unmarshal(body, &req); err != nil {
		logger.Errorf("JSON解析失败: %v", err)
		response.Error(ctx, 400, fmt.Sprintf("JSON解析错误: %v", err))
		return
	}

	logger.Infof("成功解析无头模式参数: 平台=%s, 无头模式=%v", req.Platform, req.Headless)

	// 更新商户账户无头模式设置（取/建 + 保存下沉到 service）
	if err := c.svc.SetMerchantHeadless(context.Background(), req.Platform, req.Headless); err != nil {
		if err.Error() == "不支持的平台" {
			response.Error(ctx, 400, "不支持的平台")
			return
		}
		response.Error(ctx, 500, "保存无头模式设置失败")
		return
	}

	// 读取最新设置并更新单例管理器
	settings, err := c.svc.GetMerchantHeadlessSettings(context.Background())
	if err != nil {
		response.Error(ctx, 500, "读取无头模式设置失败")
		return
	}
	manager := GetAutoReplyManager()
	headlessSettings := map[string]bool{
		"douyin":      settings.Douyin,
		"kuaishou":    settings.Kuaishou,
		"xiaohongshu": settings.Xiaohongshu,
		"xianyu":      settings.Xianyu,
	}
	for platform, headless := range headlessSettings {
		manager.SetHeadless(platform, headless)
	}

	response.Success(ctx, gin.H{
		"platform": req.Platform,
		"headless": req.Headless,
		"description": map[bool]string{
			true:  "已切换为后台运行模式（无界面）",
			false: "已切换为可视化模式（显示浏览器）",
		}[req.Headless],
		"note": "新启动的机器人将使用此设置，当前运行的机器人不受影响",
	}, "设置无头模式成功")
}

// TestBrowser 测试浏览器功能
//
// SSRF 防护（R3 修复）：原实现直接将用户提交的 URL 传给浏览器导航，
// 可被用于探测内部服务/云元数据（如 169.254.169.254）。
// 现使用 urlguard.ValidateURL 在导航前校验目标 URL。
func (c *AutoReplyController) TestBrowser(ctx *gin.Context) {
	var req struct {
		URL      string `json:"url" binding:"required"`
		Headless bool   `json:"headless"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	// SSRF 校验：拒绝私有/回环/链路本地/CGNAT 地址及非 http(s) 协议
	if err := urlguard.ValidateURL(req.URL); err != nil {
		response.Error(ctx, 400, fmt.Sprintf("URL 校验失败: %v", err))
		return
	}

	// 创建临时浏览器实例进行测试
	opts := browser.Options{
		Headless: req.Headless,
	}

	assistant, err := browser.NewAssistant(opts)
	if err != nil {
		response.Error(ctx, 500, fmt.Sprintf("创建浏览器失败: %v", err))
		return
	}
	defer assistant.Close()

	// 测试导航
	if err := assistant.Navigate(req.URL); err != nil {
		response.Error(ctx, 500, fmt.Sprintf("导航失败: %v", err))
		return
	}

	// 获取页面标题
	title, err := assistant.Evaluate("document.title")
	if err != nil {
		response.Error(ctx, 500, fmt.Sprintf("获取标题失败: %v", err))
		return
	}

	// 获取页面截图（可选）
	_, screenshotErr := assistant.Screenshot("body")

	response.Success(ctx, gin.H{
		"message":    "浏览器测试成功",
		"url":        req.URL,
		"title":      title,
		"headless":   req.Headless,
		"screenshot": screenshotErr == nil,
	}, "浏览器测试成功")
}

// GetDebugStatus 获取调试状态
func (c *AutoReplyController) GetDebugStatus(ctx *gin.Context) {
	// 从环境变量读取配置，避免硬编码
	remoteDebugging := os.Getenv("REMOTE_DEBUGGING_URL")
	if remoteDebugging == "" {
		remoteDebugging = "http://localhost:8206"
	}
	browserPath := os.Getenv("BROWSER_PATH")
	if browserPath == "" {
		browserPath = "/usr/bin/chromium-browser"
	}
	dockerContainer := os.Getenv("DOCKER_CONTAINER_NAME")
	if dockerContainer == "" {
		dockerContainer = "marketing_tools_user_server"
	}

	// 获取商户账户无头模式设置（下沉到 service，不存在则返回默认 true）
	settings, err := c.svc.GetMerchantHeadlessSettings(context.Background())
	if err != nil {
		response.Error(ctx, 500, "获取商户账户信息失败")
		return
	}

	response.Success(ctx, gin.H{
		"status": "ok",
		"headless_settings": gin.H{
			"douyin":      settings.Douyin,
			"kuaishou":    settings.Kuaishou,
			"xiaohongshu": settings.Xiaohongshu,
			"xianyu":      settings.Xianyu,
		},
		"remote_debugging": remoteDebugging,
		"browser_path":     browserPath,
		"docker_container": dockerContainer,
	}, "获取调试状态成功")
}

// ToggleHeadless 切换无头模式
func (c *AutoReplyController) ToggleHeadless(ctx *gin.Context) {
	var req struct {
		Platform string `json:"platform" binding:"required"`
		Headless bool   `json:"headless"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, 400, "参数错误")
		return
	}

	// 更新商户账户无头模式设置（取/建 + 保存下沉到 service）
	if err := c.svc.SetMerchantHeadless(context.Background(), req.Platform, req.Headless); err != nil {
		if err.Error() == "不支持的平台" {
			response.Error(ctx, 400, "不支持的平台")
			return
		}
		response.Error(ctx, 500, "更新设置失败")
		return
	}

	response.Success(ctx, gin.H{
		"message":     "无头模式设置已更新",
		"platform":    req.Platform,
		"headless":    req.Headless,
		"description": getHeadlessDescription(req.Headless),
	}, "无头模式设置已更新")
}
