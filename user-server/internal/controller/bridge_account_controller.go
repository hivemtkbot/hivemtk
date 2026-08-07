package controller

import (
	"fmt"
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

// SendManual 人工座席经桥接代发（G14）：校验归属 + 在线后投递到 message_hub(outbound, status=pending)
//
// 2026-08-06 三通道架构（WS → HTTP → outbox）：
//   - 旧实现：BridgeReachAdapter.EnqueueReply 直接入 httpReplyBuffer，由扩展端长轮询拉走
//   - 新实现：BridgeReachAdapter.EnqueueManualReply 落库 message_hub(status=pending)，
//     由扩展端 GET /api/bridge/outbox 拉取并回写网页（ingest 已改为即时返回，buffer 不再被读取，
//     继续走 buffer 会导致人工回复静默丢失）
//   - "在线"判定与 controller 列表接口同源（OnlineGraceWindow = 30s）
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
	// 必须最近上报过（HTTP 模式 30s 内有 last_sync_at 更新）才视为在线
	if bridge.GlobalBridgeAccountRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bridge account repo not initialized"})
		return
	}
	online, err := bridge.GlobalBridgeAccountRepo.IsOnline(c.Request.Context(), req.Channel, req.AccountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !online {
		c.JSON(http.StatusConflict, gin.H{"error": "bridge account offline"})
		return
	}
	// 投递人工回复到 message_hub(outbound, status=pending)，由扩展端 GET /api/bridge/outbox 拉取并回写网页
	// （2026-08-06 三通道架构；不再走 httpReplyBuffer 长轮询，否则回复会静默丢失）。
	adapter := bridge.GlobalBridgeReachAdapter()
	if adapter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bridge reach adapter not initialized"})
		return
	}
	if err := adapter.EnqueueManualReply(c.Request.Context(), req.Channel, req.AccountID, req.ConversationID, req.Content, fmt.Sprint(uid)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
