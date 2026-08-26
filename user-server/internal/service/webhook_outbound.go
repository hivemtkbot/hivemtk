package service

import (
	"bytes"

	"context"

	"encoding/json"

	"fmt"

	"io"

	"net/http"

	"net/url"

	"strconv"

	"strings"

	"sync"

	"time"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	agent_runtime "hivemtk-user/internal/aiagent/agent/runtime"
	"hivemtk-user/internal/pkg/tracing"
	"hivemtk-user/internal/repository"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ===== H-3 AI 会话回复时段感知（23:00-7:00 CST 延迟队列）=====
//
// 决策依据 MASTER_COMPETITIVE_DECISIONS.md M5 表 H-3：
// 会话 AI 回复在 23:00-7:00 (CST) 进入延迟队列，次日窗口结束（07:00 CST）首发。
//
// 延迟方案选型：独立延迟表 reach_delayed_outbound（最小方案）。
// 不复用 sop_timers：其 execution_id/node_id 强绑定 SOP 执行语义，
// 且触发回路（outbox dispatcher/wait executor）需改动只读文件；
// 独立表 + 惰性 EnsureTable（参照 systemConfigKVRepo 先例）+ 本文件内自带
// 抢占式派发循环，改动面收敛在 webhook_outbound.go 一个文件内。

const (
	aiReplyQuietStartHour = 23
	aiReplyQuietEndHour   = 7

	delayedOutboundPollInterval = 30 * time.Second
	delayedOutboundBatchSize    = 20
)

// DelayedOutboundReply AI 回复延迟出站记录（表 reach_delayed_outbound）
type DelayedOutboundReply struct {
	ID             uint          `gorm:"primaryKey;autoIncrement" json:"id"`
	Platform       string        `gorm:"type:varchar(30);index" json:"platform"`
	AccountID      string        `gorm:"type:varchar(64)" json:"account_id"`
	ConversationID string        `gorm:"type:varchar(128);index" json:"conversation_id"`
	SenderID       string        `gorm:"type:varchar(128)" json:"sender_id"`
	Content        string        `gorm:"type:text" json:"content"`
	Cards          model.JSONMap `gorm:"type:jsonb" json:"cards"`
	SendAt         time.Time     `gorm:"index" json:"send_at"`
	Status         string        `gorm:"type:varchar(20);default:'pending';index" json:"status"`
	SentAt         *time.Time    `json:"sent_at"`
	CreatedAt      time.Time     `gorm:"autoCreateTime" json:"created_at"`
}

// TableName 指定表名
func (DelayedOutboundReply) TableName() string { return "reach_delayed_outbound" }

// isAIReplyQuietHours H-3：AI 会话回复静默窗口 23:00-7:00 (CST) 判定
func isAIReplyQuietHours(t time.Time) bool {
	return inQuietHoursWindow(t, aiReplyQuietStartHour, aiReplyQuietEndHour)
}

// aiReplyQuietHoursFn 可替换时钟判定（测试注入用），生产指向 isAIReplyQuietHours（参照 smsNightRestrictedFn 先例）
var aiReplyQuietHoursFn = isAIReplyQuietHours

// delayedReplayCtxKey 标记本次 sendOutbound 为延迟重放（跳过再次入队，防循环）
type delayedReplayCtxKey struct{}

// DelayedReplayToContext 标记 ctx 为延迟队列重放路径
func DelayedReplayToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, delayedReplayCtxKey{}, true)
}

func isDelayedReplay(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(delayedReplayCtxKey{}).(bool)
	return ok && v
}

var (
	delayedOutboundTableMu   sync.Mutex
	delayedOutboundTableDone = map[*gorm.DB]bool{}
)

func ensureDelayedOutboundTable(ctx context.Context, db *gorm.DB) {
	if db == nil {
		return
	}
	delayedOutboundTableMu.Lock()
	done := delayedOutboundTableDone[db]
	delayedOutboundTableMu.Unlock()
	if done {
		return
	}
	if err := db.AutoMigrate(&DelayedOutboundReply{}); err != nil {
		logger.Errorf("[H-3] reach_delayed_outbound 建表失败: %v", err)
		return
	}
	delayedOutboundTableMu.Lock()
	delayedOutboundTableDone[db] = true
	delayedOutboundTableMu.Unlock()
}

