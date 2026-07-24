package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/tgbot"
)

// GenTGWebhookSecret 生成 32 字节十六进制 webhook secret，用于
// Telegram 入站验签（X-Telegram-Bot-Api-Secret-Token）。
// 供 controller（register-webhook）与启动期对账（ReconcileTelegramWebhooks）复用。
func GenTGWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ReconcileTelegramWebhooks 启动期对账：为所有「已启用（Status==1）且开启 webhook（WebhookEnabled）
// 且 BotToken / WebhookURL 齐全」的 Telegram 账号重新调用 setWebhook。
//
// 必要性（解决「启动」断链）：
//   - Telegram 的 webhook 由机器人服务端持久保存，服务器重启、域名/公网 URL 变更、
//     或在 UI 新建账号后，若不重新注册，Telegram 不会向本系统推送更新，
//     导致「TG 入站 → AI销售 → 出站回复」全链路静默断裂。
//   - 该函数为 best-effort：单账号失败仅记录日志，不影响其他账号与启动流程。
//
// 调用方应以 goroutine 方式在路由注册完成后调用，避免阻塞启动（setWebhook 涉及外网请求）。
func ReconcileTelegramWebhooks(svc *TelegramService) {
	if svc == nil {
		return
	}
	accs, err := svc.ListAccounts(context.Background())
	if err != nil {
		logger.Warnf("[TG-Bootstrap] 列举 Telegram 账号失败: %v", err)
		return
	}
	for _, acc := range accs {
		enabled := acc.Status == 1 && acc.WebhookEnabled
		ready := acc.BotToken != "" && acc.WebhookURL != ""
		if enabled && ready {
			// secret 缺失时自动生成并落库，确保生产环境（GIN_MODE=release）入站验签可通过
			if acc.WebhookSecret == "" {
				acc.WebhookSecret = GenTGWebhookSecret()
				if err := svc.UpdateAccount(context.Background(), acc); err != nil {
					logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 保存自动生成 secret 失败: %v", acc.ID, acc.AccountName, err)
				}
			}
			if err := tgbot.SetWebhook(acc.BotToken, acc.WebhookURL, acc.WebhookSecret); err != nil {
				logger.Warnf("[TG-Bootstrap] 账号 %d(%s) setWebhook 失败: %v", acc.ID, acc.AccountName, err)
				continue
			}
			logger.Infof("[TG-Bootstrap] 账号 %d(%s) webhook 已重新注册: %s", acc.ID, acc.AccountName, acc.WebhookURL)
			// 注册成功即经 getMe 回填机器人 @username（供群内「@机器人 才回复」识别），best-effort 不阻断主流程
			if acc.BotUsername == "" {
				if uname, gerr := tgbot.GetBotUsername(acc.BotToken); gerr == nil && uname != "" {
					acc.BotUsername = uname
					if err := svc.UpdateAccount(context.Background(), acc); err != nil {
						logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 保存 bot_username 失败(可忽略): %v", acc.ID, acc.AccountName, err)
					}
				}
			}
			continue
		}
		// 未启用 / 配置不全：若仍有 BotToken，主动清理 Telegram 侧可能残留的陈旧 webhook，
		// 避免「曾启用后禁用 / 改坏 URL」的账号继续被投递、造成无效入站与潜在重复处理。
		if acc.BotToken != "" {
			if err := tgbot.DeleteWebhook(acc.BotToken); err != nil {
				logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 清理陈旧 webhook 失败(可忽略): %v", acc.ID, acc.AccountName, err)
			} else {
				logger.Infof("[TG-Bootstrap] 账号 %d(%s) 陈旧 webhook 已清理", acc.ID, acc.AccountName)
			}
		}
	}
}
