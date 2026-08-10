// 拆分自 webhook.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
	"io"
	"strconv"
	"strings"
	"time"
)

func (s *WebhookService) sendOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub, cards []model.RichCard) {
	// 幂等守卫：与 AgentRuntime 事件总线订阅共享同一 EventID 守卫。
	// 同一 EventID 仅首条链路出站，杜绝重复消息；同时防御 webhook 平台重复投递
	// （同 EventID 二次到达）导致的重复出站。
	if !agent_runtime.ClaimReply(p.EventID) {
		logger.Ctx(ctx).Info().Str("event_id", p.EventID).Msg("skip duplicate outbound (event already replied)")
		// 可观测性：记录被幂等守卫拦截的重复出站 (私域: 无 Prometheus, 仅日志)
		return
	}
	// 出站结果追踪：仅当真正成功出站时才保留认领（防重复）；
	// 若全线出站失败则释放认领，允许平台重投在本实例内重试。
	sent := false
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer func() {
		if !sent {
			agent_runtime.ReleaseReply(p.EventID)
		}
		cancel()
	}()
	switch channel {
	case ChannelWeCom:
		// 企微出站底层 = WeComIntegrationService（与 IntegrationReachAdapter.SendWeCom 共享同一底层， 已收敛为单一出站入口）
		if s.integration == nil {
			return
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		if _, err := s.integration.SendMessage(ctx, &WeComSendRequest{
			AccountID:      uint(accID),
			ExternalUserID: p.Sender,
			MsgType:        "text",
			Content:        content,
			IsAIReply:      true,
			AIAgent:        "sales_engine",
		}); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "wecom").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelFeishu:
		if s.feishuIntegration == nil {
			s.feishuIntegration = NewFeishuIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// Feishu：个人消息用 open_id 作为发送目标；群消息必须用群 chat_id（open_chat_id），
		// 否则会被当成私信发给用户而非在群里回复。
		target := p.Sender
		idType := "open_id"
		if hubMsg != nil && hubMsg.IsGroup && hubMsg.GroupID != "" {
			target = hubMsg.GroupID
			idType = "open_chat_id"
		}
		if err := s.feishuIntegration.SendMessage(ctx, uint(accID), target, content, idType); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "feishu").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelTelegram:
		if s.tgIntegration == nil {
			s.tgIntegration = NewTelegramIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// 解析 chat_id
		var chatID int64
		if hubMsg != nil && hubMsg.ConversationID != "" {
			chatID, _ = strconv.ParseInt(hubMsg.ConversationID, 10, 64)
		}
		if chatID == 0 {
			return
		}
		if err := s.tgIntegration.SendMessage(ctx, uint(accID), chatID, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "telegram").Str("account_id", accountID).Int64("chat_id", chatID).Msg("outbound failed")
		} else {
			sent = true
		}
		// 结构化富卡片：随文本一并下发（Telegram 以 inline keyboard 按钮呈现）
		for _, card := range cards {
			if err := s.tgIntegration.SendCard(ctx, uint(accID), chatID, &card); err != nil {
				logger.Ctx(ctx).Error().Err(err).Str("channel", "telegram").Int64("chat_id", chatID).Msg("outbound card failed")
			} else {
				sent = true
			}
		}
	case ChannelWhatsapp:
		if s.waIntegration == nil {
			s.waIntegration = NewWhatsAppCloudIntegrationService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil || accID == 0 {
			return
		}
		// 收件人是手机号 (E.164)
		if err := s.waIntegration.SendMessage(ctx, uint(accID), p.Sender, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "whatsapp").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelDouyin, ChannelXiaohongshu, ChannelTiktok, ChannelXianyu, ChannelKuaishou:
		// 网页桥接渠道：AI 回复经 HTTP 长轮询投递到 Chrome 扩展（由 bridge 包注册的回调完成），
		// 不走官方 API（避免把私信误发到平台开放接口）。
		// p.EventID 传给 bridge 用于 ClaimReply 幂等守卫。
		// 2026-08-05 渠道编码统一：去掉 _web 后缀，case 改为全名（douyin/xiaohongshu/tiktok/xianyu/kuaishou）。
		//
		// 2026-08-05 修复：AI 回复持久化到 message_hub（direction=outbound, is_ai_reply=true）。
		//   原版只 DeliverBridgeOutbound 入 httpReplyBuffer，不落库 → 统一收件箱看不到 AI 回复。
		//   扩展端回写网页后 MutationObserver 会把 AI 回复当客户消息上行 → direction=inbound 的错误记录。
		//   修复：在出站前先持久化 AI 回复（direction=outbound），扩展端上行时靠 event_id 去重跳过。
		//
		// 2026-08-05 根因修复（用户指定方案：消息ID用内容hash）：
		//   原版 MsgID 用 `bridge-out-${UnixNano}`（纳秒时间戳），每次都不同，
		//   前端扫描 AI 回复消息生成的 event_id 与之完全不一致 → GetByMsgID 查不到 → 当新消息入库 → 触发 AI → 回环。
		//   改用 ContentHashMsgID(channel, conversationID, content)，与前端 contentHash 算法一致，
		//   前端扫描 AI 回复消息时生成的 event_id = ContentHashMsgID → 与 DB msg_id 相同 → 去重跳过，彻底解决回环。
		if hubMsg != nil {
			outMsg := &model.MessageHub{
				MsgID:          ContentHashMsgID(string(channel), hubMsg.ConversationID, content),
				Platform:       string(channel),
				AccountID:      accountID,
				Direction:      "outbound",
				Status:         "pending", // 显式置为 pending：确保 ListPendingOutbound 的 status='pending' 过滤能命中；不依赖 GORM default 标签（零值字符串不一定触发 DB 默认）
				MsgType:        "text",
				SenderID:       accountID,
				ReceiverID:     hubMsg.SenderID,
				Content:        content,
				ConversationID: hubMsg.ConversationID,
				IsGroup:        hubMsg.IsGroup,
				GroupID:        hubMsg.GroupID,
				IsAIReply:      true,
				AIAgent:        "sales_engine",
				IsRead:         true,
				SentAt:         time.Now(),
			}
			outMsg.TraceID = tracing.LinkOutboundTraceID(ctx, hubMsg.ConversationID)
			// 统一收件去重哈希（渠道+发送者+内容）：出站行的发送者键归一为账号(platform 身份)，
			// 与 interceptInbound 对回显消息(senderKey 同为账号)的计算一致 → 回显时哈希匹配被识别为"自己消息"。
			outMsg.DedupHash = ContentHashWithSender(string(channel), accountID, content)
			// 2026-08-07 根因修复（pending 堆积治理）：桥接 AI 回复落库前拦截「不可达目标」。
			// 占位账号(<channel>-unknown)与昵称派生会话(conv:<名>)永远无法被扩展投递，
			// enqueue 到 pending 队列只会成为永久孤儿、污染下发监控。改为标记 failed（统一收件箱仍可见）。
			if undeliverable, reason := bridgeOutboundUndeliverable(accountID, outMsg.ConversationID); undeliverable {
				outMsg.Status = "failed"
				outMsg.Extra = model.JSONMap{"undeliverable_reason": reason}
				logger.Ctx(ctx).Warn().
					Str("module", "bridge").
					Str("channel", string(channel)).
					Str("account_id", accountID).
					Str("conversation_id", outMsg.ConversationID).
					Str("reason", reason).
					Msg("bridge outbound target undeliverable; marked failed instead of pending")
			}
			// 2026-08-07 修复：ContentHashMsgID 不含 conversationID（patrol 回环去重需要），
			// 同渠道同内容不同会话的 AI 回复 msg_id 相同 → DB 唯一约束冲突 → 第二条静默丢失。
			// 修复：Create 唯一约束冲突时追加 conversation_id 后缀重试，保证不丢消息。
			// patrol 回环不受影响：contentHash 不含 convID → 仍与首条 msg_id 匹配 → 跳过。
			persisted := outMsg
			if err := s.db.Create(outMsg).Error; err != nil {
				retryMsg := &model.MessageHub{
					MsgID:          outMsg.MsgID + ":" + hubMsg.ConversationID,
					Platform:       outMsg.Platform,
					AccountID:      outMsg.AccountID,
					Direction:      "outbound",
					Status:         outMsg.Status, // 继承不可达守卫结果（failed/pending）
					MsgType:        outMsg.MsgType,
					SenderID:       outMsg.SenderID,
					ReceiverID:     outMsg.ReceiverID,
					Content:        outMsg.Content,
					ConversationID: outMsg.ConversationID,
					IsGroup:        outMsg.IsGroup,
					GroupID:        outMsg.GroupID,
					IsAIReply:      outMsg.IsAIReply,
					AIAgent:        outMsg.AIAgent,
					IsRead:         outMsg.IsRead,
					SentAt:         outMsg.SentAt,
					TraceID:        outMsg.TraceID,
					DedupHash:      outMsg.DedupHash, // 与 outMsg 完全一致（同渠道+同账号+同内容）
				}
				if err2 := s.db.Create(retryMsg).Error; err2 != nil {
					logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Str("channel", string(channel)).Msg("failed to persist bridge outbound reply to message_hub")
				} else {
					persisted = retryMsg
				}
			}
			if s.inboxConvRepo != nil && persisted.ID != 0 {
				// 同步 inbox_conversations 的 last_message_preview
				if err := s.inboxConvRepo.UpsertFromMessage(ctx, repository.UpsertFromMessageInput{
					Platform:           persisted.Platform,
					AccountID:          persisted.AccountID,
					CustomerID:         persisted.ReceiverID,
					ConversationID:     persisted.ConversationID,
					LastMessageID:      persisted.ID,
					LastMessagePreview: persisted.Content,
					LastMessageAt:      persisted.SentAt,
					LastMessageFrom:    "ai",
				}); err != nil {
					logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Msg("failed to upsert inbox_conversations for outbound")
				}
			}
			// 节点3 出站入队：AI 回复落库 outbox；目标不可达时 status=failed（异常）
			enqueueAbnormal := ""
			enqueueStatus := tracing.StatusOk
			if persisted.Status == "failed" {
				enqueueStatus = tracing.StatusAbnormal
				enqueueAbnormal = "目标不可达（占位账号/未知账号），标记 failed 而非 pending，避免污染下行出库队列"
			}
			// 出站追踪载体随 ctx 透传，确保 span 携带渠道/账号维度
			ctx = tracing.WithCarrier(ctx, &tracing.Carrier{
				TraceID:        persisted.TraceID,
				ConversationID: persisted.ConversationID,
				AccountID:      persisted.AccountID,
				Channel:        persisted.Platform,
			})
			// 节点3 出站入队：流式 API，异步非阻塞（不阻断业务主链路）
			outSpan := tracing.Start(ctx, tracing.NodeOutboundEnqueue).
				Input(map[string]any{
					"channel":     string(channel),
					"account_id":  accountID,
					"conv_id":     persisted.ConversationID,
					"content_len": len(persisted.Content),
					"is_ai_reply": persisted.IsAIReply,
				}).
				Expected("AI 回复落库 message_hub(status=pending)，进入下行出库队列").
				MsgID(persisted.MsgID)
			outSpan.Output(map[string]any{
				"msg_id": persisted.MsgID,
				"status": persisted.Status,
				"id":     persisted.ID,
			})
			var outSpanErr error
			if enqueueStatus == tracing.StatusAbnormal {
				outSpanErr = fmt.Errorf("%s", enqueueAbnormal)
			}
			outSpan.End(nil, outSpanErr)
		}
		// 2026-08-06 架构重构：AI 回复已落 message_hub(status=pending) 作为下发队列，
		// 由桥接扩展独立轮询 GET /api/bridge/outbox 拉取并转发到网页，
		// 不再依赖内存 httpReplyBuffer 长轮询（易丢消息、重启即丢）。
		// 标记 sent=true 以记录已出站历史；实际下发由 downlink 通道完成并 ack 确认。
		sent = true
	default:
		// 该渠道暂未实现主动出站：记录日志并跳过，避免静默吞掉消息难以排查
		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Str("account_id", accountID).Msg("unsupported outbound channel, skipped")
	}

	// 2026-08-05 释放 "AI 处理中" 标记（AI 回复已落库/发送完成）
	//   防止"不断发消息"机制：标记存在期间跳过 AI 触发，AI 完成后释放
	if s.ingressSvc != nil && hubMsg != nil && hubMsg.ConversationID != "" {
		s.ingressSvc.ReleaseAIProcessingFlag(ctx, hubMsg.ConversationID)
		// 极限场景修复：异步重检查 AI 推理期间遗漏的未回复客户消息
		//   时序：用户消息1触发AI → AI推理中 → 用户消息2入库但被 ai_processing 标记跳过
		//   → AI回复消息1 → 释放标记 → 消息2成为孤儿
		//   修复：释放标记后延迟 800ms 重检查，若有未回复消息则补触发
		//   用 WithoutCancel context 避免 sendOutbound 的 15s timeout 限制
		go s.ingressSvc.RecheckUnrepliedAndTrigger(context.WithoutCancel(ctx), hubMsg.ConversationID, "")
	}
}

// bridgeOutboundUndeliverable 判断桥接 AI 回复的目标是否可达。
// 仅「占位账号」永远不可达，enqueue 到 pending 队列只会成为永久孤儿、污染下发监控：
//
//	占位账号：前端账号解析失败时兜底用 <channel>-unknown 作为 account_id 上报，
//	真实账号解析后扩展用真实 id 轮询 outbox，永不拉取 -unknown；unknown 状态下也无法投递到具体会话。
//
// 注意：conv:<名> 昵称派生会话【不再】在此拦截——前端 openConversation 现已支持按列表项 name
// 匹配点击打开（见 bridge/src/core/channel-adapter.js），可尽力投递；真正打不开的会留 pending 由
// 监控归类为「待观察」，下一轮 downlink 仍可重试，不会永久丢失。
func bridgeOutboundUndeliverable(accountID, conversationID string) (bool, string) {
	if accountID != "" && strings.HasSuffix(accountID, "-unknown") {
		return true, "placeholder-account"
	}
	return false, ""
}

// =================== 工具 ===================

// isDuplicate 基于 eventID 的 TTL 幂等。
// 业务需要：外部渠道事件必须「恰好一次」处理。多实例下若各持进程内去重表，
// 重复投递会被不同实例各自放过 → 双处理。故改走全局缓存 SetNX：
//   - REDIS_HOST 配置时为 Redis 共享后端（跨实例去重）
//   - 否则为内存单例（单实例安全）
//
// TTL 内重复 key 已存在即命中返回 true；SetNX 异常时放行并告警（可用性优先）。
// 使用 context.Background() 而非入参 ctx：Bridge WS 生命周期短（重连循环会 cancel ctx），
// 而去重是基础设施功能，不应受连接生命周期影响。详见 trace ad589b80 双 orchestrator 根因。
func (s *WebhookService) isDuplicate(ctx context.Context, eventID string) bool {
	if eventID == "" {
		return false
	}
	key := "mtk:webhook:dedup:" + eventID
	set, err := cache.GetGlobalCache().SetNX(context.Background(), key, "1", WebhookDedupTTL)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Str("event_id", eventID).Msg("[webhook] dedup 后端异常，放行")
		return false
	}
	if !set {
		// 可观测性：命中去重的重复投递 (私域: 无 Prometheus, 仅日志)
		logger.Ctx(ctx).Debug().Str("event_id", eventID).Msg("[webhook] dedup hit")
		return true
	}
	return false
}

