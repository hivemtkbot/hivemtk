// Package telegram 封装 Telegram Bot API 的主动发消息与被动收消息（Webhook 验签/解析）。
// 纯协议层，零外部依赖，可独立开源。业务侧在通信层调用，不入侵核心代码。
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"

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

// SendMessage 主动发文本消息，返回 Telegram 消息 ID。
// AI 生成的回复通常是 Markdown，直接发送会把 **粗体** 等标记当纯文本泄漏。
// 这里统一把常见 Markdown 转换为 Telegram 支持的 HTML（parse_mode=HTML），
// 并在 Telegram 仍报「无法解析实体」时退化为纯文本重发，保证消息一定可达。
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	htmlText := markdownToTelegramHTML(text)
	url := fmt.Sprintf("%s/bot%s/sendMessage", c.apiBase, c.token)
	payload := map[string]any{"chat_id": chatID, "text": htmlText, "parse_mode": "HTML"}
	b, _ := json.Marshal(payload)
	respB, status, err := c.DoJSON(ctx, http.MethodPost, url, bytes.NewReader(b), c.jsonHeaders())
	if err != nil {
		return 0, fmt.Errorf("tg send: %w", err)
	}
	if status == 200 {
		return parseSendMessageID(respB), nil
	}
	// AI 生成的 markdown 经转换后仍可能非法（如未闭合标签、嵌套错误）→ 退化为纯文本重发
	if status == 400 && strings.Contains(strings.ToLower(string(respB)), "parse entities") {
		payload2 := map[string]any{"chat_id": chatID, "text": text}
		b2, _ := json.Marshal(payload2)
		respB2, status2, err2 := c.DoJSON(ctx, http.MethodPost, url, bytes.NewReader(b2), c.jsonHeaders())
		if err2 != nil {
			return 0, fmt.Errorf("tg send fallback: %w", err2)
		}
		if status2 == 200 {
			return parseSendMessageID(respB2), nil
		}
		return 0, fmt.Errorf("tg send status %d: %s", status2, string(respB2))
	}
	return 0, fmt.Errorf("tg send status %d: %s", status, string(respB))
}

// tgMdXxx 把 LLM 常见 Markdown 片段转换为 Telegram HTML 标签。
var (
	tgMdInlineCode = regexp.MustCompile("`([^`]+)`")
	tgMdBold       = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	tgMdItalic     = regexp.MustCompile(`\*([^*\n]+?)\*`)
	tgMdLink       = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
)

// markdownToTelegramHTML 将常见 Markdown（粗体/斜体/行内代码/链接）转换为 Telegram HTML。
// 先对全文做 HTML 转义（防止 < > & 原样泄漏或破坏标签），再注入受支持的标签。
// 未匹配的 ** / * 会保留为字面量（不转换），因此总能产出合法 HTML。
func markdownToTelegramHTML(md string) string {
	s := html.EscapeString(md)
	s = tgMdInlineCode.ReplaceAllString(s, "<code>$1</code>")
	s = tgMdBold.ReplaceAllString(s, "<b>$1</b>")
	s = tgMdItalic.ReplaceAllString(s, "<i>$1</i>")
	s = tgMdLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
}

// parseSendMessageID 从 sendMessage 响应中解析 message_id（解析失败返回 0）。
func parseSendMessageID(body []byte) int64 {
	var r struct {
		OK     bool `json:"ok"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if json.Unmarshal(body, &r) == nil && r.OK {
		return r.Result.MessageID
	}
	return 0
}

// SetWebhook 注册被动收消息回调；secret 用于 X-Telegram-Bot-Api-Secret-Token 验签
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	api := fmt.Sprintf("%s/bot%s/setWebhook", c.apiBase, c.token)
	payload := map[string]any{
		"url": url,
		// 显式声明要接收的更新类型，避免遗漏 callback / 入退群等事件
		"allowed_updates": []string{"message", "edited_message", "callback_query", "chat_member", "my_chat_member"},
	}
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
	MessageID      int64       `json:"message_id"`
	From           *TGUser     `json:"from"`
	Chat           *TGChat     `json:"chat"`
	Text           string      `json:"text"`
	Caption        string      `json:"caption"`
	Date           int64       `json:"date"`
	Entities       []TGEntity  `json:"entities,omitempty"`
	ReplyToMessage *TGMessage  `json:"reply_to_message,omitempty"`
	NewChatMembers []TGUser    `json:"new_chat_members,omitempty"`
	LeftChatMember *TGUser     `json:"left_chat_member,omitempty"`
	NewChatTitle   string      `json:"new_chat_title,omitempty"`
}

// TGEntity 消息内格式化实体（mention / text_mention / bot_command 等），用于精确识别 @提及
type TGEntity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	User   *TGUser `json:"user,omitempty"`
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
		// 回调查询（按钮点击）：合成入站消息，文本为 "/callback " + data
		if u.CallbackQuery != nil && u.CallbackQuery.From != nil {
			cb := u.CallbackQuery
			chatID := ""
			chatType := "private"
			isGroup := false
			if cb.Message != nil {
				chatID = strconv.FormatInt(cb.Message.Chat.ID, 10)
				chatType = cb.Message.Chat.Type
				isGroup = chatType == "group" || chatType == "supergroup"
			}
			return &core.InboundMessage{
				Platform:       "telegram",
				AccountID:      accountID,
				MessageID:      "tg_cb_" + cb.ID,
				ConversationID: chatID,
				SenderID:       strconv.FormatInt(cb.From.ID, 10),
				SenderName:     cb.From.FirstName,
				Content:        "/callback " + cb.Data,
				MsgType:        "text",
				IsGroup:        isGroup,
				GroupID:        chatID,
			}
		}
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

// Ingress 把解析后的 TG 入站消息经消息中台统一处理（构造 MessageEvent → 调用 HandleIngressMessage）。
// 渠道特有预处理：TG 用 update_id 作为 EventID 幂等键（同一 Update 重投时 update_id 不变，
// 中台据 EventID 落库 message_hub 并依赖唯一约束去重）。
// 调用方负责在调用前完成 webhook 验签；群入退群事件等系统事件由上层 dispatch 单独处理，不走本方法。
func (u *Update) Ingress(ctx context.Context, h core.IngressHandler, accountID string) error {
	if h == nil {
		return nil
	}
	inbound := u.ToInbound(accountID)
	if inbound == nil {
		return nil
	}
	event := inbound.ToMessageEvent(accountID)
	// update_id 幂等：覆盖 EventID，使中台落库的 msg_id 与 TG 重投去重对齐
	if u.UpdateID != 0 {
		event.EventID = "tg_upd_" + strconv.FormatInt(u.UpdateID, 10)
	}
	return h.HandleIngressMessage(ctx, event)
}
