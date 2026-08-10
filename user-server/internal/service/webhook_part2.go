// 拆分自 webhook.go（P2-4 God 文件拆分，同包机械拆分，不改行为）。
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/channelbot/telegram"
	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
	"strconv"
	"strings"
	"time"
)

func (s *WebhookService) ParsePayload(ctx context.Context, channel WebhookChannel, body []byte) (*ParsedPayload, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	p := &ParsedPayload{Extra: raw}
	p.EventID = getString(raw, "event_id", "EventID", "msg_id", "MsgId")
	p.EventType = getString(raw, "event_type", "EventType", "event", "Event", "type", "Type", "msg_type", "MsgType")
	p.Sender = getString(raw, "from_user", "FromUserName", "sender", "sender_id", "from")
	p.Content = getString(raw, "content", "text", "Text", "Content", "message")
	p.ChatID = getString(raw, "chat_id", "ChatID", "conversation_id", "to_user", "ToUserName")
	return p, nil
}

// ToUnifiedMessage 转成统一消息
func (s *WebhookService) ToUnifiedMessage(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload) *model.UnifiedMessage {
	return &model.UnifiedMessage{
		MessageID:   s.genMessageID(ctx, channel, accountID, p),
		Platform:    model.Platform(channel),
		AccountID:   accountID,
		ChatID:      p.ChatID,
		SenderID:    p.Sender,
		Content:     p.Content,
		ContentType: model.MessageTypeText,
		RawData:     "",
		Status:      model.MessageStatusPending,
	}
}

// =================== 内部处理 ===================

func (s *WebhookService) handleJob(ctx context.Context, job *webhookJob) {
	channel := job.channel
	if channel == "" {
		channel = WebhookChannel(job.event.Platform)
	}
	payload := job.payload
	if payload == nil {
		var err error
		payload, err = s.ParsePayload(ctx, channel, job.raw)
		if err != nil {
			logger.Errorf("[Webhook] 处理失败 event=%s: %v", job.event.EventID, err)
			return
		}
	}

	// 1) 按渠道业务分发（先分发，回填 payload.Content/payload.Sender 等标准化字段，
	//    供后续统一消息入库与 AI 编排复用，避免嵌套/密文结构导致 content/sender 为空）。
	hubMsg, tgExtra, dispatchErr := s.dispatchToChannel(ctx, channel, job.account, payload, job.raw, job.header)
	if dispatchErr != nil {
		logger.Errorf("[Webhook] dispatch %s failed event=%s: %v", channel, job.event.EventID, dispatchErr)
	}
	// 已知渠道（wecom/whatsapp/telegram/feishu）在收到非消息类事件（飞书 app_ticket、
	// 企微通讯录事件、WhatsApp 状态推送、TG 系统通知等）时会显式返回 nil hub，表示无需
	// 入库统一消息或触发 AI。自定义/其他渠道（default）返回 nil 仅表示「仅入统一消息」，
	// 仍须入库，故此处只对已知渠道做跳过处理，避免产生空内容垃圾统一消息。
	if hubMsg == nil && dispatchErr == nil {
		known := channel == ChannelWeCom || channel == ChannelWhatsapp ||
			channel == ChannelTelegram || channel == ChannelFeishu
		if known {
			logger.Infof("[Webhook] skip non-message event channel=%s event=%s", channel, job.event.EventID)
			s.markProcessed(ctx, job.event)
			return
		}
	}

	// 2) 基础入库（统一消息）：依赖上一步回填的 payload.Content / payload.Sender
	um := s.ToUnifiedMessage(ctx, channel, job.account, payload)
	if um.AccountID == "" {
		um.AccountID = job.event.EventID
	}
	if err := s.dispatchToUnified(ctx, um); err != nil {
		s.retryWithBackoff(ctx, job, payload, err)
		return
	}

	// 3) 触发 智能体（如已注入 + 启用了 智能体开关）
	// TG 群聊的触达策略（避免无脑刷屏）：
	//   · 私聊 / 非 TG：AI 直接回复；
	//   · TG 群聊「@机器人」：响应模式，直接回复（需求：群里 @bot 时才回复）；
	//   · TG 群聊「发现新晋商机线索」：主动触达（需求：群聊发现符合线索可以机器人发消息），
	//     并受冷却限制（每个发言者限频），避免同一客户反复触达；
	//   · 入群欢迎仍由 dispatchTelegram 内的 triggerTelegramJoinSales 单独处理，此处跳过。
	triggerAI := hubMsg != nil && s.shouldTriggerAI(ctx, channel, job.account)
	if triggerAI {
		if channel != ChannelTelegram || !hubMsg.IsGroup {
			s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
		} else {
			mentioned := tgExtra != nil && tgExtra.Mentioned
			newOpp := tgExtra != nil && tgExtra.NewOpportunity
			switch {
			case mentioned:
				// 响应：群内 @机器人 才回复
				s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
			case newOpp && s.tgLeadOutreachAllowed(ctx, job.account, payload.ChatID, payload.Sender):
				// 主动触达：发现新晋商机线索时，机器人主动在群里发消息（带冷却）
				s.triggerSalesEngine(ctx, channel, job.account, payload, hubMsg)
			}
		}
	}

	s.markProcessed(ctx, job.event)
}

