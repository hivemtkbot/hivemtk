package controller

import (
	"net/http"

	"hivemtk-user/internal/bridge"
	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/service"

	"github.com/gin-gonic/gin"
)

// BridgeAccountController 桥接账号管理（抖音/小红书/TikTok 网页私信）
//
//	GET  /api/bridge/accounts  —— 列出当前用户的桥接账号（含在线状态）
//	POST /api/bridge/send      —— 人工座席经桥接代发消息（G14）
type BridgeAccountController struct{}

func NewBridgeAccountController() *BridgeAccountController {
	return &BridgeAccountController{}
}

// RegisterRoutes 注册桥接账号相关路由（在已鉴权的 auth 组下调用）
func (ctrl *BridgeAccountController) RegisterRoutes(auth *gin.RouterGroup) {
	auth.GET("/bridge/accounts", ctrl.List)
	auth.POST("/bridge/send", ctrl.SendManual)
}

// List 列出当前用户的全部桥接账号
func (ctrl *BridgeAccountController) List(c *gin.Context) {
	if bridge.GlobalBridgeAccountRepo == nil {
		response.Error(c, http.StatusServiceUnavailable, "bridge account repo not initialized")
		return
	}
	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	views, err := bridge.GlobalBridgeAccountRepo.ListByUser(c.Request.Context(), uid)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"accounts": views}, "ok")
}

type bridgeSendReq struct {
	Channel        string `json:"channel"`
	AccountID      string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
}

func (ctrl *BridgeAccountController) SendManual(c *gin.Context) {
	var req bridgeSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, err.Error())
		return
	}
	if !bridge.IsBridgeChannel(req.Channel) || req.AccountID == "" || req.ConversationID == "" || req.Content == "" {
		response.Error(c, http.StatusBadRequest, "invalid params: channel/account_id/conversation_id/content required")
		return
	}
	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	if bridge.GlobalOwnershipChecker != nil {
		owns, err := bridge.GlobalOwnershipChecker(c.Request.Context(), uid, req.Channel, req.AccountID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, err.Error())
			return
		}
		if !owns {
			response.Error(c, http.StatusForbidden, "account not owned by current user")
			return
		}
	}
	if bridge.GlobalBridgeAccountRepo == nil {
		response.Error(c, http.StatusServiceUnavailable, "bridge account repo not initialized")
		return
	}
	online, err := bridge.GlobalBridgeAccountRepo.IsOnline(c.Request.Context(), req.Channel, req.AccountID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	if !online {
		response.Error(c, http.StatusConflict, "bridge account offline")
		return
	}
	if err := service.DeliverBridgeOutbound(c.Request.Context(), req.Channel, req.AccountID, req.ConversationID, "text", req.Content, ""); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, gin.H{"ok": true}, "ok")
}
