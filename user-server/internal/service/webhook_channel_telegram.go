package service

import (
	"context"

	"fmt"

	"strconv"

	"strings"

	"time"

	"hivemtk-user/internal/channelbot/telegram"

	"hivemtk-user/internal/model"

	"hivemtk-user/internal/pkg/utils/logger"

	"hivemtk-user/internal/cache"
)

type tgDispatchExtra struct {
	Mentioned      bool
	NewOpportunity bool
}

const (
	ChannelDouyin WebhookChannel = "douyin"

	ChannelKuaishou WebhookChannel = "kuaishou"

	ChannelXiaohongshu WebhookChannel = "xiaohongshu"

	ChannelXianyu WebhookChannel = "xianyu"

	ChannelTiktok WebhookChannel = "tiktok"

	ChannelWechat WebhookChannel = "wechat"

	ChannelWeCom WebhookChannel = "wecom"

	// ChannelDingTalk 钉钉企业内部应用机器人（2026-08-25 补齐出站分支：
	// 原先 AI 回复落入 sendOutbound default 被静默丢弃）
	ChannelDingTalk WebhookChannel = "dingtalk"

	ChannelWhatsapp WebhookChannel = "whatsapp"

	ChannelTelegram WebhookChannel = "telegram"

	ChannelFeishu WebhookChannel = "feishu"

	ChannelCustom WebhookChannel = "custom"
)

