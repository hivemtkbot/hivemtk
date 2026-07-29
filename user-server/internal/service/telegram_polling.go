package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/config"
	"marketing/internal/pkg/utils/logger"
	"marketing/internal/pkg/utils/tgbot"
)

// ============================================================================
// Telegram Polling Fallback（无公网域名场景的自动 fallback）
// ============================================================================
//
// 设计目的：
//   - 当 user-server 部署在内网且未配置 external.public_base_url / frp 域名时，
//     Telegram 无法主动向 user-server 推送 webhook（无公网入口）。
//   - 自动 fallback 到 long polling（getUpdates）：user-server 主动出站拉取消息，
//     走 TG 官方 API，不需要公网入口。
//   - 与 webhook 互斥：register-webhook / 启动对账成功 → 停 polling；polling 启动
//     → 主动清理 TG 侧残留 webhook（避免重复消费）。
//
// 适用场景：
//   - 本地开发 / 内网调试
//   - 小型客户无 frp 部署
//   - 公网域名暂不可用时的临时方案
//
// 限制（务必明确告知运维）：
//   - 同一 BotToken 只能有一个进程在 polling，水平扩展会触发 TG 409 Conflict
//   - 不支持公网灰度 / 蓝绿部署
//   - 与 webhook 互斥切换存在几秒的消息丢失窗口（TG 侧 update queue 在切换瞬间可能重发）
//   - 推荐生产环境使用 frp + webhook；polling 仅作 fallback
//
// 实现要点：
//   - 复用 user-server 现有的 /api/webhook/telegram/{id} 入口（通过 127.0.0.1 HTTP POST）
//     → 完整复用验签 / 幂等 / 落库 / AI 编排，无需重写业务逻辑
//   - 轮询参数：timeout=25s（避开常见代理 30s 超时），limit=100，allowed_updates 与 webhook 一致
//   - 失败退避：网络错误指数退避（1s→30s 上限），409 Conflict 立即停（说明其他进程在 polling）
//   - 重启安全：ReconcileTelegramWebhooks 启动时先 StopAllTelegramPolling 再注册 webhook

// TelegramPollingEnvKey 显式启用 / 禁用 polling 的环境变量
//   - "1"/"true"/"yes" → 强制启用
//   - "0"/"false"/"no" → 强制禁用
//   - 未设置 → 自动判定（无 public_base_url 时启用）
const TelegramPollingEnvKey = "TELEGRAM_POLLING_ENABLED"

// 默认轮询参数（与 Telegram Bot API 官方建议对齐）
const (
	tgPollingTimeoutSeconds = 25  // 略小于 30s 通用代理超时
	tgPollingLimit          = 100 // 单次返回最大 update 数
	tgPollingBackoffMin     = 1 * time.Second
	tgPollingBackoffMax     = 30 * time.Second
)

// telegramPollingState 维护每个账号的轮询状态
//
// done 通道：worker 退出前 close，StopAllTelegramPolling 等待所有 done 确认 ctx 已真正退出。
// 没有 done 的话，Stop 仅触发 cancel；但 goroutine 内部的 GetUpdates 长轮询
// 还在 hold（最多 25s 才会响应 cancel），期间若启动期对账立即走 setWebhook，
// 会与还未退出的 polling 协程产生「同一 update 被两次处理」的风险。
//
// lockHeld 标记：本进程是否持有该账号的分布式锁（S3-6）。
// true 时 StopTelegramPolling 会调用 ReleasePollingLock 释放；
// false 时（锁本就没拿到，或已被抢占）不释放，避免误清其他实例的锁。
type telegramPollingState struct {
	cancel   context.CancelFunc
	done     chan struct{}
	lockHeld bool
}

// globalTelegramPolling 注册中心：accountID(uint) → cancel func
//
// 使用 map+Mutex：项目内 TG 账号规模 ≤ 100，单实例无锁热点，足够。
// 进程退出时由 StopAllTelegramPolling 兜底。
var (
	telegramPollingMu     sync.Mutex
	telegramPollingStates = make(map[uint]*telegramPollingState)
)