// dispatchToChannel 按渠道路由业务逻辑
func (s *WebhookService) dispatchToChannel(ctx context.Context, channel WebhookChannel, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, *tgDispatchExtra, error) {
	switch channel {
	case ChannelWeCom:
		hub, err := s.dispatchWeCom(ctx, accountID, p, raw, headers)
		return hub, nil, err
	case ChannelWhatsapp:
		hub, err := s.dispatchWhatsApp(ctx, accountID, p, raw)
		return hub, nil, err
	case ChannelTelegram:
		return s.dispatchTelegram(ctx, accountID, p, raw)
	case ChannelFeishu:
		hub, err := s.dispatchFeishu(ctx, accountID, p, raw)
		return hub, nil, err
	default:
		// 自定义渠道仅入统一消息即可
		return nil, nil, nil
	}
}

// dispatchWeCom 企业微信业务分发
func (s *WebhookService) dispatchWeCom(ctx context.Context, accountID string, p *ParsedPayload, raw []byte, headers map[string]string) (*model.MessageHub, error) {
	if s.integration == nil {
		return nil, nil
	}
	// 解析企微解密后的明文
	plain := s.parseWeComPlain(ctx, raw)
	if plain == nil {
		// 已是明文结构
		plain = p.Extra
	}
	if plain == nil {
		return nil, fmt.Errorf("wecom plain nil")
	}
	// 解析字段
	fromUser := getString(plain, "FromUserName", "from")
	fromName := getString(plain, "FromUserName")
	msgType := strings.ToLower(getString(plain, "MsgType", "msg_type"))
	content := getString(plain, "Content", "content", "Text", "text")
	if content == "" {
		// 不同类型消息内容字段不同
		switch msgType {
		case "image":
			content = "[图片]"
		case "voice":
			content = "[语音]"
		case "video":
			content = "[视频]"
		case "file":
			content = "[文件]"
		case "location":
			content = "[位置]"
		case "link":
			content = getString(plain, "Title", "title") + " " + getString(plain, "Url", "url")
		default:
			content = getString(plain, "Content", "content", "Text", "text")
		}
	}
	mediaID := getString(plain, "MediaId", "media_id")
	chatID := getString(plain, "ChatId", "chat_id")
	chatType := getString(plain, "ChatType", "chat_type") // single/group
	event := getString(plain, "Event", "event")
	msgID := getString(plain, "MsgId", "msg_id")

	// 兼容事件型消息（如进入应用、菜单点击等）
	if msgType == "event" {
		logger.Infof("[Webhook] wecom event=%s from=%s", event, fromUser)
		return nil, nil
	}

	// accountID 转 uint
	var accID uint64
	if v, err := strconv.ParseUint(accountID, 10, 64); err == nil && v > 0 {
		accID = v
	} else {
		// 兜底：取第一个启用的企微账号
		acc, gerr := s.wecomRepo.GetByMerchant(ctx)
		if gerr != nil || len(acc) == 0 {
			return nil, fmt.Errorf("invalid account_id")
		}
		accID = uint64(acc[0].ID)
	}

	hubMsg, _, err := s.integration.ReceiveCallback(ctx, &ReceiveCallbackRequest{
		AccountID: uint(accID),
		FromUser:  fromUser,
		FromName:  fromName,
		MsgType:   msgType,
		Content:   content,
		MsgID:     msgID,
		MediaID:   mediaID,
		ChatID:    chatID,
		ChatType:  chatType,
	})
	// 回填标准化字段，供下游 AI 编排（triggerSalesEngine）与出站（sendOutbound）复用：
	// ParsePayload 无法解析企微嵌套/密文结构，否则 content/sender 为空导致 AI 拿到空输入、出站目标为空。
	if err == nil {
		p.Content = content
		p.Sender = fromUser
		p.ChatID = chatID
	}
	return hubMsg, err
}

