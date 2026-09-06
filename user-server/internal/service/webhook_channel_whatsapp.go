package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"hivemtk-user/internal/channelbot/whatsapp"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils/logger"
)

func waMessageContent(msgType string, body string) string {
	if msgType == "text" {
		return body
	}
	switch msgType {
	case "image":
		return "[图片]"
	case "audio":
		return "[语音]"
	case "video":
		return "[视频]"
	case "document":
		return "[文件]"
	default:
		return "[" + msgType + "]"
	}
}

func (s *WebhookService) dispatchWhatsApp(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)

	if handled, err := s.dispatchWhatsAppStatuses(ctx, accountID, raw); handled {
		return nil, err
	}

	var bufSessionID, bufMsgID string
	var bufTimestampMs int64
	if waPre, err := whatsapp.ParseWebhook(raw); err == nil {
		for _, ent := range waPre.Entry {
			for _, ch := range ent.Changes {
				for _, m := range ch.Value.Messages {
					tsSec, _ := strconv.ParseInt(m.Timestamp, 10, 64)
					if tsSec > 0 && bufSessionID == "" {
						bufSessionID = m.From
						bufMsgID = m.ID
						bufTimestampMs = tsSec * 1000
						break
					}
				}
				if bufSessionID != "" {
					break
				}
			}
			if bufSessionID != "" {
				break
			}
		}
	}
	if bufSessionID != "" {
		_, delayed := globalReorderBuffer.Offer(accountID, bufSessionID, bufMsgID, bufTimestampMs, raw)
		if delayed {
			logger.Infof("[Webhook] WhatsApp session=%s delayed by reorder buffer", bufSessionID)
			return nil, nil
		}
	}

	waPayload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return nil, fmt.Errorf("whatsapp parse: %w", err)
	}

	if err := waPayload.Ingress(ctx, s.ingressHandler(ctx), accountID); err != nil {
		return nil, err
	}

	var firstHub *model.MessageHub
	for _, e := range waPayload.Entry {
		for _, c := range e.Changes {
			for _, msg := range c.Value.Messages {
				content := waMessageContent(msg.Type, msg.Text.Body)

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
					MsgType:        msg.Type,
					Content:        content,
					SentAt:         time.Now(),
				}

				s.upsertInboxFromHub(ctx, hub, name)

				MineUnifiedLead(ctx, s, hub, WhatsAppLeadAdapter{}, accountID, "", "", msg.From, name, "", content)

				if firstHub == nil {
					firstHub = hub
					p.Content = content
					p.Sender = msg.From
					p.ChatID = msg.From
				}
			}
		}
	}
	return firstHub, nil
}

func (s *WebhookService) dispatchWhatsAppStatuses(ctx context.Context, accountID string, raw []byte) (bool, error) {
	s.ensureReposFromDB(ctx)
	payload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return false, nil
	}
	hasStatuses := false
	hasMessages := false
	for _, e := range payload.Entry {
		for _, c := range e.Changes {
			if len(c.Value.Messages) > 0 {
				hasMessages = true
			}
			for _, st := range c.Value.Statuses {
				hasStatuses = true
				if s.messageHubRepo == nil {
					break
				}
				reason := ""
				if len(st.Errors) > 0 {
					reason = st.Errors[0].Title + ": " + st.Errors[0].Message
				}
				if uerr := s.messageHubRepo.UpdateDeliveryStatus(ctx, "whatsapp", accountID, st.ID, st.Status, reason); uerr != nil {

					logger.Infof("[WhatsApp] status writeback miss wamid=%s status=%s: %v", st.ID, st.Status, uerr)
				}
			}
		}
	}
	if !hasStatuses {
		return false, nil
	}
	if hasMessages {

		return false, nil
	}
	logger.Infof("[Webhook] whatsapp statuses consumed account=%s", accountID)
	return true, nil
}
