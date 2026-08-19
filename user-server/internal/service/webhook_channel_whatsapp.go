package service

import (
	"context"

	"fmt"

	"time"

	"hivemtk-user/internal/channelbot/whatsapp"

	"hivemtk-user/internal/model"
)

// dispatchWhatsApp 解析 WhatsApp Cloud API 推送并入消息中台
// payload 形如：
//
//	{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[...]}]}]}]}
func (s *WebhookService) dispatchWhatsApp(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)

	waPayload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return nil, fmt.Errorf("whatsapp parse: %w", err)
	}

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

			name := msg.From
			for _, ct := range c.Value.Contacts {
				if ct.WAID == msg.From {
					name = ct.Profile.Name
					break
				}
			}

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

			if err := waPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
				return nil, err
			}

			s.upsertInboxFromHub(ctx, hub, name)

			// 2026-08-19：接入通用线索发现（所有渠道复用）
			MineUnifiedLead(ctx, s, hub, WhatsAppLeadAdapter{}, accountID, "", "", msg.From, name, "", content)

			p.Content = content
			p.Sender = msg.From
			p.ChatID = msg.From
			return hub, nil
		}
	}
	return nil, nil
}

