package controller

import (
	"context"
	"hivemtk-user/internal/pkg/utils/pagination"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/platform"
	"hivemtk-user/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UnifiedMessageController 统一消息控制器
type UnifiedMessageController struct {
	messageService *service.UnifiedMessageService
}

// NewUnifiedMessageController 创建统一消息控制器实例
func NewUnifiedMessageController() *UnifiedMessageController {
	return &UnifiedMessageController{
		messageService: service.NewUnifiedMessageService(),
	}
}

// GetMessages 获取消息列表
func (c *UnifiedMessageController) GetMessages(ctx *gin.Context) {

	platform := ctx.Query("platform")
	page, pageSize, err := pagination.Parse(ctx)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	messages, total, err := c.messageService.GetMessages(context.Background(), platform, page, pageSize)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, gin.H{
		"list":      messages,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}, "获取成功")
}

// GetMessageByID 获取消息详情
func (c *UnifiedMessageController) GetMessageByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的消息ID")
		return
	}

	msg, err := c.messageService.GetMessageByID(context.Background(), uint(id))
	if err != nil {
		response.Error(ctx, http.StatusNotFound, err.Error())
		return
	}

	response.Success(ctx, msg, "获取成功")
}

// GetReplies 获取消息回复列表
func (c *UnifiedMessageController) GetReplies(ctx *gin.Context) {

	messageID := ctx.Param("message_id")
	if messageID == "" {
		response.Error(ctx, http.StatusBadRequest, "缺少消息ID")
		return
	}

	replies, err := c.messageService.GetReplies(context.Background(), messageID)
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, replies, "获取成功")
}

// PlatformAccountController 平台账号控制器
type PlatformAccountController struct {
	accountService *service.PlatformAccountService
}

// NewPlatformAccountController 创建平台账号控制器实例
func NewPlatformAccountController() *PlatformAccountController {
	return &PlatformAccountController{
		accountService: service.NewPlatformAccountService(),
	}
}

// GetAccounts 获取平台账号列表
func (c *PlatformAccountController) GetAccounts(ctx *gin.Context) {

	accounts, err := c.accountService.GetAccounts(context.Background())
	if err != nil {
		response.ErrorFromDB(ctx, err, err.Error())
		return
	}

	response.Success(ctx, accounts, "获取成功")
}

// GetAccountByID 获取平台账号详情
func (c *PlatformAccountController) GetAccountByID(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	account, err := c.accountService.GetAccountByID(context.Background(), uint(id))
	if HandleDBError(ctx, err, "获取平台账号") {
		return
	}

	response.Success(ctx, account, "获取成功")
}

// CreateAccount 创建平台账号
func (c *PlatformAccountController) CreateAccount(ctx *gin.Context) {

	var req service.CreatePlatformAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	account, err := c.accountService.CreateAccount(context.Background(), &req)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, err.Error())
		return
	}

	response.Success(ctx, account, "创建成功")
}

// UpdateAccount 更新平台账号
func (c *PlatformAccountController) UpdateAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var req service.UpdatePlatformAccountRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	account, err := c.accountService.UpdateAccount(context.Background(), uint(id), &req)
	if HandleDBError(ctx, err, "更新平台账号") {
		return
	}

	response.Success(ctx, account, "更新成功")
}

// DeleteAccount 删除平台账号
func (c *PlatformAccountController) DeleteAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	if HandleDBError(ctx, c.accountService.DeleteAccount(context.Background(), uint(id)), "删除平台账号") {
		return
	}

	response.Success(ctx, nil, "删除成功")
}

// LoginAccount 登录平台账号
func (c *PlatformAccountController) LoginAccount(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	var req service.PlatformLoginRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		response.Error(ctx, http.StatusBadRequest, "请求参数错误: "+err.Error())
		return
	}

	account, err := c.accountService.Login(context.Background(), uint(id), &req)
	if HandleDBError(ctx, err, "登录平台账号") {
		return
	}

	response.Success(ctx, account, "登录成功")
}

// CheckLoginStatus 检查登录状态
func (c *PlatformAccountController) CheckLoginStatus(ctx *gin.Context) {

	idStr := ctx.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.Error(ctx, http.StatusBadRequest, "无效的账号ID")
		return
	}

	status, err := c.accountService.CheckLoginStatus(context.Background(), uint(id))
	if HandleDBError(ctx, err, "检查登录状态") {
		return
	}

	response.Success(ctx, gin.H{
		"is_logged_in": status,
	}, "检查成功")
}

// GetSupportedPlatforms 获取支持的平台列表
func (c *PlatformAccountController) GetSupportedPlatforms(ctx *gin.Context) {
	registry := platform.GetAdapterRegistry()
	platforms := registry.GetPlatforms()

	result := make([]map[string]string, 0, len(platforms))
	platformNames := map[string]string{
		"douyin":      "抖音",
		"kuaishou":    "快手",
		"xiaohongshu": "小红书",
		"xianyu":      "闲鱼",
	}

	for _, p := range platforms {
		result = append(result, map[string]string{
			"code": string(p),
			"name": platformNames[string(p)],
		})
	}

	response.Success(ctx, result, "获取成功")
}