func (s *WebhookService) dispatchTelegram(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, *tgDispatchExtra, error) {
	if s.db == nil {
		return nil, nil, nil
	}
	s.ensureReposFromDB(ctx)

	tgPayload, err := telegram.ParseUpdate(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram parse: %w", err)
	}

	if tgPayload.Message != nil {
		text := tgPayload.Message.Text
		if text == "" {
			text = tgPayload.Message.Caption
		}
		if text != "" {
			p.Content = text
		}
		if tgPayload.Message.From != nil {
			p.Sender = strconv.FormatInt(tgPayload.Message.From.ID, 10)
		}
		if tgPayload.Message.Chat != nil {
			p.ChatID = strconv.FormatInt(tgPayload.Message.Chat.ID, 10)
		}
	}

	botUsername := s.getTelegramBotUsername(ctx, accountID)

	if tgPayload.Message != nil && len(tgPayload.Message.NewChatMembers) > 0 && tgPayload.Message.Chat != nil {
		chatID := tgPayload.Message.Chat.ID
		chatType := tgPayload.Message.Chat.Type
		chatTitle := tgPayload.Message.Chat.Title
		if chatType == "" {
			chatType = "group"
		}
		chatIDStr := fmt.Sprintf("%d", chatID)
		isGroup := chatType == "group" || chatType == "supergroup"

		// 仅取第一个非 bot 新成员（TG 群通常一次入群一人；批量入群时取首位，其余会被 dedup）
		var newMember *telegram.TGUser
		for i := range tgPayload.Message.NewChatMembers {
			if !tgPayload.Message.NewChatMembers[i].IsBot {

				member := tgPayload.Message.NewChatMembers[i]
				newMember = &telegram.TGUser{
					ID:        member.ID,
					FirstName: member.FirstName,
					Username:  member.Username,
					IsBot:     member.IsBot,
				}
				break
			}
		}
		if newMember != nil {
			senderIDStr := fmt.Sprintf("%d", newMember.ID)
			fromName := newMember.FirstName
			if newMember.Username != "" {
				fromName = newMember.Username
			}

			groupLabel := chatTitle
			if groupLabel == "" {
				groupLabel = chatIDStr
			}
			eventContent := fmt.Sprintf("[入群事件] 用户 %s (@%s) 加入群组 %s", newMember.FirstName, newMember.Username, groupLabel)
			hub := &model.MessageHub{
				Platform:       "telegram",
				AccountID:      accountID,
				MsgID:          fmt.Sprintf("tg_join_%d_%d", chatID, newMember.ID),
				Direction:      "inbound",
				SenderID:       senderIDStr,
				ConversationID: chatIDStr,
				MsgType:        "event",
				Content:        eventContent,
				SentAt:         time.Now(),
				IsGroup:        isGroup,
				GroupID:        chatIDStr,
			}
			if err := s.messageHubRepo.Create(ctx, hub); err != nil {
				if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
					return nil, nil, err
				}
			}
			s.upsertInboxFromHub(ctx, hub, fromName)

			triggerMsg := fmt.Sprintf("新用户 %s (@%s) 刚加入群组「%s」。请以销售助手身份主动发起欢迎+销售开场白，引导用户了解我们的产品。",
				newMember.FirstName, newMember.Username, groupLabel)
			s.triggerTelegramJoinSales(ctx, accountID, chatIDStr, senderIDStr, triggerMsg)
			return hub, nil, nil
		}
	}

	if tgPayload.Message != nil && tgPayload.Message.LeftChatMember != nil && tgPayload.Message.Chat != nil {
		chatID := tgPayload.Message.Chat.ID
		chatIDStr := fmt.Sprintf("%d", chatID)
		left := tgPayload.Message.LeftChatMember
		senderIDStr := fmt.Sprintf("%d", left.ID)
		fromName := left.FirstName
		if left.Username != "" {
			fromName = left.Username
		}
		eventContent := fmt.Sprintf("[退群事件] 用户 %s (@%s) 离开群组", left.FirstName, left.Username)
		hub := &model.MessageHub{
			Platform:       "telegram",
			AccountID:      accountID,
			MsgID:          fmt.Sprintf("tg_left_%d_%d", chatID, left.ID),
			Direction:      "inbound",
			SenderID:       senderIDStr,
			ConversationID: chatIDStr,
			MsgType:        "event",
			Content:        eventContent,
			SentAt:         time.Now(),
			IsGroup:        true,
			GroupID:        chatIDStr,
		}
		if err := s.messageHubRepo.Create(ctx, hub); err != nil {
			if !strings.Contains(err.Error(), "UNIQUE") && !strings.Contains(err.Error(), "duplicate") {
				return nil, nil, err
			}
		}
		s.upsertInboxFromHub(ctx, hub, fromName)
		return hub, nil, nil
	}

	// 提取消息
	type tgMsg struct {
		msgID     int64
		chatID    int64
		chatType  string
		fromID    int64
		fromName  string
		username  string
		fromIsBot bool
		text      string
	}
	var picked *tgMsg
	if tgPayload.Message != nil && tgPayload.Message.From != nil && tgPayload.Message.Chat != nil {

		if tgPayload.Message.Text == "" && len(tgPayload.Message.NewChatMembers) == 0 && tgPayload.Message.LeftChatMember == nil && tgPayload.Message.NewChatTitle != "" {
			return nil, nil, nil
		}
		tm := &tgMsg{
			msgID:     tgPayload.Message.MessageID,
			chatID:    tgPayload.Message.Chat.ID,
			chatType:  tgPayload.Message.Chat.Type,
			fromID:    tgPayload.Message.From.ID,
			fromName:  tgPayload.Message.From.FirstName,
			username:  tgPayload.Message.From.Username,
			fromIsBot: tgPayload.Message.From.IsBot,
			text:      tgPayload.Message.Text,
		}
		if tm.chatType == "" {
			tm.chatType = "private"
		}
		picked = tm
	} else if tgPayload.EditedMessage != nil && tgPayload.EditedMessage.Chat != nil {
		tm := &tgMsg{
			msgID:    tgPayload.EditedMessage.MessageID,
			chatID:   tgPayload.EditedMessage.Chat.ID,
			chatType: tgPayload.EditedMessage.Chat.Type,
			fromID: func() int64 {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.ID
				}
				return 0
			}(),
			fromName: func() string {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.FirstName
				}
				return ""
			}(),
			username: func() string {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.Username
				}
				return ""
			}(),
			fromIsBot: func() bool {
				if tgPayload.EditedMessage.From != nil {
					return tgPayload.EditedMessage.From.IsBot
				}
				return false
			}(),
			text: tgPayload.EditedMessage.Text,
		}
		if tm.chatType == "" {
			tm.chatType = "private"
		}
		picked = tm
	} else if tgPayload.CallbackQuery != nil && tgPayload.CallbackQuery.From != nil {
		chatID := int64(0)
		chatType := "private"
		if tgPayload.CallbackQuery.Message != nil {
			chatID = tgPayload.CallbackQuery.Message.Chat.ID
			chatType = tgPayload.CallbackQuery.Message.Chat.Type
		}
		picked = &tgMsg{
			msgID:     0,
			chatID:    chatID,
			chatType:  chatType,
			fromID:    tgPayload.CallbackQuery.From.ID,
			fromName:  tgPayload.CallbackQuery.From.FirstName,
			fromIsBot: tgPayload.CallbackQuery.From.IsBot,
			text:      "/callback " + tgPayload.CallbackQuery.Data,
		}
	}
	if picked == nil {
		return nil, nil, nil
	}
	chatIDStr := fmt.Sprintf("%d", picked.chatID)
	senderIDStr := fmt.Sprintf("%d", picked.fromID)
	hub := &model.MessageHub{
		Platform:       "telegram",
		AccountID:      accountID,
		MsgID:          fmt.Sprintf("tg_%d", picked.msgID),
		Direction:      "inbound",
		SenderID:       senderIDStr,
		ConversationID: chatIDStr,
		MsgType:        "text",
		Content:        picked.text,
		SentAt:         time.Now(),
		IsGroup:        picked.chatType == "group" || picked.chatType == "supergroup",
		GroupID:        chatIDStr,
	}
	if hub.Content == "" {
		hub.Content = "[" + picked.chatType + "]"
	}

	if err := tgPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
		return nil, nil, err
	}
	s.upsertInboxFromHub(ctx, hub, picked.fromName)

	newOpportunity := false
	if picked.fromID != 0 && !picked.fromIsBot {
		groupTitle := ""
		if tgPayload.Message != nil && tgPayload.Message.Chat != nil {
			groupTitle = tgPayload.Message.Chat.Title
		}
		newOpportunity = s.mineTelegramGroupLead(context.Background(), hub, accountID, chatIDStr, groupTitle, senderIDStr, picked.username, picked.fromName, picked.text)
	}

	mentioned := isTelegramBotMentioned(picked.text, botUsername)
	if !mentioned && tgPayload.Message != nil && tgPayload.Message.ReplyToMessage != nil &&
		tgPayload.Message.ReplyToMessage.From != nil && tgPayload.Message.ReplyToMessage.From.IsBot {
		mentioned = true
	}

	return hub, &tgDispatchExtra{Mentioned: mentioned, NewOpportunity: newOpportunity}, nil
}

