package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hivemtk-user/internal/aiagent/agent/tooluse"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/service"
)

// BridgeReachAdapter 包装 IntegrationReachAdapter：
//
// 网页桥接渠道（douyin/xiaohongshu/tiktok/xianyu/kuaishou）的 AI 回复（2026-08-05 之后）：
//   - HTTP 长轮询模式：所有上行走 POST /api/bridge/ingest，所有下行 AI reply 走
//     httpReplyBuffer + 同请求响应的 outbound_replies；不再维护 WebSocket 长连接。
//   - 链路：WebhookService.sendOutbound → service.RegisterBridgeOutbound → BridgeReachAdapter.Send*
//     → 直接入 httpReplyBuffer → 下次同会话 /api/bridge/ingest 长轮询拿到 reply → 扩展端
//     content/common.js 的 PollingLoop 通过 dispatchOutbound 回写到网页。
//   - 私有化部署单租户，AI 回复频次低（1-5s/次），in-memory 256 容量 FIFO 足够；过期
//     reply 由「长轮询超时」自然清理，无状态机。
//
// 幂等守卫：仍由上层 WebhookService.sendOutbound 通过 ClaimReply 统一保证；本适配器不重复。
//
// 2026-08-05 渠道编码统一：bridge 渠道名 = 平台全名（无 _web 后缀）。
type BridgeReachAdapter struct {
	inner   tooluse.ReachAdapter
	ingress *service.InboxIngressService
	// httpReplyBuffer HTTP 模式 reply 缓冲（跨传输层共享；HTTP ingest 长轮询从该 buffer 拉）
	httpReplyBuffer *httpReplyBuffer
}

// NewBridgeReachAdapter 构造桥接触达适配器（HTTP-only 模式）。
//
// ingress 可选：传入时 delivery 失败会落 message_hub(outbound, status=failed) 等待补发；
// nil 时仅返回错误（用于早期装配阶段）。实际 HTTP 模式下 reply 总能 Push 到 buffer，
// 失败兜底落库通常不会触发；保留参数是为了兼容既有装配代码（router/wiring）。
func NewBridgeReachAdapter(inner tooluse.ReachAdapter, ingress ...*service.InboxIngressService) *BridgeReachAdapter {
	a := &BridgeReachAdapter{inner: inner, httpReplyBuffer: newHTTPReplyBuffer()}
	if len(ingress) > 0 {
		a.ingress = ingress[0]
	}
	return a
}

// SetIngress 后期注入 ingress（避免装配阶段循环依赖；服务启动时由 router 调用一次）
func (a *BridgeReachAdapter) SetIngress(ingress *service.InboxIngressService) {
	a.ingress = ingress
}

// globalReach 保存当前桥接触达适配器，供 SetBridgeReachAdapter 注册出站回调
var globalReach *BridgeReachAdapter

// GlobalBridgeReachAdapter 返回当前已注册的桥接触达适配器。
//
// 用途：bridge_account_controller.SendManual 等"非 WebhookService.sendOutbound 路径"需要
// 直接把 reply 推入 httpReplyBuffer（HTTP 长轮询拉到后再回写到扩展端）。
// 取代旧实现中 bridge.GetBridgeHub().Deliver（WS 模式专用）。
//
// 未注册时返回 nil，调用方须 nil-check。
func GlobalBridgeReachAdapter() *BridgeReachAdapter {
	return globalReach
}

