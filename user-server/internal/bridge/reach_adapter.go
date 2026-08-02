package bridge

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentruntime "marketing/internal/aiagent/agent/runtime"
	"marketing/internal/aiagent/agent/tooluse"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/service"
)

// BridgeReachAdapter 包装 IntegrationReachAdapter：
//
// 网页桥接渠道（douyin_web/xhs_web/tiktok_web）的回复：
//   1) 若对应账号的扩展在线：经 BridgeHub 通过 WebSocket 投递到 Chrome 扩展（回写网页私信）
//   2) 若扩展离线 / 限速命中 / buffer 满：落 message_hub(direction=outbound, status=failed)，
// 便于坐席 UI 展示和后续补发（修复 -8：离线降级落库）
//   3) 非桥接渠道：直接委托 inner（官方 API），对现有渠道零影响
//
// 幂等守卫：通过 agent_runtime.ClaimReply(eventID) 保证同一 AI 回复仅一次出站，
// 防止 bridge 重连重发时 AI 重复回复（修复 -4：ClaimReply 守卫）
type BridgeReachAdapter struct {
	inner  *tooluse.IntegrationReachAdapter
	hub    *BridgeHub
	ingress *service.InboxIngressService
}

// NewBridgeReachAdapter 构造桥接触达适配器。
//
// ingress 可选：传入时离线/限速/buffer 满会落 message_hub(outbound, status=failed)
// 等待补发；nil 时仅返回错误（用于早期装配阶段）。
func NewBridgeReachAdapter(inner *tooluse.IntegrationReachAdapter, hub *BridgeHub, ingress ...*service.InboxIngressService) *BridgeReachAdapter {
	a := &BridgeReachAdapter{inner: inner, hub: hub}
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

// SetBridgeReachAdapter 注册网页桥接触达适配器，并向 service 包登记出站回调。
//
// 此后 WebhookService.sendOutbound 在桥接渠道（douyin_web/xhs_web/tiktok_web）下，
// 会把 AI 回复经 BridgeHub 通过 WebSocket 投递到 Chrome 扩展，而非走官方 API。
// 通过回调注入（而非 service 直接 import bridge）避免 service -> bridge 导入环。
func SetBridgeReachAdapter(a *BridgeReachAdapter) {
	globalReach = a
	service.RegisterBridgeOutbound(func(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) error {
		switch channel {
		case ChannelDouyinWeb:
			_, err := a.SendDouyin(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelXHSWeb:
			_, err := a.SendXHS(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelTikTokWeb:
			_, err := a.SendTikTok(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		case ChannelKuaishouWeb:
			_, err := a.SendKuaishou(WithEventID(ctx, eventID), accountID, conversationID, msgType, content)
			return err
		default:
			return fmt.Errorf("bridge: 不支持的桥接渠道 %q", channel)
		}
	})
}

// deliverWS 经 WebSocket 下发回复到扩展；扩展离线时降级落 message_hub(status=failed)。
//
// 步骤：
// 1. ClaimReply 幂等：若 eventID 已被认领，直接跳过（修复 -4）
// 2. content 长度截断：防止超大 XSS payload 撑爆 WS 帧（修复 -7）
//  3. hub.Deliver：在线 → 推送；离线/限速/buffer 满 → 降级落 message_hub
//  4. 出站事件 ID 透传到 ctx（trace_id 关联）
func (a *BridgeReachAdapter) deliverWS(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) (string, error) {
	// 4 ClaimReply 幂等守卫：eventID 非空时拒绝重复
	if eventID != "" && !agentruntime.ClaimReply(eventID) {
		logger.Ctx(ctx).Info().Str("module", "bridge").Str("event_id", eventID).Msg("bridge deliver skipped: event already claimed")
		return "bridge:duplicate:" + eventID, nil
	}

	// 7 防止 XSS 巨大 payload：单条回复限制 4KB
	if len(content) > maxReplyContentBytes {
		logger.Ctx(ctx).Warn().Str("module", "bridge").Int("orig_bytes", len(content)).Msg("bridge reply content truncated (anti-xss)")
		content = content[:maxReplyContentBytes]
	}

	reply := &UnifiedReply{
		Channel:        channel,
		AccountID:      accountID,
		ConversationID: conversationID,
		Content:        content,
		MsgType:        msgType,
		ReplyToEventID: eventID,
	}
	if err := a.hub.Deliver(channel, accountID, reply); err != nil {
		// 离线/限速/buffer 满：落 message_hub(direction=outbound, status=failed) 等待补发
		// 私域: 无 Prometheus, 失败已落库（用 classifyDeliverErr 区分失败类型，便于排查）
		errClass := classifyDeliverErr(err)
		logger.Ctx(ctx).Warn().Str("module", "bridge").Str("deliver_err_class", errClass).
			Str("channel", channel).Str("account_id", accountID).Str("event_id", eventID).
			Msg("bridge deliver failed, classifying error for fallback persist")
		return a.persistFailedOutbound(ctx, channel, accountID, conversationID, msgType, content, eventID, err)
	}
	return "bridge:" + channel + ":" + accountID + ":" + conversationID, nil
}

// persistFailedOutbound 出站失败时降级落库（修复 -8：离线降级落库）
//
// 将 outbound 消息落 message_hub 并标记 status=failed / 失败原因；坐席 UI 可据此补发。
func (a *BridgeReachAdapter) persistFailedOutbound(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string, deliverErr error) (string, error) {
	logger.Ctx(ctx).Warn().Err(deliverErr).Str("module", "bridge").
		Str("channel", channel).Str("account_id", accountID).
		Str("event_id", eventID).
		Msg("bridge deliver failed, persisting to message_hub for later retry")

	// eventID 为空时构造合成 ID（保证幂等 key 非空，避免 DB 冲突）
	if eventID == "" {
		eventID = fmt.Sprintf("bridge-failed-%s-%d", accountID, time.Now().UnixNano())
	}
	if a.ingress == nil {
		// 无 ingress 时仅返回错误，便于上层记录 metric
		return "", fmt.Errorf("bridge deliver failed: %w (no ingress to persist)", deliverErr)
	}
	event := &model.MessageEvent{
		EventID:        eventID,
		SessionID:      channel + ":" + accountID + ":" + conversationID,
		Channel:        channel,
		SenderID:       accountID,
		MsgType:        msgType,
		Content:        content,
		ConversationID: conversationID,
		Timestamp:      time.Now(),
		Extra: map[string]any{
			"account_id":  accountID,
			"bridge":      true,
			"outbound":    true,
			"status":      "failed",
			"deliver_err": deliverErr.Error(),
		},
	}
	if perr := a.ingress.PersistBridgeHistory(ctx, event, "outbound"); perr != nil {
		logger.Ctx(ctx).Error().Err(perr).Str("module", "bridge").Str("event_id", eventID).Msg("bridge failed-outbound persist also failed")
		return "", errors.Join(deliverErr, perr)
	}
	// 返回带 status 标记的占位 ID，调用方可通过 metric 统计失败率
	return "bridge:failed:" + channel + ":" + accountID, deliverErr
}

// ===== 以下为 ReachAdapter 接口实现 =====

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

// SendDouyin 网页抖音渠道：
//   - 扩展在线：经 BridgeHub WS 下发
//   - 扩展离线：降级落 message_hub(outbound, status=failed)
func (a *BridgeReachAdapter) SendDouyin(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	if a.hub.IsOnline(ChannelDouyinWeb, accountID) {
		return a.deliverWS(ctx, ChannelDouyinWeb, accountID, openID, msgType, content, extractEventID(ctx))
	}
	// 扩展离线：降级落 message_hub(outbound, status=failed) 等待坐席补发，而非走官方 API
	return a.persistFailedOutbound(ctx, ChannelDouyinWeb, accountID, openID, msgType, content, extractEventID(ctx), ErrBridgeOffline)
}

// SendKuaishou 网页快手渠道：
//   - 扩展在线：经 BridgeHub WS 下发
//   - 扩展离线：降级落 message_hub(outbound, status=failed) 等待坐席补发，而非走官方 API
func (a *BridgeReachAdapter) SendKuaishou(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	if a.hub.IsOnline(ChannelKuaishouWeb, accountID) {
		return a.deliverWS(ctx, ChannelKuaishouWeb, accountID, openID, msgType, content, extractEventID(ctx))
	}
	return a.persistFailedOutbound(ctx, ChannelKuaishouWeb, accountID, openID, msgType, content, extractEventID(ctx), ErrBridgeOffline)
}

// SendXHS 网页小红书渠道：离线降级落库
func (a *BridgeReachAdapter) SendXHS(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	if a.hub.IsOnline(ChannelXHSWeb, accountID) {
		return a.deliverWS(ctx, ChannelXHSWeb, accountID, openID, msgType, content, extractEventID(ctx))
	}
	// 扩展离线：降级落 message_hub(outbound, status=failed) 等待坐席补发，而非走官方 API
	return a.persistFailedOutbound(ctx, ChannelXHSWeb, accountID, openID, msgType, content, extractEventID(ctx), ErrBridgeOffline)
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

// SendTikTok 网页 TikTok 渠道：离线降级落库
func (a *BridgeReachAdapter) SendTikTok(ctx context.Context, accountID, openID, msgType, content string) (string, error) {
	if a.hub.IsOnline(ChannelTikTokWeb, accountID) {
		return a.deliverWS(ctx, ChannelTikTokWeb, accountID, openID, msgType, content, extractEventID(ctx))
	}
	// 扩展离线：降级落 message_hub(outbound, status=failed) 等待坐席补发，而非走官方 API
	return a.persistFailedOutbound(ctx, ChannelTikTokWeb, accountID, openID, msgType, content, extractEventID(ctx), ErrBridgeOffline)
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
const ctxKeyEventID = "bridge_event_id"

// classifyDeliverErr 分类 Deliver 错误类型（用于 metrics 区分限速/离线/buffer 满）
func classifyDeliverErr(err error) string {
	switch {
	case errors.Is(err, ErrBridgeOffline):
		return "offline"
	case errors.Is(err, ErrBridgeRateLimited):
		return "rate_limited"
	case errors.Is(err, ErrBridgeBufferFull):
		return "buffer_full"
	default:
		return "other"
	}
}

// WithEventID 注入出站事件 ID 到 ctx（供 ClaimReply 使用）
func WithEventID(ctx context.Context, eventID string) context.Context {
	if eventID == "" {
		return ctx
	}
	type k struct{}
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
