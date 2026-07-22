// Package tgbot 封装 Telegram Bot API 调用。
//
// 设计说明：
//   - 本包仅提供无状态的工具函数（setWebhook / sendMessage / 创建邀请链接等）
//   - Bot 实例和长连接由调用方按需创建，避免在工具包中持有全局状态
//   - 严格遵守五层架构：工具层不调用 Service / Repository
package tgbot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"marketing/internal/pkg/utils/httpclient"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"
)

// InitTGBot 初始化 Bot API 客户端（长连接场景使用，例如 polling）
// 已不再被 webhook 模式使用，仅保留以兼容现有调用方
func InitTGBot(botToken string, groupID int64, proxyEnabled bool, proxyProto string, proxyHost string, proxyPort int) (*tgbotapi.BotAPI, error) {
	client := httpclient.New()

	if proxyEnabled {
		tgProxyURL, err := url.Parse(fmt.Sprintf("%s://%s:%d", proxyProto, proxyHost, proxyPort))
		if err != nil {
			return nil, fmt.Errorf("Failed to parse proxy: %s", err)
		}
		tgDialer, err := proxy.FromURL(tgProxyURL, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("Failed to obtain proxy dialer: %s", err)
		}
		tgTransport := &http.Transport{
			Dial: tgDialer.Dial,
		}
		client.Transport = tgTransport
	}
	var err error
	var Bot *tgbotapi.BotAPI
	Bot, err = tgbotapi.NewBotAPIWithClient(botToken, "https://api.telegram.org/bot%s/%s", client)
	if err != nil {
		return nil, err
	}
	if groupID != 0 {
		isAdmin, err := isBotAdmin(Bot, groupID)
		if err != nil {
			return nil, err
		}
		if !isAdmin {
			return nil, errors.New("机器人不是群组管理员")
		}
	}
	return Bot, nil
}

// SendTgMsg 通过既有的 Bot 实例发送消息
func SendTgMsg(Bot *tgbotapi.BotAPI, msgText string, TgID int64) error {
	msg := tgbotapi.NewMessage(TgID, msgText)
	msg.DisableWebPagePreview = true
	msg.ParseMode = "HTML"
	_, err := Bot.Send(msg)
	return err
}

// DeleteMsg 删除消息
func DeleteMsg(Bot *tgbotapi.BotAPI, chatID int64, msgID int) error {
	deleteConfig := tgbotapi.DeleteMessageConfig{
		ChatID:    chatID,
		MessageID: msgID,
	}
	if _, err := Bot.Request(deleteConfig); err != nil {
		return err
	}
	return nil
}

// 检查机器人是否是群组的管理员
func isBotAdmin(Bot *tgbotapi.BotAPI, groupID int64) (bool, error) {
	admins, err := Bot.GetChatAdministrators(tgbotapi.ChatAdministratorsConfig{ChatConfig: tgbotapi.ChatConfig{ChatID: groupID}})
	if err != nil {
		return false, err
	}
	for _, admin := range admins {
		if admin.User.ID == Bot.Self.ID {
			if admin.CanInviteUsers {
				return true, nil
			}
			return false, errors.New("机器人没有邀请权限")
		}
	}
	return false, errors.New("机器人不是群组管理员")
}

// CreateInviteLink 通过 Bot 实例创建邀请链接
func CreateInviteLink(Bot *tgbotapi.BotAPI, inviteTgID int64, duration time.Duration) (string, error) {
	var resMap map[string]any
	inviteLinkConfig := tgbotapi.CreateChatInviteLinkConfig{
		ChatConfig:         tgbotapi.ChatConfig{ChatID: inviteTgID},
		Name:               "邀请进群",
		ExpireDate:         int(time.Now().Add(duration).Unix()),
		MemberLimit:        1,
		CreatesJoinRequest: false,
	}
	response, _ := Bot.Request(inviteLinkConfig)
	err := json.Unmarshal(response.Result, &resMap)
	if err != nil {
		return "", errors.New("创建邀请链接失败")
	}

	return resMap["invite_link"].(string), nil
}