// enqueueDelayedOutbound 将 AI 回复写入延迟队列表，次日 07:00 CST 首发
func (s *WebhookService) enqueueDelayedOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub, cards []model.RichCard) bool {
	if s.db == nil {
		logger.Ctx(ctx).Warn().Str("channel", string(channel)).Msg("[H-3] db 未初始化，quiet hours 延迟入队失败，按原路径直接发送")
		return false
	}
	ensureDelayedOutboundTable(ctx, s.db)

	var cardsPayload model.JSONMap
	if len(cards) > 0 {
		if raw, err := json.Marshal(cards); err == nil {
			cardsPayload = model.JSONMap{"cards": json.RawMessage(raw)}
		}
	}

	rec := &DelayedOutboundReply{
		Platform:  string(channel),
		AccountID: accountID,
		SenderID:  p.Sender,
		Content:   content,
		Cards:     cardsPayload,
		SendAt:    nextQuietHoursRelease(time.Now(), aiReplyQuietEndHour),
		Status:    "pending",
	}
	if hubMsg != nil {
		rec.ConversationID = hubMsg.ConversationID
	}
	if err := s.db.WithContext(ctx).Create(rec).Error; err != nil {
		logger.Ctx(ctx).Error().Err(err).Str("channel", string(channel)).Msg("[H-3] 延迟入队失败，按原路径直接发送")
		return false
	}
	logger.Ctx(ctx).Info().
		Str("channel", string(channel)).
		Str("conv_id", rec.ConversationID).
		Time("send_at", rec.SendAt).
		Msg("[H-3] AI 回复命中 quiet hours(23:00-7:00)，进入延迟队列次日首发")
	s.startDelayedOutboundDispatch()
	return true
}

var delayedDispatchStop chan struct{}

// startDelayedOutboundDispatch 惰性启动派发循环（每服务实例一次）
func (s *WebhookService) startDelayedOutboundDispatch() {
	dispatchOnce.Do(func() {
		delayedDispatchStop = make(chan struct{})
		go func() {
			ticker := time.NewTicker(delayedOutboundPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-delayedDispatchStop:
					return
				case <-ticker.C:
					s.dispatchDueDelayedOutbound(context.Background())
				}
			}
		}()
	})
}

var dispatchOnce sync.Once

// dispatchDueDelayedOutbound 抢占式领取到期记录并重放出站
// （FOR UPDATE SKIP LOCKED，多实例并发安全，与 sop_timer 同模式）
func (s *WebhookService) dispatchDueDelayedOutbound(ctx context.Context) {
	if s.db == nil {
		return
	}
	ensureDelayedOutboundTable(ctx, s.db)
	now := time.Now()
	var picked []DelayedOutboundReply
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Raw(`SELECT * FROM reach_delayed_outbound WHERE status = ? AND send_at <= ? ORDER BY send_at ASC LIMIT ? FOR UPDATE SKIP LOCKED`,
			"pending", now, delayedOutboundBatchSize).Scan(&picked).Error; err != nil {
			return err
		}
		if len(picked) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(picked))
		for _, r := range picked {
			ids = append(ids, r.ID)
		}
		return tx.Model(&DelayedOutboundReply{}).Where("id IN ?", ids).
			Update("status", "sending").Error
	})
	if err != nil {
		// sqlite 等本地测试库不支持 SKIP LOCKED 时降级为乐观两步抢占
		picked = nil
		var ids []uint
		if err2 := s.db.WithContext(ctx).Model(&DelayedOutboundReply{}).
			Where("status = ? AND send_at <= ?", "pending", now).
			Order("send_at ASC").Limit(delayedOutboundBatchSize).
			Pluck("id", &ids).Error; err2 != nil || len(ids) == 0 {
			return
		}
		res := s.db.WithContext(ctx).Model(&DelayedOutboundReply{}).
			Where("id IN ? AND status = ?", ids, "pending").Update("status", "sending")
		if res.Error != nil || res.RowsAffected == 0 {
			return
		}
		if err2 := s.db.WithContext(ctx).Where("id IN ? AND status = ?", ids, "sending").
			Order("send_at ASC").Limit(delayedOutboundBatchSize).
			Find(&picked).Error; err2 != nil || len(picked) == 0 {
			return
		}
	}
	for i := range picked {
		s.replayDelayedOutbound(ctx, &picked[i])
	}
}

