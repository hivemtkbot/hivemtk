package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"
)

// ChatChannelController 客服 Web Widget 渠道管理（B 端）
//
// 提供：
//   - 渠道列表 / 创建 / 编辑 / 删除
//   - AppKey 轮换 / AppSecret 重置
//   - 渠道状态启用/禁用
//
// 路由前缀：/api/chat-channels（B 端 JWT 鉴权）
type ChatChannelController struct {
	svc *service.ChatChannelService
}

// NewChatChannelController 构造控制器
func NewChatChannelController(svc *service.ChatChannelService) *ChatChannelController {
	return &ChatChannelController{svc: svc}
}

// List 列出渠道
// GET /api/chat-channels
func (ctrl *ChatChannelController) List(c *gin.Context) {
	keyword := c.Query("keyword")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := ctrl.svc.List(c.Request.Context(), keyword, status, page, pageSize)
	if err != nil {
		response.ErrorFromDB(c, err, "查询失败", err.Error())
		return
	}
	response.SuccessWithPage(c, list, int64(page), int64(pageSize), total)
}

// Get 查询单个渠道
// GET /api/chat-channels/:channel_id
func (ctrl *ChatChannelController) Get(c *gin.Context) {
	channelID := c.Param("channel_id")
	channel, err := ctrl.svc.GetByChannelID(c.Request.Context(), channelID)
	if err != nil {
		response.NotFoundError(c, "渠道")
		return
	}
	// 不返回 AppSecretHash
	response.Success(c, channel, "ok")
}

// Create 创建渠道
// POST /api/chat-channels
func (ctrl *ChatChannelController) Create(c *gin.Context) {
	var req service.CreateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}

	// 获取当前用户（JWT 上下文）
	createdBy := uint(0)
	if v, ok := c.Get("user_id"); ok {
		if uid, ok := v.(uint); ok {
			createdBy = uid
		}
	}

	result, err := ctrl.svc.Create(c.Request.Context(), &req, createdBy)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, result, "渠道已创建")
}

// Update 更新渠道
// PUT /api/chat-channels/:channel_id
func (ctrl *ChatChannelController) Update(c *gin.Context) {
	channelID := c.Param("channel_id")
	var req service.UpdateChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "请求参数错误", err.Error())
		return
	}
	channel, err := ctrl.svc.Update(c.Request.Context(), channelID, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, channel, "已更新")
}

// Delete 禁用渠道（软删除）
// DELETE /api/chat-channels/:channel_id
func (ctrl *ChatChannelController) Delete(c *gin.Context) {
	channelID := c.Param("channel_id")
	if err := ctrl.svc.Delete(c.Request.Context(), channelID); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, nil, "已禁用")
}

// RotateAppKey 轮换 AppKey
// POST /api/chat-channels/:channel_id/rotate-key
func (ctrl *ChatChannelController) RotateAppKey(c *gin.Context) {
	channelID := c.Param("channel_id")
	newKey, err := ctrl.svc.RotateAppKey(c.Request.Context(), channelID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"app_key": newKey}, "AppKey 已轮换")
}

// ResetAppSecret 重置 AppSecret
// POST /api/chat-channels/:channel_id/reset-secret
func (ctrl *ChatChannelController) ResetAppSecret(c *gin.Context) {
	channelID := c.Param("channel_id")
	newSecret, err := ctrl.svc.ResetAppSecret(c.Request.Context(), channelID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	response.Success(c, gin.H{"app_secret": newSecret}, "AppSecret 已重置（仅返回一次）")
}