// StopAllTelegramPolling 停掉所有 Telegram 账号的 polling 协程
//
// 启动期对账（ReconcileTelegramWebhooks）必须先调用本函数，避免 webhook 和 polling
// 同时工作导致消息被消费两次。
//
// 同步语义（修复 S2-2）：本函数会等待每个 polling worker 的 done 通道关闭后再返回，
// 确保 ctx 真正退出后才允许调用方继续 setWebhook。否则 worker 内部 GetUpdates
// 长轮询可能在 cancel 后 0-25s 才响应，期间与新 webhook 产生双消费。
//
// S3-6：worker 全部退出后，遍历释放本进程持有的所有 polling 锁（仅当 owner 是本进程时）。
//
// 超时：默认无超时（与 SessionTTLCron.Stop 同模式）；调用方可用 select+time.After 自行限时。
func StopAllTelegramPolling() {
	telegramPollingMu.Lock()
	type stateWithID struct {
		id    uint
		state *telegramPollingState
	}
	states := make([]stateWithID, 0, len(telegramPollingStates))
	for id, s := range telegramPollingStates {
		states = append(states, stateWithID{id: id, state: s})
	}
	telegramPollingStates = make(map[uint]*telegramPollingState)
	telegramPollingMu.Unlock()
	for _, sw := range states {
		if sw.state != nil && sw.state.cancel != nil {
			sw.state.cancel()
		}
	}
	// 同步等待：所有 worker 真正退出后返回
	for _, sw := range states {
		if sw.state != nil && sw.state.done != nil {
			<-sw.state.done
		}
	}
	// S3-6：worker 已退出，遍历释放本进程持有的所有锁
	// 五层架构修复：service 不再直接调 db.GetDB()，统一走 service 门面
	// （service 门面内部转调 repository.TelegramPollingLockRepository）
	for _, sw := range states {
		if sw.state != nil && sw.state.lockHeld {
			if rerr := ReleasePollingLock(context.Background(), nil, sw.id); rerr != nil {
				logger.Warnf("[TG-Polling] 账号 %d 释放锁失败: %v", sw.id, rerr)
			}
		}
	}
}

// StartTelegramPolling 为单个 Telegram 账号启动 polling 协程
//
// 入参 acc 必须 BotToken 非空；若该账号已有 polling 协程在运行，先停旧的再启新的（避免泄漏）。
// 启动失败仅记录日志，不返回 error（polling 失败不该阻断其他账号 / 启动流程）。
//
// S3-6 分布式锁：启动前先抢占 telegram_accounts.polling_owner 锁（DB 行级原子 UPDATE）。
// 抢占失败（被其他实例持有且心跳未过期）→ 不启动 polling，避免多实例重复消费触发 TG 409。
// 抢占成功 → 启动 worker + 30s 心跳协程；worker 退出时自动释放锁。
func StartTelegramPolling(acc *model.TelegramAccount) {
	if acc == nil || acc.BotToken == "" {
		return
	}
	// 先停旧的（同步等待，确保旧 worker 真正退出再启新的）
	stopTelegramPollingLocked(acc.ID)

	// S3-6 分布式锁：DB 层抢占
	// 五层架构修复：service 不再直接调 db.GetDB()，统一走 service 门面
	// （service 门面内部转调 repository.TelegramPollingLockRepository；DB 未就绪时
	// repository 内部判 nil 并返回 ErrPollingLockDBNil）
	acquired, owner, lastHB, lockErr := TryAcquirePollingLock(context.Background(), nil, acc.ID)
	if lockErr != nil {
		logger.Warnf("[TG-Polling] 账号 %d(%s) 抢占锁失败: %v", acc.ID, acc.AccountName, lockErr)
		return
	}
	if !acquired {
		hbStr := "<nil>"
		if lastHB != nil {
			hbStr = lastHB.Format(time.RFC3339)
		}
		logger.Infof("[TG-Polling] 账号 %d(%s) 锁被其他实例持有 (owner=%s, last_heartbeat=%s)，本进程不启动 polling", acc.ID, acc.AccountName, owner, hbStr)
		return
	}
	logger.Infof("[TG-Polling] 账号 %d(%s) 分布式锁抢占成功 (worker=%s)", acc.ID, acc.AccountName, GetPollingWorkerID())

	ctx, cancel := context.WithCancel(context.Background())
	state := &telegramPollingState{cancel: cancel, done: make(chan struct{}), lockHeld: true}
	telegramPollingMu.Lock()
	telegramPollingStates[acc.ID] = state
	telegramPollingMu.Unlock()

	accountID := acc.ID
	botToken := acc.BotToken
	accountName := acc.AccountName

	go runTelegramPollingWorker(ctx, accountID, botToken, accountName, acc.WebhookSecret, state.done, state)
	logger.Infof("[TG-Polling] 账号 %d(%s) polling 协程已启动", accountID, accountName)
}