// replayDelayedOutbound 重放单条延迟 AI 回复
func (s *WebhookService) replayDelayedOutbound(ctx context.Context, rec *DelayedOutboundReply) {
	channel := WebhookChannel(rec.Platform)
	hubMsg := &model.MessageHub{
		ConversationID: rec.ConversationID,
		SenderID:       rec.SenderID,
		SentAt:         rec.CreatedAt,
	}
	p := &ParsedPayload{EventID: fmt.Sprintf("delayed-%d", rec.ID), Sender: rec.SenderID}

	var cards []model.RichCard
	if rec.Cards != nil {
		if raw, ok := rec.Cards["cards"]; ok {
			if data, err := json.Marshal(raw); err == nil {
				_ = json.Unmarshal(data, &cards)
			}
		}
	}

	s.sendOutbound(DelayedReplayToContext(ctx), channel, rec.AccountID, p, rec.Content, hubMsg, cards)

	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&DelayedOutboundReply{}).Where("id = ?", rec.ID).
		Updates(map[string]any{"status": "sent", "sent_at": &now}).Error; err != nil {
		logger.Ctx(ctx).Warn().Err(err).Uint("id", rec.ID).Msg("[H-3] 延迟回复状态回写失败")
	}
}

func (s *WebhookService) sendOutbound(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, content string, hubMsg *model.MessageHub, cards []model.RichCard) {

	// H-3：AI 会话回复时段感知——23:00-7:00 (CST) 进入延迟队列，次日 07:00 首发。
	// 延迟重放路径（isDelayedReplay）与入队失败降级路径不受影响。
	if !isDelayedReplay(ctx) && aiReplyQuietHoursFn(time.Now()) {
		if s.enqueueDelayedOutbound(ctx, channel, accountID, p, content, hubMsg, cards) {
			return
		}
	}

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
		outConv := ""
		if hubMsg != nil {
			outConv = hubMsg.ConversationID
		}
		if err := s.feishuIntegration.SendMessage(ctx, uint(accID), target, content, idType, outConv); err != nil {
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

		// 2026-08-25 修复：WhatsApp Cloud API 24 小时客服窗口预检。
		// 官方文档：仅当用户最近一次消息在 24h 内才允许发自由文本；窗口外只能发模板消息。
		// 注意：不能用 hubMsg.SentAt 作锚点——AI 路径的 hubMsg 由 TriggerInboundAI 以 time.Now()
		// 现场构造（恒为"刚刚"→ 检查成死代码）。必须回查 message_hub 该会话最后一条真实入站行。
		// （查不到/零值视为未知，放行尝试，避免误杀）
		s.ensureReposFromDB(ctx)
		if s.messageHubRepo != nil && hubMsg != nil && hubMsg.ConversationID != "" {
			if last, qerr := s.messageHubRepo.GetLastInboundByConversation(ctx, hubMsg.ConversationID); qerr == nil && last != nil && !last.SentAt.IsZero() {
				if time.Since(last.SentAt) > 24*time.Hour {
					logger.Ctx(ctx).Warn().
						Str("channel", "whatsapp").Str("account_id", accountID).
						Str("to", p.Sender).
						Time("last_inbound_at", last.SentAt).
						Msg("[WhatsApp] 超出 24h 客服窗口，AI 文本回复不可送达（需模板消息），标记失败")
					return
				}
			}
		}

		if err := s.waIntegration.SendMessage(ctx, uint(accID), p.Sender, content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "whatsapp").Str("account_id", accountID).Msg("outbound failed")
		} else {
			sent = true
		}
	case ChannelDingTalk:
		// 钉钉企业内部应用机器人回复（2026-08-25 补齐）。
		// 官方文档《机器人回复/发送消息》：回调消息体携带临时 sessionWebhook，
		// 回复即 POST {"msgtype":"text","text":{"content":"..."}}；过期后无法补发（无替代 DM 通道）。
		webhookURL := ""
		var expiredAt int64
		if hubMsg != nil && hubMsg.Extra != nil {
			if v, ok := hubMsg.Extra["session_webhook"].(string); ok {
				webhookURL = v
			}
			switch t := hubMsg.Extra["session_webhook_expired_at"].(type) {
			case int64:
				expiredAt = t
			case float64:
				expiredAt = int64(t)
			}
		}
		if webhookURL == "" {
			logger.Ctx(ctx).Error().Str("channel", "dingtalk").Str("account_id", accountID).
				Str("conv_id", convIDOrEmpty(hubMsg)).
				Msg("[DingTalk] 缺少 sessionWebhook（回调未携带或非机器人消息），AI 回复无法送达")
			return
		}
		// 过期时间归一到秒：官方 sessionWebhookExpiredTime 为毫秒时间戳；
		// >1e12 视为毫秒（防新旧协议混用），<=1e12 视为秒。
		if expiredAt > 1_000_000_000_000 {
			expiredAt /= 1000
		}
		if expiredAt > 0 && time.Now().Unix() > expiredAt {
			logger.Ctx(ctx).Warn().Str("channel", "dingtalk").Str("account_id", accountID).
				Int64("expired_at", expiredAt).
				Msg("[DingTalk] sessionWebhook 已过期，AI 回复无法送达（用户重新发言可恢复）")
			return
		}
		// 防御性校验：仅允许钉钉官方域名，防止 Extra 被非可信来源写入后变成 SSRF 出口
		u, perr := url.Parse(webhookURL)
		if perr != nil || !dingtalkWebhookHostAllowed(u) {
			logger.Ctx(ctx).Error().Str("channel", "dingtalk").Str("account_id", accountID).
				Msg("[DingTalk] sessionWebhook 域名非法（仅允许 *.dingtalk.com），拒绝发送")
			return
		}
		payload := map[string]any{
			"msgtype": "text",
			"text":    map[string]string{"content": content},
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "dingtalk").Msg("build dingtalk reply request failed")
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "dingtalk").Msg("dingtalk reply send failed")
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		var dtResult struct {
			Errcode int    `json:"errcode"`
			Errmsg  string `json:"errmsg"`
		}
		if err := json.Unmarshal(respBody, &dtResult); err != nil || dtResult.Errcode != 0 {
			logger.Ctx(ctx).Error().Int("http_status", resp.StatusCode).Int("errcode", dtResult.Errcode).
				Str("errmsg", dtResult.Errmsg).
				Msg("[DingTalk] AI 回复出站被平台拒绝")
			return
		}
		sent = true
	case ChannelWechat:
		// 微信公众号客服消息出站（2026-08-25 修复）：
		// 原先无此分支，AI 回复落入 default 被 "unsupported outbound channel" 静默丢弃。
		// accountID 解析失败时传 0，由 SendCustomMessage 自动回退第一个 active 公众号账号
		// （与 WechatService.SendCustomMessage 的 accountID=0 语义一致）。
		if s.wechatIntegration == nil {
			s.wechatIntegration = NewWechatService(s.db)
		}
		accID, err := strconv.ParseUint(accountID, 10, 64)
		if err != nil {
			accID = 0
		}
		if _, err := s.wechatIntegration.SendCustomMessage(ctx, uint(accID), p.Sender, "text", content); err != nil {
			logger.Ctx(ctx).Error().Err(err).Str("channel", "wechat").Str("account_id", accountID).
				Str("open_id", p.Sender).Msg("[Wechat] AI 回复出站失败")
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
			if err := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "platform"}, {Name: "msg_id"}, {Name: "conversation_id"}}, DoNothing: true}).Create(outMsg).Error; err != nil {
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
				if err2 := s.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "platform"}, {Name: "msg_id"}, {Name: "conversation_id"}}, DoNothing: true}).Create(retryMsg).Error; err2 != nil {
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

// convIDOrEmpty nil 安全取会话 ID（日志用）
func convIDOrEmpty(h *model.MessageHub) string {
	if h == nil {
		return ""
	}
	return h.ConversationID
}

// dingtalkWebhookHostAllowed sessionWebhook 域名允许列表（包级变量便于测试注入）。
// 生产仅放行钉钉官方域名。
var dingtalkWebhookHostAllowed = func(u *url.URL) bool {
	return u != nil && (u.Host == "oapi.dingtalk.com" || strings.HasSuffix(u.Host, ".dingtalk.com"))
}

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
