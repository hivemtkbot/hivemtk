// marketing/internal/controller/tiktok_auto_reply_controller.go
package controller

import (
	"math"
	"net/http"
	"strconv"

	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

type TikTokAutoReplyController struct {
	svc *service.TikTokAutoReplyService
}

func NewTikTokAutoReplyController() *TikTokAutoReplyController {
	return &TikTokAutoReplyController{
		svc: service.NewTikTokAutoReplyService(),
	}
}

// 当前用户ID解析
func (ctrl *TikTokAutoReplyController) currentUserID(c *gin.Context) uint {
	v, ok := c.Get("user_id")
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case uint:
		return t
	case float64:
		if t < 0 || t > math.MaxUint32 {
			return 0
		}
		return uint(t)
	case int:
		if t < 0 {
			return 0
		}
		return uint(t)
	case int64:
		if t < 0 {
			return 0
		}
		return uint(t)
	case string:
		id, _ := strconv.ParseUint(t, 10, 64)
		return uint(id)
	}
	return 0
}

// 获取TikTok自动回复账号列表
func (ctrl *TikTokAutoReplyController) GetAccounts(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	accounts, err := ctrl.svc.ListAccounts(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取账号列表失败", err.Error())
		return
	}
	response.Success(c, accounts, "获取账号列表成功")
}

// 获取TikTok自动回复规则
func (ctrl *TikTokAutoReplyController) GetRule(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	rule, err := ctrl.svc.GetRule(userID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取规则失败", err.Error())
		return
	}
	keywords := []string{}
	if rule.Keywords != "" {
		keywords = []string{rule.Keywords}
	}
	response.Success(c, gin.H{
		"id":            rule.ID,
		"name":          "TikTok 默认规则",
		"is_active":     rule.IsActive,
		"auto_reply":    rule.IsActive,
		"reply_delay":   rule.Frequency,
		"start_time":    derefString(rule.StartTime),
		"end_time":      derefString(rule.EndTime),
		"daily_limit":   rule.DailyLimit,
		"keywords":      keywords,
		"reply_content": rule.ReplyContent,
		"reply_config":  gin.H{},
	}, "获取规则成功")
}

// 保存TikTok自动回复规则
func (ctrl *TikTokAutoReplyController) SaveRule(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	var req service.SaveRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	rule, err := ctrl.svc.SaveRule(userID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "保存规则失败", err.Error())
		return
	}
	response.Success(c, rule, "保存规则成功")
}

// 保存TikTok账号Cookie
func (ctrl *TikTokAutoReplyController) SaveCookies(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的账号ID", err.Error())
		return
	}
	var req struct {
		Cookie string `json:"cookie" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	// R8 修复：透传 userID 做账号所有权校验（IDOR 防护）
	userID := ctrl.currentUserID(c)
	if userID == 0 {
		response.Error(c, http.StatusUnauthorized, "用户未认证")
		return
	}
	if err := ctrl.svc.SaveCookies(uint(id), req.Cookie, userID); err != nil {
		HandleServiceError(c, err)
		return
	}
	response.Success(c, gin.H{
		"account_id": id,
		"cookie":     "***", // 不回显敏感数据
	}, "保存Cookie成功")
}

// 更新或创建TikTok账号
func (ctrl *TikTokAutoReplyController) UpsertAccount(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	var req service.UpsertAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "参数错误", err.Error())
		return
	}
	account, err := ctrl.svc.UpsertAccount(userID, &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "保存账号失败", err.Error())
		return
	}
	response.Success(c, account, "账号保存成功")
}

// 删除TikTok账号
func (ctrl *TikTokAutoReplyController) DeleteAccount(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "无效的账号ID", err.Error())
		return
	}
	if err := ctrl.svc.DeleteAccount(uint(id), userID); err != nil {
		response.Error(c, http.StatusInternalServerError, "删除账号失败", err.Error())
		return
	}
	response.Success(c, gin.H{
		"account_id": id,
	}, "账号删除成功")
}

// 获取TikTok自动回复日志
func (ctrl *TikTokAutoReplyController) ListLogs(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	logs, total, err := ctrl.svc.ListLogs(userID, page, pageSize)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "获取日志失败", err.Error())
		return
	}
	response.Success(c, gin.H{
		"list":  logs,
		"total": total,
		"page":  page,
		"size":  pageSize,
	}, "获取日志成功")
}

// 启动TikTok自动回复
func (ctrl *TikTokAutoReplyController) Start(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	if err := ctrl.svc.Start(userID); err != nil {
		response.Error(c, http.StatusInternalServerError, "启动失败", err.Error())
		return
	}
	status, _ := ctrl.svc.Status(userID)
	response.Success(c, gin.H{
		"status": "started",
		"info":   status,
	}, "自动回复已启动")
}

// 停止TikTok自动回复
func (ctrl *TikTokAutoReplyController) Stop(c *gin.Context) {
	userID := ctrl.currentUserID(c)
	if err := ctrl.svc.Stop(userID); err != nil {
		response.Error(c, http.StatusInternalServerError, "停止失败", err.Error())
		return
	}
	status, _ := ctrl.svc.Status(userID)
	response.Success(c, gin.H{
		"status": "stopped",
		"info":   status,
	}, "自动回复已停止")
}

// RegisterRoutes 注册路由
func (ctrl *TikTokAutoReplyController) RegisterRoutes(router *gin.RouterGroup) {
	autoReply := router.Group("/tiktok/auto-reply")
	{
		autoReply.GET("/accounts", ctrl.GetAccounts)
		autoReply.GET("/rule", ctrl.GetRule)
		autoReply.POST("/rule", ctrl.SaveRule)
		autoReply.POST("/accounts", ctrl.UpsertAccount)
		autoReply.POST("/accounts/:id/cookies", ctrl.SaveCookies)
		autoReply.DELETE("/accounts/:id", ctrl.DeleteAccount)
		autoReply.GET("/logs", ctrl.ListLogs)
		autoReply.POST("/start", ctrl.Start)
		autoReply.POST("/stop", ctrl.Stop)
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
