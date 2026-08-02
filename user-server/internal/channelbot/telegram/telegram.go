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
	"time"

	"marketing/internal/channelbot/core"
)

const (
	defaultAPIBase = "https://api.telegram.org"
	// TGMessageMaxLength Telegram 单条消息最大字符数（官方 API 硬限制）
	// 超过会被 TG 拒绝 400 "message is too long"。
	// 拆分时按"按行/段"边界优先，避免在单词/字符中间截断。
	TGMessageMaxLength = 4096
	// TG_INLINE_ROWS_MAX / TG_INLINE_BUTTONS_PER_ROW TG inline_keyboard 上限
	// 官方限制：最多 100 个按钮；每行最多 8 个。
	// 超出时本实现自动截断。
	TGInlineRowsMax          = 100
	TGInlineButtonsPerRowMax = 8
	// 默认出站重试参数（429/5xx）
	tgSendMaxRetries  = 3
	tgSendInitialWait = 200 * time.Millisecond
	tgSendMaxWait     = 5 * time.Second
)

// Client Telegram Bot API 客户端
type Client struct {
	core.BaseClient
	token   string
	apiBase string
}

// NewClient 创建 Telegram 客户端
func NewTelegramClient(token string, opts ...core.ClientOption) *Client {
	c := &Client{token: token, apiBase: defaultAPIBase}
	c.BaseClient = core.NewBaseClient(opts...)
	return c
}

func (c *Client) jsonHeaders() map[string]string {
	return map[string]string{"Content-Type": "application/json; charset=utf-8"}
}

// SendMessageOptions 主动发消息的可选参数（opts 可变参；零值表示不设置）
type SendMessageOptions struct {
	ReplyToMessageID          int64
	InlineKeyboard            [][]InlineButton
	DisableWebPreview         bool
	ParseMode                 string
	DisableMarkdownConversion bool
}

// InlineButton 内联按钮（URL 与 CallbackData 互斥；同时存在时优先 CallbackData）
type InlineButton struct {
	Text         string
	CallbackData string
	URL          string
}

// SendMessage 主动发文本消息，返回 Telegram 消息 ID。
//
// 健壮性（与本任务全链路审计 Top-5 对齐）：
//  1. AI 生成的回复通常是 Markdown，统一转换为 Telegram HTML（parse_mode=HTML），
//     并在解析失败时回退纯文本重发
//  2. 单条消息 > 4096 字符：按行/段优先切分，循环发送多条
//  3. 429 限流：读 parameters.retry_after 退避后重试，最多 3 次
//  4. 5xx / 网络错误：指数退避重试，最多 3 次
//  5. 支持 reply_to_message_id、inline_keyboard、disable_web_page_preview
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendMessageOptions) (int64, error) {
	opt := SendMessageOptions{}
	if len(opts) > 0 {
		opt = opts[0]
	}
	chunks := splitMessage(text, TGMessageMaxLength)
	if len(chunks) == 0 {
		return 0, fmt.Errorf("empty text")
	}
	var firstID int64
	for i, chunk := range chunks {
		perOpt := opt
		if i > 0 {
			perOpt.ReplyToMessageID = 0
			perOpt.InlineKeyboard = nil
			perOpt.DisableMarkdownConversion = true
		}
		msgID, err := c.sendSingle(ctx, chatID, chunk, perOpt)
		if err != nil {
			return firstID, fmt.Errorf("tg send chunk %d/%d failed: %w", i+1, len(chunks), err)
		}
		if i == 0 {
			firstID = msgID
		}
	}
	return firstID, nil
}

// sendSingle 实际调用 sendMessage（带重试 + Markdown→HTML）
func (c *Client) sendSingle(ctx context.Context, chatID int64, text string, opt SendMessageOptions) (int64, error) {
	body := text
	parseMode := opt.ParseMode
	if parseMode == "" {
		parseMode = "HTML"
	}
	if !opt.DisableMarkdownConversion {
		body = markdownToTelegramHTML(text)
	}
	payload := map[string]any{
		"chat_id":    chatID,
		"text":       body,
		"parse_mode": parseMode,
	}
	if opt.ReplyToMessageID > 0 {
		payload["reply_to_message_id"] = opt.ReplyToMessageID
	}
	if opt.DisableWebPreview {
		payload["disable_web_page_preview"] = true
	}
	if kb := buildInlineKeyboard(opt.InlineKeyboard); kb != nil {
		payload["reply_markup"] = kb
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("tg send marshal: %w", err)
	}
	url := fmt.Sprintf("%s/bot%s/sendMessage", c.apiBase, c.token)

	var lastErr error
	wait := tgSendInitialWait
	for attempt := 0; attempt < tgSendMaxRetries; attempt++ {
		if attempt > 0 {
			if err := sleepCtx(ctx, wait); err != nil {
				return 0, err
			}
			wait *= 2
			if wait > tgSendMaxWait {
				wait = tgSendMaxWait
			}
		}
		respB, status, err := c.DoJSON(ctx, http.MethodPost, url, bytes.NewReader(b), c.jsonHeaders())
		if err != nil {
			lastErr = fmt.Errorf("tg send: %w", err)
			continue
		}
		if status == 200 {
			return parseSendMessageID(respB), nil
		}
		if status == 400 && !opt.DisableMarkdownConversion && strings.Contains(strings.ToLower(string(respB)), "parse entities") {
			payload2 := map[string]any{"chat_id": chatID, "text": text}
			b2, err := json.Marshal(payload2)
			if err != nil {
				return 0, fmt.Errorf("tg send fallback marshal: %w", err)
			}
			respB2, status2, err2 := c.DoJSON(ctx, http.MethodPost, url, bytes.NewReader(b2), c.jsonHeaders())
			if err2 != nil {
				lastErr = fmt.Errorf("tg send fallback: %w", err2)
				continue
			}
			if status2 == 200 {
				return parseSendMessageID(respB2), nil
			}
			lastErr = fmt.Errorf("tg send fallback status %d: %s", status2, string(respB2))
			continue
		}
		if status == 429 {
			ra := parseRetryAfter(respB)
			if ra > 0 {
				wait = time.Duration(ra)*time.Second + 200*time.Millisecond
			}
			lastErr = fmt.Errorf("tg send 429 (rate limited, retry_after=%ds): %s", ra, string(respB))
			continue
		}
		if status >= 500 && status < 600 {
			lastErr = fmt.Errorf("tg send status %d: %s", status, string(respB))
			continue
		}
		return 0, fmt.Errorf("tg send status %d: %s", status, string(respB))
	}
	return 0, fmt.Errorf("tg send exhausted %d retries: %w", tgSendMaxRetries, lastErr)
}

