package service

import (
	"context"
	"errors"
)

// errBridgeOutboundNotRegistered 桥接出站回调未注册（bridge 包初始化未完成）
var errBridgeOutboundNotRegistered = errors.New("bridge outbound not registered")

// BridgeOutboundFunc 桥接渠道出站回调：把 AI 回复经 WebSocket 投递到 Chrome 扩展。
//
// 由 bridge 包在初始化时注册（避免 service -> bridge 的导入环），
// WebhookService.sendOutbound 在网页桥接渠道（douyin_web/xhs_web/tiktok_web）下
// 调用 DeliverBridgeOutbound，从而把 AI 回复原路回写到浏览器扩展，而非走官方 API。
//
// eventID 用于 ClaimReply 幂等守卫（同一 AI 回复仅一次出站）。
type BridgeOutboundFunc func(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) error

// bridgeOutbound 全局桥接出站回调（由 bridge.SetBridgeReachAdapter 注册）
var bridgeOutbound BridgeOutboundFunc

// RegisterBridgeOutbound 注册桥接出站回调（bridge 包调用，避免导入环）
func RegisterBridgeOutbound(f BridgeOutboundFunc) {
	bridgeOutbound = f
}

// DeliverBridgeOutbound 经桥接回调下发 AI 回复；未注册时返回错误（调用方降级日志）
func DeliverBridgeOutbound(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) error {
	if bridgeOutbound == nil {
		return errBridgeOutboundNotRegistered
	}
	return bridgeOutbound(ctx, channel, accountID, conversationID, msgType, content, eventID)
}
