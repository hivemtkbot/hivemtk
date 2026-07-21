// Package telegram 封装 Telegram Bot API 的主动发消息与被动收消息（Webhook 验签/解析）。
// 纯协议层，零外部依赖，可独立开源。业务侧在通信层调用，不入侵核心代码。
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"marketing/internal/channelbot/core"
)

const defaultAPIBase = "https://api.telegram.org"

// Client Telegram Bot API 客户端
type Client struct {
	core.BaseClient
	token   string
	apiBase string
}

// NewClient 创建 Telegram 客户端
func NewClient(token string, opts ...core.ClientOption) *Client {
	c := &Client{token: token, apiBase: defaultAPIBase}
	c.BaseClient = core.NewBaseClient(opts...)
	return c
}

func (c *Client) jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json; charset=utf-8"}
}

// SendMessage 主动发文本消息，返回 Telegram 消息 ID
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	url := fmt.Sprintf("%s/bot%s/sendMessage", c.apiBase, c.token)
	payload := map[string]any{"chat_id": chatID, "text": text}
	b, _ := json.Marshal(payload)
	respB, status, err := c.DoJSON(ctx, http.MethodPost, url, bytes.NewReader(b), c.jsonHeaders())
	if err != nil {
		return 0, fmt.Errorf("tg send: %w", err)
	}
	if status != 200 {
		return 0, fmt.Errorf("tg send status %d: %s", status, string(respB))
	}
	var r struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	_ = json.Unmarshal(respB, &r)
	return r.Result.MessageID, nil
}

// SetWebhook 注册被动收消息回调；secret 用于 X-Telegram-Bot-Api-Secret-Token 验签
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	api := fmt.Sprintf("%s/bot%s/setWebhook", c.apiBase, c.token)
	payload := map[string]any{"url": url}
	if secret != "" {
		payload["secret_token"] = secret
	}
	b, _ := json.Marshal(payload)
	respB, status, err := c.DoJSON(ctx, http.MethodPost, api, bytes.NewReader(b), c.jsonHeaders())
	if err != nil {
		return fmt.Errorf("tg setWebhook: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("tg setWebhook status %d: %s", status, string(respB))
	}
	return nil
}

// DeleteWebhook 删除 webhook
func (c *Client) DeleteWebhook(ctx context.Context) error {
	api := fmt.Sprintf("%s/bot%s/deleteWebhook", c.apiBase, c.token)
	respB, status, err := c.DoJSON(ctx, http.MethodPost, api, bytes.NewReader([]byte("{}")), c.jsonHeaders())
	if err != nil {
		return fmt.Errorf("tg deleteWebhook: %w", err)
	}
	if status != 200 {
		return fmt.Errorf("tg deleteWebhook status %d: %s", status, string(respB))
	}
	return nil
}

// VerifyWebhook 校验 X-Telegram-Bot-Api-Secret-Token（明文常量比较，非 HMAC）。
// secret 为空表示未配置，跳过验签（与项目既有行为一致）。
func VerifyWebhook(secret, headerSecret string) bool {
	if secret == "" {
		return true
	}
	return core.SecureEqual(secret, headerSecret)
}

// ---- 被动：入站 Update 解析 ----

// Update Telegram webhook 推送的 Update 结构（精简版）
type Update struct {
	UpdateID      int64            `json:"update_id"`
	Message       *TGMessage       `json:"message"`
	EditedMessage *TGMessage       `json:"edited_message"`
	ChannelPost   *TGMessage       `json:"channel_post"`
	CallbackQuery *TGCallbackQuery `json:"callback_query,omitempty"`
}

// TGCallbackQuery 回调查询
type TGCallbackQuery struct {
	ID      string             `json:"id"`
	Data    string             `json:"data"`
	From    *TGUser            `json:"from"`
	Message *TGCallbackMessage `json:"message,omitempty"`
}

// TGCallbackMessage 回调所属消息（精简）
type TGCallbackMessage struct {
	Chat TGChat `json:"chat"`
}

// TGMessage 消息
type TGMessage struct {
	MessageID      int64    `json:"message_id"`
	From           *TGUser  `json:"from"`
	Chat           *TGChat  `json:"chat"`
	Text           string   `json:"text"`
	Caption        string   `json:"caption"`
	Date           int64    `json:"date"`
	NewChatMembers []TGUser `json:"new_chat_members,omitempty"`
	LeftChatMember *TGUser  `json:"left_chat_member,omitempty"`
	NewChatTitle   string   `json:"new_chat_title,omitempty"`
}

// TGUser 用户
type TGUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
	IsBot     bool   `json:"is_bot"`
}

// TGChat 会话
type TGChat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	UserName string `json:"username"`
}

// ParseUpdate 解析 Telegram webhook body
func ParseUpdate(body []byte) (*Update, error) {
	var u Update
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, fmt.Errorf("tg parse update: %w", err)
	}
	return &u, nil
}

// ToInbound 归一化为 core.InboundMessage（accountID 由调用方填充）
func (u *Update) ToInbound(accountID string) *core.InboundMessage {
	msg := u.Message
	if msg == nil {
		msg = u.EditedMessage
	}
	if msg == nil {
		msg = u.ChannelPost
	}
	if msg == nil {
		return nil
	}
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}
	name := ""
	var senderID string
	if msg.From != nil {
		name = msg.From.FirstName
		if msg.From.Username != "" {
			name = msg.From.Username
		}
		senderID = strconv.FormatInt(msg.From.ID, 10)
	}
	var chatID, groupID, groupName string
	var isGroup bool
	if msg.Chat != nil {
		chatID = strconv.FormatInt(msg.Chat.ID, 10)
		groupID = chatID
		groupName = msg.Chat.Title
		isGroup = msg.Chat.Type == "group" || msg.Chat.Type == "supergroup"
	}
	return &core.InboundMessage{
		Platform:       "telegram",
		AccountID:      accountID,
		MessageID:      "tg_" + strconv.FormatInt(msg.MessageID, 10),
		ConversationID: chatID,
		SenderID:       senderID,
		SenderName:     name,
		Content:        content,
		MsgType:        "text",
		IsGroup:        isGroup,
		GroupID:        groupID,
		GroupName:      groupName,
		Timestamp:      msg.Date,
	}
}
