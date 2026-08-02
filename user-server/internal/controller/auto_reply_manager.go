package controller

import (
	"net/http"
	"strconv"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

type AutoReplyManagerController struct {
	service *service.AutoReplyService
}

func NewAutoReplyManagerController(svc *service.AutoReplyService) *AutoReplyManagerController {
	return &AutoReplyManagerController{
		service: svc,
	}
}

// ListRules 获取自动回复规则列表
func (c *AutoReplyManagerController) ListRules(ctx *gin.Context) {
	var req dto.AutoReplyRuleListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	rules, total, err := c.service.ListRules(ctx, &req)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取规则列表失败")
		return
	}

	response.Success(ctx, gin.H{
		"list":  rules,
		"total": total,
	}, "获取规则列表成功")
}

// CreateRule 创建自动回复规则
func (c *AutoReplyManagerController) CreateRule(ctx *gin.Context) {
	// 从 JWT 上下文获取用户 ID
	userID, exists := ctx.Get("user_id")
	if !exists {
		response.Error(ctx, http.StatusUnauthorized, "未认证")
		return
	}

	var req struct {
		Keyword      string  `json:"keyword"`  // 兼容单数形式
		Keywords     string  `json:"keywords"` // 复数形式
		ReplyType    string  `json:"reply_type"`
		ReplyContent string  `json:"reply_content"`
		Priority     int     `json:"priority"`
		Enabled      bool    `json:"enabled"`
		Platform     string  `json:"platform"`
		Frequency    int     `json:"frequency"`
		DailyLimit   int     `json:"daily_limit"`
		StartTime    *string `json:"start_time,omitempty"`
		EndTime      *string `json:"end_time,omitempty"`
		IsActive     bool    `json:"is_active"`
		IsRagEnabled bool    `json:"is_rag_enabled"`
		RagProductID *string `json:"rag_product_id,omitempty"`
	}

	// 使用 BindJSON 而不是 ShouldBindJSON，以便更好地处理错误
	if err := ctx.BindJSON(&req); err != nil {
		logger.Errorf("[AutoReply CreateRule] BindJSON error: %v", err)
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	// 兼容 keyword 和 keywords 字段
	keywords := req.Keywords
	if keywords == "" {
		keywords = req.Keyword
	}

	// 验证必填字段
	if keywords == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少必填字段：keyword 或 keywords")
		return
	}
	if req.ReplyContent == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少必填字段：reply_content")
		return
	}

	// 如果未提供 platform，默认为 douyin
	platform := req.Platform
	if platform == "" {
		platform = "douyin"
	}

	// 转换 enabled 到 is_active
	isActive := req.IsActive
	if !isActive && req.Enabled {
		isActive = req.Enabled
	}

	ruleReq := &dto.AutoReplyRuleRequest{
		UserID:       userID.(uint),
		Platform:     platform,
		Keywords:     keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		IsActive:     isActive,
		IsRagEnabled: req.IsRagEnabled,
		RagProductID: req.RagProductID,
	}

	rule, err := c.service.CreateRule(ctx, ruleReq)
	if err != nil {
		response.ErrorFromDB(ctx, err, "创建规则失败："+err.Error())
		return
	}

	response.Success(ctx, rule, "创建规则成功")
}

// UpdateRule 更新自动回复规则
func (c *AutoReplyManagerController) UpdateRule(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "ID参数错误")
		return
	}

	var req struct {
		Keyword      string  `json:"keyword"`
		Keywords     string  `json:"keywords"`
		ReplyType    string  `json:"reply_type"`
		ReplyContent string  `json:"reply_content"`
		Priority     int     `json:"priority"`
		Enabled      bool    `json:"enabled"`
		Platform     string  `json:"platform"`
		Frequency    int     `json:"frequency"`
		DailyLimit   int     `json:"daily_limit"`
		StartTime    *string `json:"start_time,omitempty"`
		EndTime      *string `json:"end_time,omitempty"`
		IsActive     bool    `json:"is_active"`
		IsRagEnabled bool    `json:"is_rag_enabled"`
		RagProductID *string `json:"rag_product_id,omitempty"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误："+err.Error())
		return
	}

	// 兼容 keyword 和 keywords 字段
	keywords := req.Keywords
	if keywords == "" {
		keywords = req.Keyword
	}

	// 如果未提供 platform，默认为 douyin
	platform := req.Platform
	if platform == "" {
		platform = "douyin"
	}

	// 转换 enabled 到 is_active
	isActive := req.IsActive
	if !isActive && req.Enabled {
		isActive = req.Enabled
	}

	ruleReq := &dto.AutoReplyRuleRequest{
		Platform:     platform,
		Keywords:     keywords,
		ReplyContent: req.ReplyContent,
		Frequency:    req.Frequency,
		DailyLimit:   req.DailyLimit,
		StartTime:    req.StartTime,
		EndTime:      req.EndTime,
		IsActive:     isActive,
		IsRagEnabled: req.IsRagEnabled,
		RagProductID: req.RagProductID,
	}

	rule, err := c.service.UpdateRule(ctx, uint(id), ruleReq)
	if HandleServiceError(ctx, err) {
		return
	}

	response.Success(ctx, rule, "更新规则成功")
}

// DeleteRule 删除自动回复规则
func (c *AutoReplyManagerController) DeleteRule(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "ID参数错误")
		return
	}

	if err := c.service.DeleteRule(ctx, uint(id)); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "删除规则失败")
		return
	}

	response.Success(ctx, nil, "删除规则成功")
}

// TestMatching 测试关键词匹配
func (c *AutoReplyManagerController) TestMatching(ctx *gin.Context) {
	var req struct {
		Platform string `json:"platform" binding:"required"`
		Message  string `json:"message" binding:"required"`
		UserID   uint   `json:"user_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	result, err := c.service.TestMatching(ctx, req.Platform, req.Message, req.UserID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "测试匹配失败")
		return
	}

	response.Success(ctx, result, "测试匹配成功")
}