// stopTelegramPollingLocked 停掉指定账号的 polling 协程（持锁版本，调用方需保证已上锁）
//
// 同步等待 worker 真正退出（通过 done 通道）：
//   - cancel 触发后 worker 内部 GetUpdates 长轮询最多 25s 才响应
//   - 若不等 done，调用方可能立即进入下一段逻辑（启新 worker / setWebhook），
//     产生短窗口双消费
//   - 本函数必须释放锁后再等 done（持锁等 done = 持锁 25s）
//
// S3-6：worker 退出后释放分布式锁（仅当本进程持有时）
func stopTelegramPollingLocked(accountID uint) {
	telegramPollingMu.Lock()
	s, ok := telegramPollingStates[accountID]
	if ok {
		delete(telegramPollingStates, accountID)
	}
	telegramPollingMu.Unlock()
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}
	// S3-6：worker 已退出，释放分布式锁（best-effort，失败仅记日志）
	// 五层架构修复：service 不再直接调 db.GetDB()，统一走 service 门面
	if s.lockHeld {
		if rerr := ReleasePollingLock(context.Background(), nil, accountID); rerr != nil {
			logger.Warnf("[TG-Polling] 账号 %d 释放锁失败: %v", accountID, rerr)
		} else {
			logger.Infof("[TG-Polling] 账号 %d 分布式锁已释放", accountID)
		}
	}
}

// StopTelegramPolling 停掉指定账号的 polling 协程（公开 API，同步等待 worker 真正退出）
func StopTelegramPolling(accountID uint) {
	telegramPollingMu.Lock()
	s, ok := telegramPollingStates[accountID]
	if ok {
		delete(telegramPollingStates, accountID)
	}
	telegramPollingMu.Unlock()
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.done != nil {
		<-s.done
	}
	// S3-6：worker 已退出，释放分布式锁（best-effort）
	// 五层架构修复：service 不再直接调 db.GetDB()，统一走 service 门面
	if s.lockHeld {
		if rerr := ReleasePollingLock(context.Background(), nil, accountID); rerr != nil {
			logger.Warnf("[TG-Polling] 账号 %d 释放锁失败: %v", accountID, rerr)
		} else {
			logger.Infof("[TG-Polling] 账号 %d 分布式锁已释放", accountID)
		}
	}
}

// IsTelegramPollingActive 检查某账号 polling 是否在运行（用于 status 端点）
func IsTelegramPollingActive(accountID uint) bool {
	telegramPollingMu.Lock()
	defer telegramPollingMu.Unlock()
	_, ok := telegramPollingStates[accountID]
	return ok
}