// parseWeComPlain 如果 body 包含 encrypt 字段，尝试解密（需要从 wecomRepo 拉 EncodingAESKey）
func (s *WebhookService) parseWeComPlain(ctx context.Context, raw []byte) map[string]any {
	var p map[string]any
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	enc, _ := p["encrypt"].(string)
	if enc == "" {
		return p
	}
	// 需要获取 aesKey
	if s.wecomRepo == nil {
		return nil
	}
	// 这里使用 accountID 解析，由调用方注入；走 dispatchWeCom 的 raw
	// 注：因为 dispatchWeCom 中是按 accountID 路由，所以这里再次从 wecomRepo 拉
	accs, err := s.wecomRepo.GetByMerchant(ctx)
	if err != nil || len(accs) == 0 {
		return nil
	}
	var out map[string]any
	for _, a := range accs {
		if a.EncodingAESKey == "" {
			continue
		}
		plain, err := DecryptWeComMessage(a.EncodingAESKey, enc)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(plain, &out); err != nil {
			continue
		}
		return out
	}
	return nil
}

// dispatchWhatsApp 解析 WhatsApp Cloud API 推送并入消息中台
// payload 形如：
//
//	{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[...]}]}]}]}
func (s *WebhookService) dispatchWhatsApp(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)
	// 解析 WA webhook（被动入站解析委托独立包 channelbot/whatsapp）
	waPayload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return nil, fmt.Errorf("whatsapp parse: %w", err)
	}
	// 提取第一条有效消息
	for _, e := range waPayload.Entry {
		for _, c := range e.Changes {
			if len(c.Value.Messages) == 0 {
				continue
			}
			msg := c.Value.Messages[0]
			var content string
			msgType := msg.Type
			switch msg.Type {
			case "text":
				content = msg.Text.Body
			case "image":
				content = "[图片]"
			case "audio":
				content = "[语音]"
			case "video":
				content = "[视频]"
			case "document":
				content = "[文件]"
			default:
				content = "[" + msg.Type + "]"
			}
			// 查找联系人姓名
			name := msg.From
			for _, ct := range c.Value.Contacts {
				if ct.WAID == msg.From {
					name = ct.Profile.Name
					break
				}
			}
			// 写入消息中台
			hub := &model.MessageHub{
				Platform:       "whatsapp",
				AccountID:      accountID,
				MsgID:          msg.ID,
				Direction:      "inbound",
				SenderID:       msg.From,
				ConversationID: msg.From,
				MsgType:        msgType,
				Content:        content,
				SentAt:         time.Now(),
			}
			// 入站消息统一经消息中台处理（标准化 + 人工锁 + AI 串行锁 + 落库 + 触发 AgentRuntime）。
			// hub 仅作为收件箱会话 upsert 与上层 AI 触发的字段载体，不再单独 messageHubRepo.Create。
			if err := waPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
				return nil, err
			}
			// 写收件箱会话
			s.upsertInboxFromHub(ctx, hub, name)
			// 回填标准化字段，供下游 AI 编排与出站复用：
			// ParsePayload 无法解析 WhatsApp 嵌套结构，否则 content/sender 为空导致 AI 空输入、出站目标（手机号）为空。
			p.Content = content
			p.Sender = msg.From
			p.ChatID = msg.From
			return hub, nil
		}
	}
	return nil, nil
}

