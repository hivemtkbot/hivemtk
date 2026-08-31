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

// waMessageContent WhatsApp 消息内容映射：文本取正文，媒体类映射为占位符
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

// dispatchWhatsApp 解析 WhatsApp Cloud API 推送并入消息中台
// payload 形如：
//
//	{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[...]}}]}]}
//
// W-4 多消息批次：Meta 单次推送可能携带多条 messages[]，逐条入队，不再只处理第一条。
// T3（ChatbotX 模式移植）：statuses 回执（sent/delivered/read/failed，按 wamid）
// 前置分流——只更新 message_hub 出站行状态，不走入站管线、不触发 AI。
func (s *WebhookService) dispatchWhatsApp(ctx context.Context, accountID string, p *ParsedPayload, raw []byte) (*model.MessageHub, error) {
	if s.db == nil {
		return nil, nil
	}
	s.ensureReposFromDB(ctx)

	// T3: statuses 回执优先处理；存在 statuses 且无 messages 的推送直接结束
	if handled, err := s.dispatchWhatsAppStatuses(ctx, accountID, raw); handled {
		return nil, err
	}

	// P1-7: WhatsApp Cloud API 消息重排序缓冲接入
	// Meta 弱网/抖动下可能跨 webhook 乱序到达，先 Offer buffer 按 session 维度排序后再处理
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

	// Ingress 内部遍历全部 entry/changes/messages，整批只调一次避免重复入队
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

				// 2026-08-19：接入通用线索发现（所有渠道复用）
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

// dispatchWhatsAppStatuses 消费 WA statuses 回执（T3）。
// 返回 handled=true 表示本次推送只含回执（已处理或忽略），调用方直接结束。
// 二次审查 S2 修复：statuses 与 messages 混合推送时（Meta 批量场景），先消费
// 回执再返回 handled=false 让消息继续走主管线，避免整批丢消息。
// best-effort：行未命中（wamid 未回写/旧占位）静默忽略，不影响主链路。
func (s *WebhookService) dispatchWhatsAppStatuses(ctx context.Context, accountID string, raw []byte) (bool, error) {
	s.ensureReposFromDB(ctx)
	payload, err := whatsapp.ParseWebhook(raw)
	if err != nil {
		return false, nil // 不是合法 WA payload，交给主链路处理
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
					// wamid 未命中（占位 ID 未回写/旧数据）是常态，降为 info 语义
					logger.Infof("[WhatsApp] status writeback miss wamid=%s status=%s: %v", st.ID, st.Status, uerr)
				}
			}
		}
	}
	if !hasStatuses {
		return false, nil
	}
	if hasMessages {
		// 混合推送：回执已消费，消息交还主管线
		return false, nil
	}
	logger.Infof("[Webhook] whatsapp statuses consumed account=%s", accountID)
	return true, nil
}
