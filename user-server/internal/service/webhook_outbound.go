package service

import (

	"context"

	"fmt"

	"strconv"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/repository"


	"gorm.io/gorm/clause"
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

			outMsg.Extra = model.JSONMap{
				"dm_target":    "conv", 
				"scenario":     "auto_reply",
				"triggered_by": "ai_dispatch",
			}
			if agentID := extractAgentIDFromCtx(ctx); agentID != "" {
				outMsg.Extra["agent_id"] = agentID
			}
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
			if undeliverable, reason := isBridgeChannelUndeliverableLocal(accountID, outMsg.ConversationID); undeliverable {
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
		if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "msg_id"}, {Name: "conversation_id"}}, DoNothing: true}).Create(outMsg).Error; err != nil {
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
			if err2 := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "msg_id"}, {Name: "conversation_id"}}, DoNothing: true}).Create(retryMsg).Error; err2 != nil {
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

			// Phase 1: SSE 事件驱动通知（异步，不阻塞主路径）
			// 注意：状态可能在创建后立即被轮询改为 inflight，故 pending/inflight 都触发
			if GlobalSSEPublisher != nil && persisted != nil && persisted.ID != 0 &&
				(persisted.Status == "pending" || persisted.Status == "inflight") {
				logger.Ctx(ctx).Debug().
					Int("hub_id", int(persisted.ID)).
					Str("channel", string(channel)).
					Str("account_id", accountID).
					Str("conv_id", persisted.ConversationID).
					Msg("[SSE] webhook_outbound calling GlobalSSEPublisher")
				go func() {
					defer func() {
						if r := recover(); r != nil {
							logger.Ctx(ctx).Error().Any("error", r).Msg("[SSE] GlobalSSEPublisher panic recovered in webhook_outbound")
						}
					}()
					GlobalSSEPublisher(
						string(channel), accountID,
						uint64(persisted.ID),
						persisted.ConversationID,
						persisted.MsgType,
						persisted.ReceiverID,
						persisted.Content,
						persisted.IsAIReply,
						persisted.SentAt,
					)
				}()
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

// isBridgeChannelUndeliverableLocal AI 回复路径占位账号拦截（与 bridge_outbound.go 共享同一规则）。
//
// 历史：service/bridge_outbound.go 统一实现 `bridgeOutboundUndeliverable(accountID, conversationID) (bool, string)`。
// 本地包装（is*Local）保持调用方语义清晰：本路径只关心"占位账号"这一类拦截，AI reply 不复用
// 通用 bridge outbound 的其他可能扩展（如未来新增的会话级过滤）。
func isBridgeChannelUndeliverableLocal(accountID, conversationID string) (bool, string) {
	return bridgeOutboundUndeliverable(accountID, conversationID)
}


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

