package service

import (
	"context"

	"fmt"

	"strconv"

	"strings"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/repository"
)

func (s *WebhookService) sendOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub, cards []model.RichCard) {

	if !agent_runtime.ClaimReply(p.EventID) {
		logger.Ctx(ctx).Info().Str("event_id", p.EventID).Msg("skip duplicate outbound (event already replied)")

		return
	}

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

		if err := s.waIntegration.SendMessage(ctx, uint(accID), p.Sender, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "whatsapp").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelDouyin, ChannelXiaohongshu, ChannelTiktok, ChannelXianyu, ChannelKuaishou:

		if hubMsg != nil {
			outMsg := &model.MessageHub{
				MsgID:          ContentHashMsgID(string(channel), hubMsg.ConversationID, content),
				Platform:       string(channel),
				AccountID:      accountID,
				Direction:      "outbound",
				Status:         "pending",
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

			outMsg.DedupHash = ContentHashWithSender(string(channel), accountID, content)

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

			persisted := outMsg
			if err := s.db.Create(outMsg).Error; err != nil {
				retryMsg := &model.MessageHub{
					MsgID:          outMsg.MsgID + ":" + hubMsg.ConversationID,
					Platform:       outMsg.Platform,
					AccountID:      outMsg.AccountID,
					Direction:      "outbound",
					Status:         outMsg.Status,
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
					DedupHash:      outMsg.DedupHash,
				}
				if err2 := s.db.Create(retryMsg).Error; err2 != nil {
					logger.Ctx(ctx).Warn().Err(err).Str("module", "bridge").Str("channel", string(channel)).Msg("failed to persist bridge outbound reply to message_hub")
				} else {
					persisted = retryMsg
				}
			}
			if s.inboxConvRepo != nil && persisted.ID != 0 {

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

			enqueueAbnormal := ""
			enqueueStatus := tracing.StatusOk
			if persisted.Status == "failed" {
				enqueueStatus = tracing.StatusAbnormal
				enqueueAbnormal = "目标不可达（占位账号/未知账号），标记 failed 而非 pending，避免污染下行出库队列"
			}

			ctx = tracing.WithCarrier(ctx, &tracing.Carrier{
				TraceID:        persisted.TraceID,
				ConversationID: persisted.ConversationID,
				AccountID:      persisted.AccountID,
				Channel:        persisted.Platform,
			})

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

		sent = true
	default:

		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Str("account_id", accountID).Msg("unsupported outbound channel, skipped")
	}

	if s.ingressSvc != nil && hubMsg != nil && hubMsg.ConversationID != "" {
		s.ingressSvc.ReleaseAIProcessingFlag(ctx, hubMsg.ConversationID)

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