func (s *WebhookService) dispatchTelegram(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, *tgDispatchExtra, error) {
	if s.db == nil {
		return nil, nil, nil
	}
	s.ensureReposFromDB(ctx)
	// 解析 TG 消息（被动入站解析委托独立包 channelbot/telegram，支持消息/编辑/回调/入群事件）
	tgPayload, err := telegram.ParseUpdate(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("telegram parse: %w", err)
	}
	// 从 Telegram 消息中提取文本内容，设置到 ParsedPayload.Content
	// 这样 triggerSmartOrchestrator 可以正确获取消息内容
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
	// 群内「@机器人 才回复」所需的 @username（注册 webhook 时经 getMe 自动回填；为空则降级为仅回复被@机器人消息）
	botUsername := s.getTelegramBotUsername(ctx, accountID)

	// ====================================================================
	// TG 入群事件：自动触发 智能体流程
	// ====================================================================
	// 当 new_chat_members 不为空时，为每个新成员构造一条入群事件消息，
	// 写入消息中台 + 触发 智能体流程（销售引擎主动发起欢迎+销售话术）。
	// 设计原则：入群即销售触点，不依赖用户主动 /start 或发消息。
	// ====================================================================
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
				// tgPayload.Message.NewChatMembers 元素类型是匿名 struct，需先转换为 telegram.TGUser
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
			// 构造入群事件消息（写入消息中台，便于审计/复盘）
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

			// 触发 智能体流程：入群即销售起点
			// SalesRequest.UserMessage 用自然语言描述入群事件，便于 LLM 理解上下文
			triggerMsg := fmt.Sprintf("新用户 %s (@%s) 刚加入群组「%s」。请以销售助手身份主动发起欢迎+销售开场白，引导用户了解我们的产品。",
				newMember.FirstName, newMember.Username, groupLabel)
			s.triggerTelegramJoinSales(ctx, accountID, chatIDStr, senderIDStr, triggerMsg)
			return hub, nil, nil
		}
	}

	// 退群事件：仅记录到消息中台，不触发 AI
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
		// 如果消息只有 new_chat_title 等系统通知（无 text、无 new_chat_members），跳过
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
	// 入站消息统一经消息中台处理（标准化 + 人工锁 + AI 串行锁 + 落库 + 触发 AgentRuntime）。
	// TG 渠道特有逻辑（群入退群事件/线索挖掘/@提及判定）保留在 dispatch 内作为中台调用前后的预处理；
	// hub 仅作为收件箱会话 upsert 与上层 AI 触发的字段载体，不再单独 messageHubRepo.Create。
	if err := tgPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
		return nil, nil, err
	}
	s.upsertInboxFromHub(ctx, hub, picked.fromName)

	// 销售线索/商机 自动挖掘：群发言与私聊中「真人」的发言都是潜在客户（发言即线索）。
	// 静默写入线索库（去重 + 意向分增量更新），与 AI 自动回复解耦、不向会话发任何消息，绝不刷屏；
	// 排除机器人自身回复(fromIsBot)与系统事件(fromID==0)；best-effort，不影响入站主链路。
	// 返回 newOpportunity：本次是否让发言者「新晋为商机」，用于群内「发现线索主动触达」。
	newOpportunity := false
	if picked.fromID != 0 && !picked.fromIsBot {
		groupTitle := ""
		if tgPayload.Message != nil && tgPayload.Message.Chat != nil {
			groupTitle = tgPayload.Message.Chat.Title
		}
		newOpportunity = s.mineTelegramGroupLead(context.Background(), hub, chatIDStr, groupTitle, senderIDStr, picked.username, picked.fromName, picked.text)
	}

	// 群内「@机器人 才回复」的提及判定：文本含 @username，或本条是「回复了某条机器人消息」
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
		// 后端异常时放行（可用性优先，仅损失防刷屏），不阻断正常触达
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
	// 入群场景的人设：销售助手主动开场
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
	// 出站：发送到 TG 群组
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
