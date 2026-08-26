package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

// bridgeChannels 网页桥接渠道白名单。
//
// B-5 单源化（2026-08-26）：本包不再手工维护渠道清单，运行时由 bridge 包 init()
// 从 channelgw.Default（权威注册表）取值注入（SetBridgeChannels），
// 消除双份清单漂移。service → channelgw 存在反向依赖故无法直接 import，
// 注入时序由包初始化顺序保证（bridge import channelgw，其 init 先于 main）。
var bridgeChannels = map[string]struct{}{}

// SetBridgeChannels 用 channelgw.Default 的渠道名集合重建白名单（B-5 运行时单一来源）。
// 由 bridge 包初始化时调用一次；names 为空时忽略（防止误清空白名单）。
func SetBridgeChannels(names []string) {
	if len(names) == 0 {
		logger.Errorf("[B-5] SetBridgeChannels called with empty names; keep previous whitelist")
		return
	}
	m := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n == "" {
			continue
		}
		m[n] = struct{}{}
	}
	if len(m) == 0 {
		return
	}
	bridgeChannels = m
}

// errBridgeOutboundNotReady 桥接出站未装配（router.Setup 尚未注入 InboxIngressService）。
//
// 历史：早期曾以 RegisterBridgeOutbound 全局回调注入；为消除 callback 间接层 + 与 AI reply
// 走同一 InboxIngressService.DeliverOutbound 路径，现改为直接持有入站服务单例。
// 私有化部署单租户场景下装配时序固定，启动后必非 nil。
var errBridgeOutboundNotReady = errors.New("bridge outbound not ready: InboxIngressService not registered")

// globalInboxIngressService 全局入站服务引用（router.Setup 在装配完成后注入）。
//
// 单一源：app.SetBridgeIngressSvc 会同步设置本变量；本包内 DeliverBridgeOutbound
// 走直接方法调用而非 callback，避免 service → bridge 导入环。
var globalInboxIngressService *InboxIngressService

// GlobalSSEPublisher SSE 事件通知回调（由 bridge 包在初始化时设置）。
//
// 解耦：service 包不能 import bridge（避免循环依赖 bridge → service → bridge），
// 故通过全局函数回调通知 bridge 包的 SSEBus.Publish。回调在独立 goroutine 中执行，
// 不阻塞 DeliverBridgeOutbound 主路径。
//
// 参数：
//   - channel, accountID: 渠道和账号标识
//   - hubID: message_hub 表主键（用于 SSE id/Last-Event-ID）
//   - convID: 会话 ID（用于前端路由）
//   - msgType: 消息类型 (text/image/voice etc)
//   - receiverID: 接收者 ID
//   - content: 消息内容
//   - isAIReply: 是否为 AI 回复
//   - createdAt: 创建时间
var GlobalSSEPublisher func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time)

// SetGlobalSSEPublisher 由 bridge 包在初始化时调用，注入 SSE 通知回调
func SetGlobalSSEPublisher(fn func(channel, accountID string, hubID uint64, convID, msgType, receiverID, content string, isAIReply bool, createdAt time.Time)) {
	GlobalSSEPublisher = fn
	logger.Info("[SSE] GlobalSSEPublisher registered successfully")
}

// SetGlobalInboxIngressService 由 router.Setup 在装配完成后调用一次。
func SetGlobalInboxIngressService(s *InboxIngressService) { globalInboxIngressService = s }

// GlobalInboxIngressService 读取桥接入站服务（装配前为 nil）
func GlobalInboxIngressService() *InboxIngressService { return globalInboxIngressService }