func (s *WebhookService) allowRate(ctx context.Context, key string) bool {
	s.rlMu.Lock()
	b, ok := s.rlBuckets[key]
	if !ok {
		b = &tokenBucket{
			capacity:   WebhookRateBurst,
			refillRate: float64(WebhookRateLimit),
			tokens:     float64(WebhookRateBurst),
			lastRefill: time.Now(),
			lastAccess: time.Now(),
		}
		s.rlBuckets[key] = b
	}
	b.lastAccess = time.Now()
	s.rlMu.Unlock()
	return b.allow(context.Background())
}

// startRLJanitor 定期清理 idle 超过 5 分钟的限速桶（防内存泄漏）。
// tokenBucket 按 (channel, accountID) 为 key，若无清理则永久增长。
func (s *WebhookService) startRLJanitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.rlMu.Lock()
				cutoff := time.Now().Add(-5 * time.Minute)
				for k, b := range s.rlBuckets {
					if b.lastAccess.Before(cutoff) {
						delete(s.rlBuckets, k)
					}
				}
				s.rlMu.Unlock()
			}
		}
	}()
}

func (s *WebhookService) generateEventID(ctx context.Context, channel WebhookChannel, accountID string, body []byte) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + string(body)))
	return fmt.Sprintf("evt_%s", hex.EncodeToString(h[:8]))
}

