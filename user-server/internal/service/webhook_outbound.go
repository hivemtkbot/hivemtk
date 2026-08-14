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

			// 2026-08-15 修复（P1-4 outbound 场景元数据）：
			//   原实现：outbound.Extra 完全留空，导致：
			//     1) 桥接扩展端无法判断「这条 outbox 是回复原会话（conv）还是给某成员私信（dm_target=member）」
			//     2) 统一收件箱 UI 无法按 scenario / intent 维度聚合（客服回看时一片纯文本，没有上下文标签）
			//     3) 监控 / trace_learning 无法按 scenario 取数（自学习场景按 scenario 路由失效）
			//   修复：sendOutbound 调用点构造场景元数据。约定：
			//     - dm_target   : 必填；"conv"=回复原会话，"member"=给成员私信（lead_outreach 走 member）
			//     - scenario    : 必填；"auto_reply" / "transfer_respond" / "dm_outreach" / "follow_up"
			//     - intent      : 选填；SalesResponse.Intent.IntentType（greeting/inquiry/objection/...）
			//     - confidence  : 选填；HandleResult.Confidence（0-1）
			//     - agent_id    : 选填；多智能体路由时填充
			//     - triggered_by: 选填；"ai_dispatch" / "human_reply" / "system"
			//   私域部署原则：字段缺失必须显式填空（"unknown"），不允许 nil，
			//     否则 trace_learning / 监控聚合按 Extra 字段 group by 时 key 漂移。
			outMsg.Extra = model.JSONMap{
				"dm_target":    "conv", // 回复原会话（区别于 lead_outreach 走 member）
				"scenario":     "auto_reply",
				"triggered_by": "ai_dispatch",
			}
			// 从 ctx carrier 取 agent_id（多智能体路由时由 loadAgentForChannel 注入）
			if agentID := extractAgentIDFromCtx(ctx); agentID != "" {
				outMsg.Extra["agent_id"] = agentID
			}
			// result 由调用方（runAIGeneration）通过 ctx 注入，本处尝试取出
			if result := HandleResultFromContext(ctx); result != nil {
				if result.Confidence > 0 {
					outMsg.Extra["confidence"] = result.Confidence
				}
				if result.SalesResponse != nil && result.SalesResponse.Intent != nil && result.SalesResponse.Intent.IntentType != "" {
					outMsg.Extra["intent"] = string(result.SalesResponse.Intent.IntentType)
				}
				if result.HandlerType != "" {
					outMsg.Extra["handler_type"] = string(result.HandlerType)
				}
			}
			// 占位账号/未知账号等 undeliverable 场景：保留原有 undeliverable_reason 并标 scenario
			if undeliverable, reason := bridgeOutboundUndeliverable(accountID, outMsg.ConversationID); undeliverable {
				outMsg.Status = "failed"
				if outMsg.Extra == nil {
					outMsg.Extra = model.JSONMap{}
				}
				outMsg.Extra["undeliverable_reason"] = reason
				outMsg.Extra["scenario"] = "undeliverable"
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

// ============================================================================
// 2026-08-15 P1-4 outbound 场景元数据 helper
// ----------------------------------------------------------------------------
// 目的：sendOutbound 需要从 ctx 拿到 agent_id 和 HandleResult（用于补 dm_target/scenario/
//       intent/confidence/handler_type 等字段）。把 helper 与 sendOutbound 放在同一文件，
//       避免散落到其他位置后被遗忘。
// 注入方：
//   - webhook_ai.go runAIGeneration 在 result 拿到后把 HandleResult 注入 ctx
//   - loadAgentForChannel 在解析后把 agentID 注入 ctx
// 私域部署原则：ctx 取出为空时按 "unknown" 占位，不允许 nil 字段。
// ============================================================================

type handleResultCtxKey struct{}

// HandleResultToContext 把 HandleResult 注入 ctx，供 sendOutbound 取出补字段。
func HandleResultToContext(ctx context.Context, r *HandleResult) context.Context {
	if ctx == nil || r == nil {
		return ctx
	}
	return context.WithValue(ctx, handleResultCtxKey{}, r)
}

// HandleResultFromContext 从 ctx 取出 HandleResult（注入方未设时返回 nil）。
func HandleResultFromContext(ctx context.Context) *HandleResult {
	if ctx == nil {
		return nil
	}
	if r, ok := ctx.Value(handleResultCtxKey{}).(*HandleResult); ok {
		return r
	}
	return nil
}

type agentIDCtxKey struct{}

// AgentIDToContext 把 agentID 注入 ctx（多 AI 智能体路由时填充）。
func AgentIDToContext(ctx context.Context, agentID string) context.Context {
	if ctx == nil || agentID == "" {
		return ctx
	}
	return context.WithValue(ctx, agentIDCtxKey{}, agentID)
}

// extractAgentIDFromCtx 从 ctx 取出 agentID；未注入时返回 "unknown"（不允许空串，
// 私域部署要求所有 Outbound.Extra 字段都有稳定 key，否则 trace_learning/监控聚合 group by 时 key 漂移）。
func extractAgentIDFromCtx(ctx context.Context) string {
	if ctx == nil {
		return "unknown"
	}
	if v, ok := ctx.Value(agentIDCtxKey{}).(string); ok && v != "" {
		return v
	}
	return "unknown"
}