// IsTelegramPollingEnabled 判定当前部署是否启用 polling
//
// 优先级：
//  1. 环境变量 TELEGRAM_POLLING_ENABLED（"1"/"true"/"yes" → 强制启用；"0"/"false"/"no" → 强制禁用）
//  2. 配置 external.public_base_url 是否设置
//     - 已设置 → 公网域名可用，应走 webhook；polling 禁用
//     - 未设置 → 内网/本地部署，自动启用 polling
//
// 注意：与 webhook 互斥（一个 token 同时只允许一种模式）。
func IsTelegramPollingEnabled() bool {
	if v := strings.ToLower(strings.TrimSpace(os.Getenv(TelegramPollingEnvKey))); v != "" {
		switch v {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return config.GetPublicBaseURL() == ""
}

// EnsureTelegramMode 根据部署模式自动选择 webhook 或 polling
//
//   - polling 启用（无公网域名）→ 停 webhook、启动 polling
//   - polling 禁用（有公网域名）→ 走原有 webhook 路径（由调用方负责）
//
// 启动期对账（ReconcileTelegramWebhooks）调用本函数决定每个账号的模式。
// 单账号 register-webhook 成功后也应该调用本函数（让 polling 停掉以避免重复消费）。
func EnsureTelegramMode(svc *TelegramService) {
	if !IsTelegramPollingEnabled() {
		return
	}
	accs, err := svc.ListAccounts(context.Background())
	if err != nil {
		logger.Warnf("[TG-Mode] 列举 Telegram 账号失败: %v", err)
		return
	}
	for _, acc := range accs {
		if acc.BotToken == "" || acc.Status != 1 {
			continue
		}
		// 停 webhook（清理 TG 侧残留），避免双消费
		if err := tgbot.DeleteWebhook(acc.BotToken); err != nil {
			logger.Warnf("[TG-Mode] 账号 %d(%s) 清理 webhook 失败(可忽略): %v", acc.ID, acc.AccountName, err)
		}
		StartTelegramPolling(acc)
	}
}

// runTelegramPollingWorker 单一账号的 polling 循环
//
// 主循环逻辑：
//  1. 调用 getUpdates（offset=lastUpdateID+1, timeout=25s, limit=100）
//  2. 解析响应；成功 → 把每个 update POST 到 127.0.0.1:8204/api/webhook/telegram/{id}
//     复用现有 webhook 入口的验签 / 幂等 / 落库 / AI 编排
//  3. 推进 offset；网络错误 → 指数退避；ctx 取消 → 退出
//  4. 409 Conflict（说明其他进程在 polling）→ 立即退出（运维应当只用单实例）
//
// done 通道：worker 退出前 close，StopAllTelegramPolling 通过它同步等待。
//
// S3-6 心跳：worker 启动时同时启动 30s 心跳协程。心跳失败（锁被抢占）→ 主动 cancel worker，
// 避免本进程继续 polling 与新持有者产生双消费。
func runTelegramPollingWorker(ctx context.Context, accountID uint, botToken, accountName, webhookSecret string, done chan struct{}, state *telegramPollingState) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[TG-Polling] 账号 %d(%s) 协程 panic: %v", accountID, accountName, r)
		}
		// 退出时从注册中心移除
		telegramPollingMu.Lock()
		delete(telegramPollingStates, accountID)
		telegramPollingMu.Unlock()
		// 通知 Stop 等待方：worker 真正退出
		if done != nil {
			close(done)
		}
		logger.Infof("[TG-Polling] 账号 %d(%s) polling 协程已退出", accountID, accountName)
	}()

	// S3-6：启动心跳协程
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go runPollingHeartbeat(heartbeatCtx, accountID, accountName, state)

	offset := int64(0)
	backoff := tgPollingBackoffMin
	// 同进程内多账号共用一个 HTTP 客户端（连接复用）
	// 支持 HTTP_PROXY / HTTPS_PROXY 环境变量
	client := &http.Client{
		Timeout: tgPollingTimeoutSeconds*time.Second + 5*time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
		},
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := tgbot.GetUpdates(ctx, botToken, offset, tgPollingLimit, tgPollingTimeoutSeconds)
		if err != nil {
			// 409 Conflict：另一实例在 polling；本协程退出
			if isTelegramConflictError(err) {
				logger.Warnf("[TG-Polling] 账号 %d(%s) 检测到 409 Conflict（同 token 另一实例在 polling），本协程退出: %v", accountID, accountName, err)
				return
			}
			logger.Warnf("[TG-Polling] 账号 %d(%s) getUpdates 失败，%v 后重试: %v", accountID, accountName, backoff, err)
			if sleepCtx(ctx, backoff) {
				return
			}
			// 指数退避
			backoff *= 2
			if backoff > tgPollingBackoffMax {
				backoff = tgPollingBackoffMax
			}
			continue
		}
		backoff = tgPollingBackoffMin // 成功后重置

		for _, raw := range updates {
			// 推进 offset：每个 update 单独处理
			uid := extractUpdateID(raw)
			if uid > 0 {
				offset = uid + 1
			}
			if err := deliverTelegramUpdate(ctx, client, accountID, webhookSecret, raw); err != nil {
				logger.Warnf("[TG-Polling] 账号 %d(%s) 投递 update 失败: %v", accountID, accountName, err)
			}
		}
	}
}