// SimulateMessage 模拟消息
func (c *AutoReplyManagerController) SimulateMessage(ctx *gin.Context) {
	var req struct {
		Platform  string `json:"platform" binding:"required"`
		Message   string `json:"message" binding:"required"`
		Sender    string `json:"sender"`
		UserID    uint   `json:"user_id"`
		AccountID uint   `json:"account_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	result, err := c.service.SimulateMessage(ctx, req.Platform, req.Message, req.Sender, req.UserID, req.AccountID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "模拟消息失败")
		return
	}

	response.Success(ctx, result, "模拟消息成功")
}

// TestBatchMatching 测试批量匹配
func (c *AutoReplyManagerController) TestBatchMatching(ctx *gin.Context) {
	var req struct {
		Platform  string   `json:"platform" binding:"required"`
		Messages  []string `json:"messages" binding:"required"`
		UserID    uint     `json:"user_id"`
		AccountID uint     `json:"account_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	results, err := c.service.TestBatchMatching(ctx, req.Platform, req.Messages, req.UserID, req.AccountID)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "批量匹配测试失败")
		return
	}

	response.Success(ctx, gin.H{
		"results": results,
		"total":   len(results),
	}, "批量匹配测试成功")
}

// TestRateLimit 测试速率限制
func (c *AutoReplyManagerController) TestRateLimit(ctx *gin.Context) {
	var req struct {
		Platform  string `json:"platform" binding:"required"`
		UserID    uint   `json:"user_id"`
		AccountID uint   `json:"account_id"`
		TestCount int    `json:"test_count"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	if req.TestCount <= 0 {
		req.TestCount = 10
	}

	results, err := c.service.TestRateLimit(ctx, req.Platform, req.UserID, req.AccountID, req.TestCount)
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "速率限制测试失败")
		return
	}

	response.Success(ctx, gin.H{
		"results": results,
		"summary": gin.H{
			"total":        len(results),
			"allowed":      countAllowed(results),
			"rate_limited": countRateLimited(results),
		},
	}, "速率限制测试成功")
}

// ResetDailyLimit 重置每日限制
func (c *AutoReplyManagerController) ResetDailyLimit(ctx *gin.Context) {
	var req struct {
		Platform  string `json:"platform" binding:"required"`
		UserID    uint   `json:"user_id"`
		AccountID uint   `json:"account_id"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "参数错误")
		return
	}

	if err := c.service.ResetDailyLimit(ctx, req.Platform, req.UserID, req.AccountID); err != nil {
		response.Error(ctx, http.StatusInternalServerError, "重置每日限制失败")
		return
	}

	response.Success(ctx, gin.H{
		"message": "每日限制已重置",
	}, "重置每日限制成功")
}

// GetRateLimitStats 获取速率限制统计
func (c *AutoReplyManagerController) GetRateLimitStats(ctx *gin.Context) {
	platform := ctx.Query("platform")
	userID, _ := strconv.ParseUint(ctx.Query("user_id"), 10, 32)
	accountID, _ := strconv.ParseUint(ctx.Query("account_id"), 10, 32)

	stats, err := c.service.GetRateLimitStats(ctx, platform, uint(userID), uint(accountID))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取速率限制统计失败")
		return
	}

	response.Success(ctx, stats, "获取速率限制统计成功")
}

// GetConcurrentStats 获取并发统计
func (c *AutoReplyManagerController) GetConcurrentStats(ctx *gin.Context) {
	platform := ctx.Query("platform")
	userID, _ := strconv.ParseUint(ctx.Query("user_id"), 10, 32)

	stats, err := c.service.GetConcurrentStats(ctx, platform, uint(userID))
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "获取并发统计失败")
		return
	}

	response.Success(ctx, stats, "获取并发统计成功")
}

// GetStatistics 获取综合统计
//
// 直接调用 c.service.GetDB().Model().Count() 多次会违反"controller 不得直接访问数据库"的硬约束，已下沉到 service.GetStatistics。
//
// `userID, _ := strconv.ParseUint(...)` 会忽略解析错误，非数值 user_id 会变成 0，
// 导致 if userID > 0 过滤失效，泄露跨用户聚合数据。对非法 user_id 显式返回 400。
func (c *AutoReplyManagerController) GetStatistics(ctx *gin.Context) {
	platform := ctx.Query("platform")

	// 校验 user_id：非空时必须是正整数
	var userID uint64 = 0
	if raw := ctx.Query("user_id"); raw != "" {
		var err error
		userID, err = strconv.ParseUint(raw, 10, 32)
		if err != nil || userID == 0 {
			response.Error(ctx, http.StatusBadRequest, "user_id 参数非法")
			return
		}
	}

	stats, err := c.service.GetStatistics(ctx.Request.Context(), platform, uint(userID))
	if err != nil {
		logger.Errorf("获取综合统计失败: %v", err)
		response.Error(ctx, http.StatusInternalServerError, "获取综合统计失败")
		return
	}

	response.Success(ctx, stats, "获取综合统计成功")
}

// Helper functions
func countAllowed(results []model.RateLimitTestResult) int {
	count := 0
	for _, result := range results {
		if result.Allowed {
			count++
		}
	}
	return count
}

func countRateLimited(results []model.RateLimitTestResult) int {
	count := 0
	for _, result := range results {
		if !result.Allowed {
			count++
		}
	}
	return count
}