func (s *WebhookService) genMessageID(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload) string {
	h := sha256.Sum256([]byte(string(channel) + ":" + accountID + ":" + p.Sender + ":" + p.Content + ":" + p.EventID))
	// UnifiedMessage.MessageID 列宽为 varchar(50)，"msg_" 前缀 + 完整 64 hex 会超长。
	// 截断为前 22 hex 字符（共 26 字符，留足唯一空间：2^88）。
	return fmt.Sprintf("msg_%s", hex.EncodeToString(h[:])[:22])
}

// TruncateForStore 截断防止 raw_data 过大
func (s *WebhookService) TruncateForStore(ctx context.Context, body []byte) string {
	const max = 64 * 1024
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "...[truncated]"
}

func (s *WebhookService) getAccountSecret(ctx context.Context, platform, accountID string) (string, error) {
	if s.accountRepo == nil {
		return "", nil
	}
	// 防止全局 nil db 触发 panic
	if s.db == nil {
		return "", nil
	}
	acc, err := s.accountRepo.GetByPlatform(ctx, platform)
	if err != nil {
		return "", nil
	}
	return acc.APISecret, nil
}

// getTelegramWebhookSecret 获取 Telegram Bot 的 webhook secret
// secret 来自 TelegramAccount.WebhookSecret（在 setWebhook 时由商户配置）
// 未配置时返回空字符串，调用方应跳过验签
func (s *WebhookService) getTelegramWebhookSecret(ctx context.Context, accountID string) string {
	if s.telegramRepo == nil || s.db == nil {
		return ""
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return ""
	}
	acc, err := s.telegramRepo.GetByID(ctx, uint(accID))
	if err != nil {
		return ""
	}
	return acc.WebhookSecret
}

