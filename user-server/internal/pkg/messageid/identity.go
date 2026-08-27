package messageid

import (
	"strings"

	"hivemtk-user/internal/model"
)

// NormalizeCustomerIDFromMessageHub 从 message_hub 消息中归一化客户标识。
// 取代原 unified_inbox/inbox.go 内部 inboxCustomerID 包级函数（Deprecated 内存收件箱）。
// 供 inbox_ingress / message_hub / reconciliation 等多包复用，避免各包各自实现归一规则。
func NormalizeCustomerIDFromMessageHub(msg *model.MessageHub) string {
	if msg == nil {
		return ""
	}
	if msg.IsGroup && msg.ConversationID != "" {
		return msg.ConversationID
	}
	if msg.ConversationID != "" {
		if msg.SenderID != "" && strings.HasPrefix(msg.SenderID, msg.ConversationID+" ") {
			return msg.ConversationID
		}
		if msg.ReceiverID != "" && strings.HasPrefix(msg.ReceiverID, msg.ConversationID+" ") {
			return msg.ConversationID
		}
	}
	if msg.Direction == "outbound" {
		if msg.ReceiverID != "" {
			return msg.ReceiverID
		}
		return msg.SenderID
	}
	return msg.SenderID
}

// NormalizeCustomerNameFromMessageHub 从 message_hub 消息中归一化客户名称。
func NormalizeCustomerNameFromMessageHub(msg *model.MessageHub) string {
	name := msg.SenderName
	if name == "" && NormalizeCustomerIDFromMessageHub(msg) == msg.ConversationID {
		name = CleanConversationTitle(msg.ConversationID)
	}
	return name
}

// CleanConversationTitle 去除 conv: 前缀。
func CleanConversationTitle(convID string) string {
	if convID == "" {
		return ""
	}
	if strings.HasPrefix(convID, "conv:") {
		return strings.TrimPrefix(convID, "conv:")
	}
	return convID
}