// DeliverBridgeOutbound 桥接渠道出站统一入口：构造 MessageHub 并持久化到 outbox，
// 由桥接扩展 GET /api/bridge/outbox 拉取后转发到网页（2026-08-06 三通道架构）。
//
// 替代已废弃的内存 httpReplyBuffer 长轮询路径——2026-08-06 后 buffer 不再被任何调用方读取，
// 走 buffer 会导致人工外联/主动触达消息静默丢失。本方法直接落库 message_hub(status=pending)，
// 与 AI 回复（webhook_outbound.go sendOutbound 桥接分支）走同一下发队列，保证可靠投递。
//
// 调用方：
//   - service/proactive_reach.go::sendBridge     主动外联（修复 proactive_reach 走死通道 bug）
//   - service/douyin_integration.go::SendMessage  抖音主动私聊（修复同 bug）
//   - tooluse.RegisterBridgeOutboundDeliver      AI 智能体 reach.*.send 工具的 bridge 分支
//
// 入参约定：
//   - accountID:  本账号在平台的标识（私有化部署由扩展在 ingest 时上报）
//   - conversationID: 客户会话 ID（与 ingest 端 convention 一致）
//   - eventID:  仅用于日志/tracing，不作为 msg_id（msg_id 由 content 哈希生成）
//   - 返回 error: nil=落库成功；非 nil=落库失败（不进入 outbox 队列）
func DeliverBridgeOutbound(ctx context.Context, channel, accountID, conversationID, msgType, content, eventID string) error {
	if globalInboxIngressService == nil {
		return errBridgeOutboundNotReady
	}
	if channel == "" || accountID == "" || conversationID == "" {
		return fmt.Errorf("bridge outbound: channel/account_id/conversation_id required")
	}
	if _, ok := bridgeChannels[channel]; !ok {
		return fmt.Errorf("bridge: 不支持的桥接渠道 %q", channel)
	}
	if undeliverable, reason := bridgeOutboundUndeliverable(accountID, conversationID); undeliverable {
		logger.Ctx(ctx).Warn().
			Str("module", "bridge").
			Str("channel", channel).
			Str("account_id", accountID).
			Str("conversation_id", conversationID).
			Str("reason", reason).
			Msg("bridge outbound target undeliverable; rejected before enqueue")
		return fmt.Errorf("bridge outbound undeliverable: %s", reason)
	}
	h := &model.MessageHub{
		MsgID:          ContentHashMsgID(channel, conversationID, content),
		Platform:       channel,
		AccountID:      accountID,
		Direction:      "outbound",
		Status:         "pending",
		MsgType:        msgType,
		SenderID:       accountID,
		ReceiverID:     conversationID,
		Content:        content,
		ConversationID: conversationID,
		IsAIReply:      false,
		IsRead:         true,
		SentAt:         time.Now(),
	}
	if eventID != "" {
		logger.Ctx(ctx).Debug().
			Str("module", "bridge").
			Str("event_id", eventID).
			Str("msg_id", h.MsgID).
			Msg("bridge outbound enqueue with upstream event_id (for tracing only)")
	}
	if err := globalInboxIngressService.DeliverOutbound(ctx, h); err != nil {
		logger.Ctx(ctx).Error().Err(err).
			Str("module", "bridge").
			Str("channel", channel).
			Str("account_id", accountID).
			Str("conversation_id", conversationID).
			Msg("bridge outbound DeliverOutbound failed")
		return err
	}

	// Phase 1: SSE 事件通知（异步，不阻塞主路径）
	if GlobalSSEPublisher != nil {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Errorf("[SSE] GlobalSSEPublisher panic recovered: %v", r)
				}
			}()
			GlobalSSEPublisher(
				channel, accountID,
				uint64(h.ID),
				h.ConversationID,
				h.MsgType,
				h.ReceiverID,
				h.Content,
				h.IsAIReply,
				h.CreatedAt,
			)
		}()
	}

	return nil
}

// bridgeOutboundUndeliverable 判断桥接出站目标是否可达。
//
// 拦截两类不可达目标：
//   - 占位账号 `<channel>-unknown`：前端账号解析失败时兜底上报，真实账号解析后扩展用真实 id 轮询 outbox，
//     永不拉取 -unknown；unknown 状态下无法投递到具体会话。
//   - 派生于昵称的兜底会话 `conv:<name>`：前端 openConversation 按列表项 name 匹配点击打开已能尽力投递；
//     真正打不开的会留 pending 由监控归类为"待观察"，下一轮 downlink 仍可重试，不算硬失败。
//
// 因此本函数仅拦截「占位账号」一种情况。
//
// 返回 (undeliverable, reason)：reason 仅在 undeliverable=true 时非空，供 webhook_outbound 写入
// outbox.Extra["undeliverable_reason"] 供后续排查 / 监控。
func bridgeOutboundUndeliverable(accountID, conversationID string) (bool, string) {
	if accountID != "" && strings.HasSuffix(accountID, "-unknown") {
		return true, "placeholder-account"
	}
	return false, ""
}

// isBridgeChannel 检查渠道是否在网页桥接白名单
func isBridgeChannel(channel string) bool {
	_, ok := bridgeChannels[channel]
	return ok
}
