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

	"hivemtk-user/internal/config"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/httpclient"
	"hivemtk-user/internal/pkg/tgbot"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)


// TelegramPollingEnvKey 显式启用 / 禁用 polling 的环境变量
//   - "1"/"true"/"yes" → 强制启用
//   - "0"/"false"/"no" → 强制禁用
//   - 未设置 → 自动判定（无 public_base_url 时启用）
const TelegramPollingEnvKey = "TELEGRAM_POLLING_ENABLED"

// 默认轮询参数（与 Telegram Bot API 官方建议对齐）
const (
	tgPollingTimeoutSeconds = 25  
	tgPollingLimit          = 100 
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
	for _, sw := range states {
		if sw.state != nil && sw.state.done != nil {
			<-sw.state.done
		}
	}
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
	stopTelegramPollingLocked(acc.ID)

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
		telegramPollingMu.Lock()
		delete(telegramPollingStates, accountID)
		telegramPollingMu.Unlock()
		if done != nil {
			close(done)
		}
		logger.Infof("[TG-Polling] 账号 %d(%s) polling 协程已退出", accountID, accountName)
	}()

	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go runPollingHeartbeat(heartbeatCtx, accountID, accountName, state)

	offset := int64(0)
	backoff := tgPollingBackoffMin
	// v3 审计 P2-34 修复：使用全局 http.Client + connection pool
	// 原：每个 worker 独立 &http.Client + &http.Transport → 连接数随 worker 线性增长
	// 新：复用 httpclient.Client（全局共享连接池）
	client := httpclient.Client

	// 并发投递本轮 update 的 worker 上限（本地 webhook 幂等，顺序无关）
	const tgPollingDeliverConcurrency = 16

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		updates, err := tgbot.GetUpdates(ctx, botToken, offset, tgPollingLimit, tgPollingTimeoutSeconds)
		if err != nil {
			if isTelegramConflictError(err) {
				logger.Warnf("[TG-Polling] 账号 %d(%s) 检测到 409 Conflict（同 token 另一实例在 polling），本协程退出: %v", accountID, accountName, err)
				return
			}
			// R43: 401 = token 永久失效（被撤销/伪造），无限重试毫无意义 →
			// 停止本账号 polling 并单次告警（与 409 同级的确定性退出路径）
			if isTelegramAuthError(err) {
				logger.Errorf("[TG-Polling] 账号 %d(%s) token 鉴权失败(401)，已停止该账号 polling——请在渠道页更换有效 Bot Token 后重新启用: %v", accountID, accountName, err)
				return
			}
			logger.Warnf("[TG-Polling] 账号 %d(%s) getUpdates 失败，%v 后重试: %v", accountID, accountName, backoff, err)
			if sleepCtx(ctx, backoff) {
				return
			}
			backoff *= 2
			if backoff > tgPollingBackoffMax {
				backoff = tgPollingBackoffMax
			}
			continue
		}
		backoff = tgPollingBackoffMin 

		maxUID := offset - 1
		sem := make(chan struct{}, tgPollingDeliverConcurrency)
		var wg sync.WaitGroup
		for _, raw := range updates {
			uid := extractUpdateID(raw)
			if uid > maxUID {
				maxUID = uid
			}
			r := raw
			wg.Add(1)
			sem <- struct{}{}
			// 最高标准审计 P1-3 修复：TG update 投递（消息发送路径）改走 SafeGo
			utils.SafeGo(ctx, "telegram_polling.deliver", func(_ context.Context) {
				defer wg.Done()
				defer func() { <-sem }()
				if err := deliverTelegramUpdate(ctx, client, accountID, webhookSecret, r); err != nil {
					logger.Warnf("[TG-Polling] 账号 %d(%s) 投递 update 失败: %v", accountID, accountName, err)
				}
			})
		}
		wg.Wait()
		offset = maxUID + 1
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
			lockLost, err := HeartbeatPollingLock(ctx, nil, accountID)
			if err != nil {
				if !isPollingLockDBNotReadyError(err) {
					logger.Warnf("[TG-Polling] 账号 %d(%s) 心跳失败: %v", accountID, accountName, err)
				}
				continue
			}
			if lockLost {
				logger.Warnf("[TG-Polling] 账号 %d(%s) 锁已丢失（被其他进程抢占），主动停止 worker", accountID, accountName)
				telegramPollingMu.Lock()
				if s, ok := telegramPollingStates[accountID]; ok {
					s.lockHeld = false
				}
				telegramPollingMu.Unlock()
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
		req.Header.Set("X-Telegram-Polling-Source", "1") 
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
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
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
// isTelegramAuthError R43: 401 Unauthorized = token 永久无效（确定性退出，不重试）
func isTelegramAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "401") || strings.Contains(msg, "Unauthorized")
}

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

