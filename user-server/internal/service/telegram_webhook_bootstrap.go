package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/tgbot"
	"hivemtk-user/internal/pkg/utils/logger"
)

// GenTGWebhookSecret 生成 32 字节十六进制 webhook secret，用于
// Telegram 入站验签（X-Telegram-Bot-Api-Secret-Token）。
// 供 controller（register-webhook）与启动期对账（ReconcileTelegramWebhooks）复用。
func GenTGWebhookSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// TelegramWebhookURLPathPrefix 是本系统接收 Telegram 推送的标准路径前缀。
// 所有合法的 TG webhook URL 必须形如：https://host/api/webhook/telegram/{id}
// → 前缀校验可拦截误填（如填了 /webhook/tg、/tg-hook 等），避免无效 setWebhook。
const TelegramWebhookURLPathPrefix = "/api/webhook/telegram/"

// ValidateTelegramWebhookURL 校验 webhook URL 是否符合 Telegram + 本系统要求
//
// 校验规则（修复 S3-5）：
//  1. URL 必须可被 net/url 解析
//  2. scheme 必须为 https（Telegram 强制要求）
//  3. host 必须非空（含端口时端口 > 0）
//  4. path 必须以 /api/webhook/telegram/ 开头（确保 Telegram 推送能被本系统路由到对应 controller）
//
// 校验失败返回非空错误，错误信息对运维友好；返回 nil 表示通过。
//
// 注意：仅做本地静态校验，不发起网络请求。运行期仍依赖 verifyWebhookInfo 拉 getWebhookInfo 自检。
func ValidateTelegramWebhookURL(raw string) error {
	v := strings.TrimSpace(raw)
	if v == "" {
		return fmt.Errorf("webhook URL 为空")
	}
	u, err := url.Parse(v)
	if err != nil {
		return fmt.Errorf("webhook URL 解析失败: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("webhook URL scheme 必须是 https，当前=%q（Telegram 强制 https）", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("webhook URL 缺少 host（如 https://chat.example.com）")
	}
	if !strings.HasPrefix(u.Path, TelegramWebhookURLPathPrefix) {
		return fmt.Errorf("webhook URL path 必须以 %q 开头，当前=%q", TelegramWebhookURLPathPrefix, u.Path)
	}
	return nil
}

// ResolveTelegramWebhookURL 解析单个 Telegram 账号的 webhook 回调 URL
//
// 解析顺序（自高到低）：
//  1. 账号表 telegram_accounts.webhook_url（用户在 UI 显式填写的，覆盖默认）
//  2. 配置文件 external.public_base_url 或环境变量 PUBLIC_BASE_URL（公网域名/frp 暴露的域名）
//     → 私域部署基线：user-server 跑在 frp 后，请求 Host 总是 localhost:8204，
//     必须显式声明公网域名，否则 Telegram 永远无法回调本系统。
//
// 返回值：(url, hasPublicBase)
//   - url: 最终 webhook URL；账号未启用 / 未配置 时返回空串
//   - hasPublicBase: 解析过程中是否命中了 public_base_url（用于决定是否走 polling fallback）
func ResolveTelegramWebhookURL(acc *model.TelegramAccount) (string, bool) {
	if acc == nil {
		return "", false
	}
	explicit := strings.TrimSpace(acc.WebhookURL)
	if explicit != "" {
		return explicit, true
	}
	publicBase := config.GetPublicBaseURL()
	if publicBase == "" {
		return "", false
	}
	return strings.TrimRight(publicBase, "/") + fmt.Sprintf("/api/webhook/telegram/%d", acc.ID), true
}

func verifyWebhookInfo(acc *model.TelegramAccount) {
	if acc == nil || acc.BotToken == "" {
		return
	}
	info, err := tgbot.GetWebhookInfo(acc.BotToken)
	if err != nil {
		logger.Warnf("[TG-Bootstrap] 账号 %d(%s) getWebhookInfo 失败(可忽略): %v", acc.ID, acc.AccountName, err)
		return
	}
	if info == nil {
		return
	}
	if gotURL, _ := info["url"].(string); gotURL != "" && gotURL != acc.WebhookURL {
		logger.Warnf("[TG-Bootstrap] 账号 %d(%s) webhook URL 已被覆盖: 期望=%s 实际=%s (另一进程可能正在 polling/注册)", acc.ID, acc.AccountName, acc.WebhookURL, gotURL)
	}
	if pending, ok := info["pending_update_count"].(float64); ok && pending > 0 {
		logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 存在 %v 条 pending update (建议检查 webhook 端点健康度 / 上次重启前是否有未处理消息)", acc.ID, acc.AccountName, pending)
	}
	if lastErr, _ := info["last_error_message"].(string); lastErr != "" {
		logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 最近 webhook 推送失败: %s", acc.ID, acc.AccountName, lastErr)
	}
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
	StopAllTelegramPolling()
	for _, acc := range accs {
		enabled := acc.Status == 1 && acc.WebhookEnabled
		resolved, hasPublic := ResolveTelegramWebhookURL(acc)
		ready := acc.BotToken != "" && resolved != ""
		if enabled && ready {
			if vErr := ValidateTelegramWebhookURL(resolved); vErr != nil {
				now := time.Now()
				acc.LastErrorAt = &now
				acc.LastErrorMsg = "webhook URL 校验失败: " + vErr.Error()
				_ = svc.UpdateAccount(context.Background(), acc)
				logger.Warnf("[TG-Bootstrap] 账号 %d(%s) webhook URL 校验失败: %v (url=%s)", acc.ID, acc.AccountName, vErr, resolved)
				continue
			}
			if acc.WebhookURL == "" && hasPublic {
				acc.WebhookURL = resolved
			}
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
			verifyWebhookInfo(acc)
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
		if acc.BotToken != "" {
			if err := tgbot.DeleteWebhook(acc.BotToken); err != nil {
				logger.Warnf("[TG-Bootstrap] 账号 %d(%s) 清理陈旧 webhook 失败(可忽略): %v", acc.ID, acc.AccountName, err)
			} else {
				logger.Infof("[TG-Bootstrap] 账号 %d(%s) 陈旧 webhook 已清理", acc.ID, acc.AccountName)
			}
		}
	}
	EnsureTelegramMode(svc)
}
