package controller

import (
	"net/http"

	"marketing/internal/bridge"

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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bridge account repo not initialized"})
		return
	}
	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	views, err := bridge.GlobalBridgeAccountRepo.ListByUser(c.Request.Context(), uid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"accounts": views})
}

type bridgeSendReq struct {
	Channel       string `json:"channel"`
	AccountID     string `json:"account_id"`
	ConversationID string `json:"conversation_id"`
	Content       string `json:"content"`
}

// SendManual 人工座席经桥接代发（G14）：校验归属 + 在线后投递到扩展
func (ctrl *BridgeAccountController) SendManual(c *gin.Context) {
	var req bridgeSendReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !bridge.IsBridgeChannel(req.Channel) || req.AccountID == "" || req.ConversationID == "" || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid params: channel/account_id/conversation_id/content required"})
		return
	}
	// 账号归属校验
	uidAny, _ := c.Get("user_id")
	uid, _ := uidAny.(uint)
	if bridge.GlobalOwnershipChecker != nil {
		owns, err := bridge.GlobalOwnershipChecker(c.Request.Context(), uid, req.Channel, req.AccountID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if !owns {
			c.JSON(http.StatusForbidden, gin.H{"error": "account not owned by current user"})
			return
		}
	}
	// 必须在线才能投递
	if !bridge.GetBridgeHub().IsOnline(req.Channel, req.AccountID) {
		c.JSON(http.StatusConflict, gin.H{"error": "bridge account offline"})
		return
	}
	reply := &bridge.UnifiedReply{
		Channel:        req.Channel,
		AccountID:      req.AccountID,
		ConversationID: req.ConversationID,
		Content:        req.Content,
		MsgType:        "text",
	}
	if err := bridge.GetBridgeHub().Deliver(req.Channel, req.AccountID, reply); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