// runPollingHeartbeat S3-6 心跳协程：每 30s 更新 polling_heartbeat_at 续约锁
//
// 心跳失败（锁已被其他进程抢占）→ 主动触发 worker 退出，
// 避免双消费。心跳协程与 worker 共用 ctx（worker 退出时自动取消）。
func runPollingHeartbeat(ctx context.Context, accountID uint, accountName string, state *telegramPollingState) {
	ticker := time.NewTicker(PollingLockHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 五层架构修复：service 不再直接调 db.GetDB()，统一走 service 门面
			// （service 门面内部转调 repository.TelegramPollingLockRepository）
			lockLost, err := HeartbeatPollingLock(ctx, nil, accountID)
			if err != nil {
				// DB 未就绪时静默续约（保持原行为：避免抖动告警）
				if !isPollingLockDBNotReadyError(err) {
					logger.Warnf("[TG-Polling] 账号 %d(%s) 心跳失败: %v", accountID, accountName, err)
				}
				continue
			}
			if lockLost {
				logger.Warnf("[TG-Polling] 账号 %d(%s) 锁已丢失（被其他进程抢占），主动停止 worker", accountID, accountName)
				// 标记 lockHeld=false，让 Stop 不要再尝试释放（锁已经不是我们的了）
				telegramPollingMu.Lock()
				if s, ok := telegramPollingStates[accountID]; ok {
					s.lockHeld = false
				}
				telegramPollingMu.Unlock()
				// 主动 cancel worker
				if state != nil && state.cancel != nil {
					state.cancel()
				}
				return
			}
		}
	}
}

// isPollingLockDBNotReadyError 判定 service 门面返回的 error 是否是「DB 未就绪」
//
// DB 未就绪时心跳会持续失败，但不需要告警（启动期正常时序），仅跳过本次。
func isPollingLockDBNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "db is nil")
}

// deliverTelegramUpdate 把单条 update 投递到本机的 webhook 入口
//
// 复用现有 /api/webhook/telegram/{id} 入口的完整业务逻辑（验签 / 幂等 / 落库 / AI 编排），
// 避免在 polling worker 里重写一遍。
//
// 失败重试：网络错误重试 2 次（200ms 间隔），最终失败仅记录日志。
func deliverTelegramUpdate(ctx context.Context, client *http.Client, accountID uint, webhookSecret string, raw json.RawMessage) error {
	url := fmt.Sprintf("%s/api/webhook/telegram/%d", config.DefaultUserServerBaseURL, accountID)
	var lastErr error
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Telegram-Polling-Source", "1") // 标记来源（便于监控 / 调试）
		// SOP-041 修复：polling 兜底投递到本地 webhook 入口时，必须携带与 setWebhook
		// 一致的 X-Telegram-Bot-Api-Secret-Token，否则本地验签 401（消息丢失）。
		// webhook_secret 为空时（未配置验签）不附加，handler 会跳过验签，向后兼容。
		if webhookSecret != "" {
			req.Header.Set("X-Telegram-Bot-Api-Secret-Token", webhookSecret)
		}
		req.ContentLength = int64(len(raw))
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if sleepCtx(ctx, 200*time.Millisecond) {
				return ctx.Err()
			}
			continue
		}
		// 必须读完 body 才能复用连接
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		// 4xx 业务错误（账号未启用、验签失败等）→ 不重试
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return fmt.Errorf("polling → webhook 返回 %d（不重试）", resp.StatusCode)
		}
		lastErr = fmt.Errorf("polling → webhook 返回 %d", resp.StatusCode)
		if sleepCtx(ctx, 200*time.Millisecond) {
			return ctx.Err()
		}
	}
	return lastErr
}

// extractUpdateID 提取单条 update 的 update_id（容错处理非数字 / 缺失）
func extractUpdateID(raw json.RawMessage) int64 {
	var probe struct {
		UpdateID int64 `json:"update_id"`
	}
	_ = json.Unmarshal(raw, &probe)
	return probe.UpdateID
}

// isTelegramConflictError 判定错误是否是 409 Conflict（polling 重复实例）
func isTelegramConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "409") || strings.Contains(msg, "conflict") || strings.Contains(msg, "terminated by other getupdates")
}

// sleepCtx 等待指定时长，ctx 取消时立即返回 true
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}