// SetBridgeReachAdapter 注册网页桥接触达适配器，并向 service 包登记出站回调。
//
// 此后 WebhookService.sendOutbound 在桥接渠道（douyin/xiaohongshu/tiktok/xianyu/kuaishou）下，
// 会把 AI reply 落库 message_hub(status=pending)，由桥接扩展 GET /api/bridge/outbox 拉取并回写网页
// （2026-08-06 三通道架构；旧 httpReplyBuffer 长轮询路径已废弃）。
// 通过回调注入（而非 service 直接 import bridge）避免 service -> bridge 导入环。
//
// 2026-08-05 渠道编码统一：bridge 渠道名 = 平台全名（无 _web 后缀）。
func SetBridgeReachAdapter(a *BridgeReachAdapter) {
	globalReach = a
	// 注册 AI 回复出站回调（WebhookService.sendOutbound 桥接分支在 8f4625d 后已直接落库 message_hub，
	// 此处 RegisterBridgeOutbound 回调内部经 Send* → httpReplyBuffer 属遗留接线，仅保留接口兼容）。
	service.RegisterBridgeOutbound(func(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) error {
		switch channel {
		case ChannelDouyinWeb:
			_, err := a.SendDouyin(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelXHSWeb:
			_, err := a.SendXHS(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelTikTok:
			_, err := a.SendTikTok(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelKuaishouWeb:
			_, err := a.SendKuaishou(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelXianyuWeb:
			_, err := a.SendXianyu(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		default:
			return fmt.Errorf("bridge: 不支持的桥接渠道 %q", channel)
		}
	})
}

// deliverHTTP HTTP-only 模式：把 AI reply 入 httpReplyBuffer，等下次 ingest 长轮询拉到。
//
// 步骤：
//  1. content 长度截断：防止超大 XSS payload 撑爆响应（修复 -7）；截断时置 Truncated=true
//     （审计 2026-08-05 P0 修复：原截断后无标记，客户端看到半截消息不知情）
//  2. Push 到 httpReplyBuffer（256 容量 FIFO 淘汰；扩展长轮询 500s 内必拉到）
//
// 幂等守卫仍由上层 sendOutbound 统一处理（ClaimReply）。
func (a *BridgeReachAdapter) deliverHTTP(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) (string, error) {
	if a == nil {
		return "", errors.New("bridge adapter not initialized")
	}
	// 7 防止 XSS 巨大 payload：单条回复限制 4KB
	truncated := false
	if len(content) > maxReplyContentBytes {
		logger.Ctx(ctx).Warn().Str("module", "bridge").Int("orig_bytes", len(content)).Msg("bridge reply content truncated (anti-xss)")
		content = content[:maxReplyContentBytes]
		truncated = true
	}
	reply := &UnifiedReply{
		Channel:        channel,
		AccountID:      accountID,
		ConversationID: conversationID,
		Content:        content,
		MsgType:        msgType,
		ReplyToEventID: eventID,
		Truncated:      truncated,
	}
	if a.httpReplyBuffer == nil {
		return "", errors.New("bridge httpReplyBuffer not initialized")
	}
	a.httpReplyBuffer.Push(reply)
	return "bridge:" + channel + ":" + accountID + ":" + conversationID, nil
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
	h := &model.MessageHub{
		MsgID:          service.ContentHashMsgID(channel, conversationID, content),
		Platform:       channel,
		AccountID:      accountID,
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        "text",
		SenderID:       senderID,
		ReceiverID:     conversationID,
		Content:        content,
		ConversationID: conversationID,
		IsAIReply:      false,
		IsRead:         true,
		SentAt:         time.Now(),
	}
	return a.ingress.DeliverOutbound(ctx, h)
}

// ===== 以下为 ReachAdapter 接口实现 =====
//
// HTTP-only 模式：所有桥接渠道（douyin/xiaohongshu/tiktok/xianyu/kuaishou）的出站走
// deliverHTTP（入 httpReplyBuffer）→ 下次 /api/bridge/ingest 长轮询拉到 → 扩展端回写网页。
// 不再判在线、不再持久化失败：HTTP 模式下 reply 直接入 buffer，没有「扩展离线」概念。

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

// SendDouyin 网页抖音渠道：reply 入 httpReplyBuffer
func (a *BridgeReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverHTTP(ctx, ChannelDouyinWeb, accountID, openID, msgType, content, extractEventID(ctx))
}

// SendKuaishou 网页快手渠道：reply 入 httpReplyBuffer
func (a *BridgeReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverHTTP(ctx, ChannelKuaishouWeb, accountID, openID, msgType, content, extractEventID(ctx))
}

// SendXHS 网页小红书渠道：reply 入 httpReplyBuffer
func (a *BridgeReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverHTTP(ctx, ChannelXHSWeb, accountID, openID, msgType, content, extractEventID(ctx))
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

// SendTikTok 网页 TikTok 渠道：reply 入 httpReplyBuffer
func (a *BridgeReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverHTTP(ctx, ChannelTikTok, accountID, openID, msgType, content, extractEventID(ctx))
}

// SendXianyu 网页闲鱼渠道：reply 入 httpReplyBuffer
func (a *BridgeReachAdapter) SendXianyu(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	return a.deliverHTTP(ctx, ChannelXianyuWeb, accountID, openID, msgType, content, extractEventID(ctx))
}

func (a *BridgeReachAdapter) Recall(ctx context.Context, channel, msgID string) error {
	return a.inner.Recall(ctx, channel, msgID)
}

func (a *BridgeReachAdapter) AccountHealth(ctx context.Context, channel, accountID string) (*tooluse.AccountHealthInfo, error) {
	return a.inner.AccountHealth(ctx, channel, accountID)
}

func (a *BridgeReachAdapter) ListAccounts(ctx context.Context, channel string) ([]tooluse.AccountInfo, error) {
	return a.inner.ListAccounts(ctx, channel)
}

// extractEventID 从 ctx 取出出站事件 ID（由 WebhookService 透传，便于 ClaimReply 幂等）
//
// ctx key: bridge_event_id；调用方通过 WithEventID 注入。
//
// 2026-08-05 修复：原误把 ctxKeyEventID 定义为 const string，无引用价值，纯累赘，
// 删掉以保持文件极简。

// WithEventID 注入出站事件 ID 到 ctx（供 ClaimReply 使用）
func WithEventID(ctx context.Context, eventID string) context.Context {
	if eventID == "" {
		return ctx
	}
	// 用唯一 key 类型避免与其他包冲突
	return context.WithValue(ctx, bridgeEventIDKey{}, eventID)
}

type bridgeEventIDKey struct{}

func extractEventID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(bridgeEventIDKey{}).(string); ok {
		return v
	}
	return ""
}
