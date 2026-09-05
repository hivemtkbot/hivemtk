package bridge

import (
	"context"
	"errors"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/service"
)

// BridgeReachAdapter 桥接触达适配器：非网页渠道委托 inner；网页渠道直接调 service.DeliverBridgeOutbound。
//
// 2026-08-18 二次审核重构：
//   - 移除 httpReplyBuffer + Send*（抖音/小红书/TikTok/闲鱼/快手）方法 + 全部死代码；
//     旧实现把 AI 回复推入 in-memory buffer 后无人读取，proactive_reach / douyin_integration
//     / AI Agent reach.*.send 全部走这条死通道 → 静默丢消息。
//   - 修复后：所有 bridge 渠道出站走 service.DeliverBridgeOutbound → 落库 message_hub
//     → 扩展端 GET /api/bridge/outbox 拉取（与 AI 回复同队列）。
//   - 本结构体仅保留：构造器 + EnqueueManualReply（手动发送走 message_hub）+ ReachAdapter
//     接口透传方法（SendSMS/Email/WeCom/...）+ 内部属性 inner + ingress。
//
// 非网页渠道完全转发给 inner（tooluse.IntegrationReachAdapter）；不引入额外行为。
type BridgeReachAdapter struct {
	inner   tooluse.ReachAdapter
	ingress *service.InboxIngressService
}

// NewBridgeReachAdapter 构造桥接触达适配器。
//
// ingress 可选：nil 时 EnqueueManualReply 返回 error。Send* 等旧方法已删除，构造时
// 不再注入 httpReplyBuffer。
func NewBridgeReachAdapter(inner tooluse.ReachAdapter, ingress ...*service.InboxIngressService) *BridgeReachAdapter {
	a := &BridgeReachAdapter{inner: inner}
	if len(ingress) > 0 {
		a.ingress = ingress[0]
	}
	return a
}

// SetIngress 后期注入 ingress（避免装配阶段循环依赖；服务启动时由 router 调用一次）
func (a *BridgeReachAdapter) SetIngress(ingress *service.InboxIngressService) {
	a.ingress = ingress
}

// EnqueueManualReply 人工座席经桥接代发的消息：持久化到 message_hub(direction=outbound, status=pending)，
// 由桥接扩展 GET /api/bridge/outbox 拉取并转发到网页（2026-08-06 三通道架构）。
//
// 替代旧的 EnqueueReply（直接推入 httpReplyBuffer）：ingest 改为即时返回后该 buffer 不再被读取，
// 若人工回复仍走 buffer 会静默丢失。本方法直接落库为待下发消息，与 AI 回复走同一下发队列，保证可靠投递。
func (a *BridgeReachAdapter) EnqueueManualReply(ctx context.Context, channel, accountID, conversationID, content, senderID string) error {
	if a == nil {
		return errors.New("bridge adapter not initialized")
	}
	if a.ingress == nil {
		return errors.New("bridge ingress not initialized")
	}
	return service.DeliverBridgeOutbound(ctx, channel, accountID, conversationID, "text", content, "")
}

func (a *BridgeReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverToOutbox(ctx, "douyin", accountID, openID, msgType, content)
}

func (a *BridgeReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverToOutbox(ctx, "kuaishou", accountID, openID, msgType, content)
}

func (a *BridgeReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverToOutbox(ctx, "xiaohongshu", accountID, openID, msgType, content)
}

func (a *BridgeReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverToOutbox(ctx, "tiktok", accountID, openID, msgType, content)
}

func (a *BridgeReachAdapter) SendXianyu(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverToOutbox(ctx, "xianyu", accountID, openID, msgType, content)
}

func (a *BridgeReachAdapter) deliverToOutbox(ctx context.Context, channel, accountID, openID, msgType, content string) (string, error) {
	if err := service.DeliverBridgeOutbound(ctx, channel, accountID, openID, msgType, content, ""); err != nil {
		return "", err
	}
	return "bridge:" + channel + ":" + accountID + ":" + openID, nil
}

func (a *BridgeReachAdapter) SendSMS(ctx context.Context, phone, content, templateID string, params map[string]string) (string, error) {
	return a.inner.SendSMS(ctx, phone, content, templateID, params)
}

func (a *BridgeReachAdapter) SendEmail(ctx context.Context, to, subject, content string, attachments []string) (string, error) {
	return a.inner.SendEmail(ctx, to, subject, content, attachments)
}

func (a *BridgeReachAdapter) SendWeCom(ctx context.Context, accountID, externalUserID, msgType, content string) (string, error) {
	return a.inner.SendWeCom(ctx, accountID, externalUserID, msgType, content)
}

func (a *BridgeReachAdapter) SendWeixin(ctx context.Context, openID, msgType, content string) (string, error) {
	return a.inner.SendWeixin(ctx, openID, msgType, content)
}

func (a *BridgeReachAdapter) SendDingTalk(ctx context.Context, chatID, msgType, content string) (string, error) {
	return a.inner.SendDingTalk(ctx, chatID, msgType, content)
}

func (a *BridgeReachAdapter) SendTelegram(ctx context.Context, accountID, chatID, content string) (string, error) {
	return a.inner.SendTelegram(ctx, accountID, chatID, content)
}

func (a *BridgeReachAdapter) SendWhatsApp(ctx context.Context, accountID, toPhone, content string) (string, error) {
	return a.inner.SendWhatsApp(ctx, accountID, toPhone, content)
}

func (a *BridgeReachAdapter) SendFeishu(ctx context.Context, accountID, openID, content string) (string, error) {
	return a.inner.SendFeishu(ctx, accountID, openID, content)
}

func (a *BridgeReachAdapter) SendWeb(ctx context.Context, sessionID, content string) (string, error) {
	return a.inner.SendWeb(ctx, sessionID, content)
}

func (a *BridgeReachAdapter) SendCard(ctx context.Context, channel, accountID, externalUserID, cardID string) (string, error) {
	return a.inner.SendCard(ctx, channel, accountID, externalUserID, cardID)
}

func (a *BridgeReachAdapter) Recall(ctx context.Context, channel, msgID string) error {
	return a.inner.Recall(ctx, channel, msgID)
}

func (a *BridgeReachAdapter) AccountHealth(ctx context.Context, channel, accountID string) (*tooluse.AccountHealthInfo, error) {
	return a.inner.AccountHealth(ctx, channel, accountID)
}

// GlobalBridgeReachAdapter 占位全局（reach_sender_wiring.go 引用）
var GlobalBridgeReachAdapter *BridgeReachAdapter

func (a *BridgeReachAdapter) ListAccounts(ctx context.Context, channel string) ([]tooluse.AccountInfo, error) {
	return a.inner.ListAccounts(ctx, channel)
}