// SendInviteJoinGroup 通过 Bot 实例发送邀请链接给用户
func SendInviteJoinGroup(Bot *tgbotapi.BotAPI, groupID int64, userTgID int64) error {
	inviteLinkDuration := time.Minute * 3
	inviteLink, err := CreateInviteLink(Bot, groupID, inviteLinkDuration)
	if err != nil {
		return err
	}
	sendText := fmt.Sprintf("<a href=\"%s\">点击进群(180秒内有效)</a>", inviteLink)
	msg := tgbotapi.NewMessage(userTgID, sendText)
	msg.DisableWebPagePreview = true
	msg.ParseMode = "HTML"
	_, err = Bot.Send(msg)

	return nil
}

// UnbanUser 通过 Bot 实例解封用户
func UnbanUser(Bot *tgbotapi.BotAPI, chatID int64, userID int64) error {
	unbanConfig := tgbotapi.UnbanChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		OnlyIfBanned: true,
	}
	_, err := Bot.Request(unbanConfig)
	return err
}

// ============================================================================
// 以下为 TG 智能体流程新增工具函数（无状态，基于 bot_token 直接调用 Bot API）
// ============================================================================

var defaultHTTPClient = &http.Client{Timeout: 15 * time.Second}

// callBotAPI 通用 Bot API 调用（无状态，基于 bot_token）
func callBotAPI(botToken, method string, params url.Values) (map[string]any, error) {
	if botToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN 未配置,无法调用 Telegram Bot API")
	}
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, method)
	resp, err := defaultHTTPClient.PostForm(apiURL, params)
	if err != nil {
		return nil, fmt.Errorf("调用 %s 失败: %w", method, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		OK          bool           `json:"ok"`
		Result      map[string]any `json:"result"`
		Description string         `json:"description"`
		ErrorCode   int            `json:"error_code"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("Telegram API 错误(%d): %s", result.ErrorCode, result.Description)
	}
	return result.Result, nil
}

// SetWebhook 注册 Telegram Webhook
// webhookURL 形如：https://your-domain/api/webhook/telegram/{account_id}
// secret 由 Telegram 在 X-Telegram-Bot-Api-Secret-Token 头中回传，用于验签
func SetWebhook(botToken, webhookURL, secret string) error {
	params := url.Values{}
	params.Set("url", webhookURL)
	params.Set("allowed_updates", `["message","edited_message","callback_query","chat_member","my_chat_member"]`)
	if secret != "" {
		params.Set("secret_token", secret)
	}
	_, err := callBotAPI(botToken, "setWebhook", params)
	return err
}

// DeleteWebhook 删除 Telegram Webhook（切回 polling 模式时使用）
func DeleteWebhook(botToken string) error {
	_, err := callBotAPI(botToken, "deleteWebhook", url.Values{})
	return err
}

// SendMessage 直接通过 bot_token 发送文本消息（无状态）
// 用于 智能体流程出站回复
func SendMessage(botToken string, chatID int64, text string) error {
	params := url.Values{}
	params.Set("chat_id", strconv.FormatInt(chatID, 10))
	params.Set("text", text)
	params.Set("parse_mode", "HTML")
	params.Set("disable_web_page_preview", "true")
	_, err := callBotAPI(botToken, "sendMessage", params)
	return err
}

// GetMe 获取 Bot 自身信息（用于校验 Bot Token 可用性）
func GetMe(botToken string) (map[string]any, error) {
	return callBotAPI(botToken, "getMe", url.Values{})
}

// GetWebhookInfo 获取当前 webhook 配置（url、pending_update_count、last_error 等），
// 用于校验 webhook 是否已正确注册。
func GetWebhookInfo(botToken string) (map[string]any, error) {
	return callBotAPI(botToken, "getWebhookInfo", url.Values{})
}