// tgLeadOutreachCooldown 「发现线索主动触达」对同一发言者的冷却时长，避免群内刷屏。
const tgLeadOutreachCooldown = 30 * time.Minute

// getTelegramBotUsername 取账号绑定的机器人 @username（用于群内 @提及 识别）。
// 该值通常在注册 webhook 时经 getMe 自动回填；缺失则返回空（上层降级为仅回复被@机器人消息）。
func (s *WebhookService) getTelegramBotUsername(ctx context.Context, accountID string) string {
	if s.telegramRepo == nil {
		return ""
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return ""
	}
	acc, err := s.telegramRepo.GetByID(ctx, uint(accID))
	if err != nil || acc == nil {
		return ""
	}
	return strings.TrimSpace(acc.BotUsername)
}

// isTelegramBotMentioned 判断一条消息文本是否 @提及了本机器人。
// botUsername 为空（未配置/未回填）时返回 false。匹配大小写不敏感（TG 用户名统一小写）。
// 采用词边界匹配：@username 之后必须紧跟「非用户名字符」（字母/数字/下划线之外）或结尾，
// 避免把 @mybotX、@mybotfoo 误判为 @mybot。
func isTelegramBotMentioned(text, botUsername string) bool {
	uname := strings.TrimSpace(botUsername)
	if uname == "" {
		return false
	}
	needle := "@" + strings.ToLower(uname)
	lower := strings.ToLower(text)
	idx := strings.Index(lower, needle)
	if idx < 0 {
		return false
	}
	end := idx + len(needle)
	if end < len(lower) {
		next := lower[end]
		if (next >= 'a' && next <= 'z') || (next >= '0' && next <= '9') || next == '_' {
			return false
		}
	}
	return true
}