func (s *WebhookService) getWechatSecrets(ctx context.Context, accountID string) (string, string) {
	return "", ""
}

// GetWeComSecrets 公开方法：供 controller 层 URL 验证使用
func (s *WebhookService) GetWeComSecrets(ctx context.Context, accountID string) (string, string, error) {
	return s.getWeComSecrets(ctx, accountID)
}

// getWeComSecrets 从 wecom_accounts 读取 token + EncodingAESKey
// accountID 优先按数字 ID 解析；解析失败则取第一条启用 webhook 的账号
func (s *WebhookService) getWeComSecrets(ctx context.Context, accountID string) (string, string, error) {
	if s.wecomRepo == nil {
		return "", "", errors.New("wecomRepo nil")
	}
	if s.db == nil {
		return "", "", nil
	}
	// 1) 按 ID 解析
	if id, err := strconv.ParseUint(accountID, 10, 64); err == nil && id > 0 {
		acc, err := s.wecomRepo.GetByID(ctx, uint(id))
		if err == nil && acc != nil {
			return acc.CallbackToken, acc.EncodingAESKey, nil
		}
	}
	// 2) 兜底：取第一个启用 webhook 的账号
	accs, err := s.wecomRepo.GetByMerchant(ctx)
	if err != nil {
		return "", "", err
	}
	for _, a := range accs {
		if a.WebhookEnabled {
			return a.CallbackToken, a.EncodingAESKey, nil
		}
	}
	if len(accs) > 0 {
		return accs[0].CallbackToken, accs[0].EncodingAESKey, nil
	}
	return "", "", errors.New("wecom account not found")
}

// PendingCount 待处理事件数
func (s *WebhookService) PendingCount(ctx context.Context) int64 {
	if s.eventRepo == nil {
		return 0
	}
	c, _ := s.eventRepo.CountUnprocessed(ctx)
	return c
}

// QueueLen 队列长度
func (s *WebhookService) QueueLen(ctx context.Context) int { return len(s.queue) }

// ReadAll 读取请求体
func ReadAll(r io.Reader) ([]byte, error) { return io.ReadAll(r) }

// helpers
func getString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}