// splitMessage 按字符上限切分文本，优先在"段落 / 行 / 句子 / 单词"边界切
//
// 全部基于 rune 计算（CJK 一个字符多字节，不能用 byte 偏移）；
// 硬切分兜底只在 limit 个 rune 范围内找最近边界，避免越界。
func splitMessage(text string, limit int) []string {
	if limit <= 0 {
		limit = TGMessageMaxLength
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return []string{}
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}
	var out []string
	for len(runes) > limit {
		split := -1
		// 1) 段落边界 "\n\n"（在前 limit 个 rune 范围内找最近）
		for i := limit; i > 0; i-- {
			if i+1 < len(runes) && runes[i] == '\n' && runes[i+1] == '\n' {
				split = i + 2
				break
			}
		}
		// 2) 行边界 "\n"
		if split == -1 {
			for i := limit; i > 0; i-- {
				if runes[i] == '\n' {
					split = i + 1
					break
				}
			}
		}
		// 3) 句子边界
		if split == -1 {
			for i := limit; i > 0; i-- {
				if isSentenceSep(runes[i]) {
					split = i + 1
					break
				}
			}
		}
		// 4) 空格边界
		if split == -1 {
			for i := limit; i > 0; i-- {
				if runes[i] == ' ' {
					split = i + 1
					break
				}
			}
		}
		// 5) 硬切兜底
		if split <= 0 {
			split = limit
		}
		head := strings.TrimRight(string(runes[:split]), " \n")
		if head == "" {
			head = string(runes[:split])
		}
		out = append(out, head)
		runes = runes[split:]
	}
	if len(runes) > 0 {
		out = append(out, strings.TrimRight(string(runes), " \n"))
	}
	return out
}

// isSentenceSep 中英文常见句子分隔符
func isSentenceSep(r rune) bool {
	switch r {
	case '。', '！', '？', '\n', '.', '!', '?':
		return true
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// buildInlineKeyboard 把业务侧按钮序列化为 Telegram reply_markup.inline_keyboard
func buildInlineKeyboard(rows [][]InlineButton) map[string]any {
	if len(rows) == 0 {
		return nil
	}
	out := make([][]map[string]string, 0, len(rows))
	for i, row := range rows {
		if i >= TGInlineRowsMax {
			break
		}
		btnRow := make([]map[string]string, 0, len(row))
		for j, btn := range row {
			if j >= TGInlineButtonsPerRowMax {
				break
			}
			if btn.Text == "" {
				continue
			}
			entry := map[string]string{"text": btn.Text}
			if btn.CallbackData != "" {
				entry["callback_data"] = btn.CallbackData
			} else if btn.URL != "" {
				entry["url"] = btn.URL
			} else {
				continue
			}
			btnRow = append(btnRow, entry)
		}
		if len(btnRow) > 0 {
			out = append(out, btnRow)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return map[string]any{"inline_keyboard": out}
}

// parseRetryAfter 从 429 响应体解析 retry_after（秒）
func parseRetryAfter(body []byte) int {
	var r struct {
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Parameters.RetryAfter
}

// sleepCtx 带 ctx 取消的 sleep
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
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
//
// allowed_updates 与 GetUpdates 保持一致：覆盖全部 Update 类型（含 channel_post /
// edited_channel_post / inline_query），避免被用作战道管理员或 inline 模式时静默丢消息。
func (c *Client) SetWebhook(ctx context.Context, url, secret string) error {
	api := fmt.Sprintf("%s/bot%s/setWebhook", c.apiBase, c.token)
	payload := map[string]any{
		"url": url,
		"allowed_updates": []string{
			"message",
			"edited_message",
			"channel_post",
			"edited_channel_post",
			"callback_query",
			"inline_query",
			"chosen_inline_result",
			"chat_member",
			"my_chat_member",
			"chat_join_request",
		},
	}
	if secret != "" {
		payload["secret_token"] = secret
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("tg setWebhook marshal: %w", err)
	}
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
	MessageID      int64      `json:"message_id"`
	From           *TGUser    `json:"from"`
	Chat           *TGChat    `json:"chat"`
	Text           string     `json:"text"`
	Caption        string     `json:"caption"`
	Date           int64      `json:"date"`
	Entities       []TGEntity `json:"entities,omitempty"`
	ReplyToMessage *TGMessage `json:"reply_to_message,omitempty"`
	NewChatMembers []TGUser   `json:"new_chat_members,omitempty"`
	LeftChatMember *TGUser    `json:"left_chat_member,omitempty"`
	NewChatTitle   string     `json:"new_chat_title,omitempty"`
}

// TGEntity 消息内格式化实体（mention / text_mention / bot_command 等），用于精确识别 @提及
type TGEntity struct {
	Type   string  `json:"type"`
	Offset int     `json:"offset"`
	Length int     `json:"length"`
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