// tgLeadOutreachAllowed 判断该（账号, 群组, 发言者）是否允许本次「发现线索主动触达」。
// 业务需要：绝不能骚扰用户，多实例下若各持进程内冷却 map，可能仍被不同实例短时重复触达刷屏。
// 实现：基于全局缓存 SetNX + 冷却 TTL——首次设置返回 true(允许)，冷却窗口内已存在返回 false(拦截)，
// 超时后自动释放。REDIS_HOST 配置时为 Redis 共享后端（跨实例一致），否则为内存单例。
func (s *WebhookService) tgLeadOutreachAllowed(ctx context.Context, accountID, chatID, senderID string) bool {
	key := "mtk:tg:outreach:" + accountID + ":" + chatID + ":" + senderID
	set, err := cache.GetGlobalCache().SetNX(ctx, key, "1", tgLeadOutreachCooldown)
	if err != nil {

		return true
	}
	return set
}

// triggerTelegramJoinSales TG 入群事件触发 智能体流程
// 与 triggerSalesEngine 类似，但 UserMessage 是入群事件描述，让 LLM 主动发起销售对话
func (s *WebhookService) triggerTelegramJoinSales(ctx context.Context, accountID, chatID, senderID, triggerMsg string) {
	if s.salesEngine == nil {
		return
	}
	if !s.shouldTriggerAI(ctx, ChannelTelegram, accountID) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := &SalesRequest{
		SessionID:   "telegram:" + chatID,
		CustomerID:  senderID,
		OneID:       "telegram:" + senderID,
		UserMessage: triggerMsg,
		Platform:    "telegram",
		AutoExecute: true,
		Config:      DefaultSalesEngineConfig(),
	}

	req.Config.Persona = "你是 Telegram 群组里的销售助手。新用户加入群组时，主动发起一段简洁、亲切的欢迎+销售开场白，引导用户了解产品。回复不超过 80 字。"

	resp, err := s.salesEngine.Handle(ctx, req)
	if err != nil {
		logger.Errorf("[Webhook] TG 入群触发 智能体失败 account=%s chat=%s: %v", accountID, chatID, err)
		return
	}
	if resp == nil || resp.Reply == "" {
		return
	}
	if resp.TransferredToHuman {
		logger.Infof("[Webhook] TG 入群触发转人工: %s", resp.TransferReason)
		return
	}

	chatIDInt, _ := strconv.ParseInt(chatID, 10, 64)
	if chatIDInt == 0 {
		return
	}
	if s.tgIntegration == nil {
		s.tgIntegration = NewTelegramIntegrationService(s.db)
	}
	accID, err := strconv.ParseUint(accountID, 10, 64)
	if err != nil || accID == 0 {
		return
	}
	if err := s.tgIntegration.SendMessage(ctx, uint(accID), chatIDInt, resp.Reply); err != nil {
		logger.Errorf("[Webhook] TG 入群欢迎消息发送失败 account=%s chat=%s: %v", accountID, chatID, err)
	}
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
