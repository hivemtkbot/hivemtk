package service

// Webhook 事件恢复扫描器 —— ChatbotX「先落库后处理 + 对账自愈」模式的移植。
//
// 背景：WebhookService.Receive 同步落库 webhook_events 后投进程内 chan
// （容量 512，4 worker）。chan 在进程重启时丢失，落在队列里未处理的事件
// 将永远停在 processed=false，只能人工对账。本扫描器以 DB 为真源做兜底：
//
//	每 30s 扫描 created_at 超过 60s 冷却期且 processed=false 的事件，
//	经 Redis SetNX 认领门闸（跨实例互斥）后重放 handleJob 全链路。
//
// 可靠性语义（at-least-once）：
//   - 若 fast path 已处理但未及 markProcessed 就崩溃，重放会重复触发副作用；
//     幂等性由下游兜底：message_hub 复合唯一索引（platform,msg_id,conversation_id）
//     duplicate 容忍 + AI 出站 agent_runtime.ClaimReply(eventID) 事件级防重
//     + AI 处理分布式锁 mtk:ai_processing:{convID}。
//   - 跨实例互斥不依赖 SQL 行锁（事务提交即释放，覆盖不了处理窗口），
//     由 Redis SetNX mtk:webhook:recovering:{id} 承担；Redis 异常时放行
//     （可用性优先，与 isDuplicate 同一哲学），靠上述下游幂等收敛。
//   - 毒丸防护：Redis INCR mtk:webhook:retry:{eventID} 计数（TTL 24h），
//     连续失败达上限后标记 processed 跳过，防止坏事件每轮阻塞扫描。
//
// 开关：WEBHOOK_RECOVERY_ENABLED（默认开）；WEBHOOK_RECOVERY_INTERVAL_SECONDS
// 调扫描间隔。Stop 复用 WebhookService 的 stopCh/wg，随服务统一优雅退出。
import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/repository"
)

const (
	webhookRecoveryCooldownDefault = 60 * time.Second
	webhookRecoveryIntervalDefault = 30 * time.Second
	webhookRecoveryBatchDefault    = 100
	webhookRecoveryMaxRetry        = 5
	webhookRecoveryGateTTL         = 5 * time.Minute
	webhookRecoveryRetryTTL        = 24 * time.Hour

	// TruncateForStore 的截断标记：超限 payload 无法完整重放，直接终止重试。
	webhookRawTruncatedSuffix = "...[truncated]"
)

type webhookRecoveryScanner struct {
	svc       *WebhookService
	eventRepo *repository.WebhookEventRepository

	// handleFn 重放入口，默认 svc.handleJob；测试可注入桩函数解耦全链路依赖。
	handleFn func(ctx context.Context, job *webhookJob)

	interval  time.Duration
	cooldown  time.Duration
	batchSize int
	enabled   bool

	stopCh chan struct{}
	mu     sync.Mutex

	// lastID 扫描游标：id ASC limit N 的窗口若被 100 条反复失败的事件占满，
	// 会饿死后续积压（二次审查 S3 修复）。游标越过本轮批次，批不满时回绕。
	lastID uint64
}

// newWebhookRecoveryScanner 构造扫描器；db 为 nil（零值服务/单测）时返回 nil。
func newWebhookRecoveryScanner(svc *WebhookService) *webhookRecoveryScanner {
	if svc == nil || svc.db == nil {
		return nil
	}
	eventRepo := svc.eventRepo
	if eventRepo == nil {
		eventRepo = repository.NewWebhookEventRepository()
		repository.SetWebhookEventRepoDB(eventRepo, svc.db)
	}
	return &webhookRecoveryScanner{
		svc:       svc,
		eventRepo: eventRepo,
		handleFn:  svc.handleJob,
		interval:  webhookEnvSeconds("WEBHOOK_RECOVERY_INTERVAL_SECONDS", webhookRecoveryIntervalDefault),
		cooldown:  webhookEnvSeconds("WEBHOOK_RECOVERY_COOLDOWN_SECONDS", webhookRecoveryCooldownDefault),
		batchSize: webhookEnvInt("WEBHOOK_RECOVERY_BATCH", webhookRecoveryBatchDefault),
		enabled:   os.Getenv("WEBHOOK_RECOVERY_ENABLED") != "false",
		stopCh:    make(chan struct{}),
	}
}

// markProcessed 标记事件已处理（扫描器自持 repo，不依赖 svc.eventRepo 装配状态）。
func (sc *webhookRecoveryScanner) markProcessed(ctx context.Context, evt *model.WebhookEvent) {
	now := time.Now()
	evt.Processed = true
	evt.ProcessedAt = &now
	if sc.eventRepo != nil {
		_ = sc.eventRepo.Update(ctx, evt)
	}
}

// start 启动扫描循环（幂等；disabled 或 scanner 为 nil 时静默跳过）。
// 复用 svc.wg/stopCh，WebhookService.Stop 时一并退出。
func (s *WebhookService) startRecoveryScanner() {
	sc := newWebhookRecoveryScanner(s)
	if sc == nil || !sc.enabled {
		return
	}
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()

	ctx := context.Background()
	utils.SafeGo(ctx, "webhook.recovery_scanner", func(ctx context.Context) {
		defer s.wg.Done()
		ticker := time.NewTicker(sc.interval)
		defer ticker.Stop()
		logger.Infof("[WebhookRecovery] scanner started interval=%s cooldown=%s batch=%d", sc.interval, sc.cooldown, sc.batchSize)
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				sc.scanOnce(ctx)
			}
		}
	})
}

// scanOnce 执行一轮扫描：认领 → 重放 → markProcessed。返回本轮重放的事件数。
func (sc *webhookRecoveryScanner) scanOnce(ctx context.Context) int {
	if sc.eventRepo == nil {
		return 0
	}
	cutoff := time.Now().Add(-sc.cooldown)
	events, err := sc.eventRepo.ListStaleUnprocessedAfter(ctx, cutoff, sc.lastID, sc.batchSize)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Msg("[WebhookRecovery] scan query failed")
		return 0
	}
	if len(events) == 0 {
		sc.lastID = 0 // 回绕：从头再扫（事件可能已被 fast path 收敛或下轮可认领）
		return 0
	}
	if len(events) < sc.batchSize {
		sc.lastID = 0 // 不足一批说明已扫到尾部，下轮回绕
	} else {
		sc.lastID = uint64(events[len(events)-1].ID)
	}
	replayed := 0
	for _, evt := range events {
		token, claimed := sc.claim(ctx, evt)
		if !claimed {
			continue
		}
		sc.replay(ctx, evt)
		// 处理完立即释放门闸：markProcessed 失败的事件下一轮即可重试，
		// 收敛由毒丸计数兜底（5 次上限）
		_, _ = cache.GetGlobalCache().ReleaseLock(context.Background(),
			"mtk:webhook:recovering:"+strconv.FormatUint(uint64(evt.ID), 10), token)
		replayed++
	}
	if replayed > 0 {
		logger.Infof("[WebhookRecovery] replayed %d/%d stale events", replayed, len(events))
	}
	return replayed
}

// claim Redis SetNX 认领门闸：同一事件同一时刻只被一个实例重放。
// 门闸值 = token，处理完成后 ReleaseLock 校验持有者释放（防误删他人门闸）。
// Redis 异常时放行（fail-open），副作用幂等由下游兜底。
func (sc *webhookRecoveryScanner) claim(ctx context.Context, evt *model.WebhookEvent) (string, bool) {
	key := "mtk:webhook:recovering:" + strconv.FormatUint(uint64(evt.ID), 10)
	token := "recovery-" + strconv.FormatUint(uint64(evt.ID), 10) + "-" + time.Now().Format("150405.000000000")
	ok, err := cache.GetGlobalCache().SetNX(context.Background(), key, token, webhookRecoveryGateTTL)
	if err != nil {
		logger.Ctx(ctx).Warn().Err(err).Uint("event_id", evt.ID).Msg("[WebhookRecovery] claim backend error, proceeding (fail-open)")
		return "", true
	}
	if !ok {
		return "", false // 其他实例正在处理
	}
	return token, true
}

// replay 重放单个事件：重建 webhookJob 后复用 handleJob 全链路。
// 返回是否真正执行了重放（毒丸/空账号上下文/不可重放载荷返回 false）。
func (sc *webhookRecoveryScanner) replay(ctx context.Context, evt *model.WebhookEvent) bool {
	// 毒丸防护：多次重放仍失败的事件终止重试，避免永久阻塞扫描。
	if n := sc.incrRetry(ctx, evt.EventID); n > webhookRecoveryMaxRetry {
		logger.Ctx(ctx).Warn().Uint("event_id", evt.ID).Str("event", evt.EventID).Int64("attempts", n).Msg("[WebhookRecovery] poison event, giving up")
		sc.markProcessed(ctx, evt)
		return false
	}

	// 截断载荷无法完整重放：JSON 已破损，解析必然失败，直接终止重试。
	if evt.RawData == "" || strings.HasSuffix(evt.RawData, webhookRawTruncatedSuffix) {
		logger.Ctx(ctx).Warn().Uint("event_id", evt.ID).Msg("[WebhookRecovery] raw payload missing/truncated, cannot replay")
		sc.markProcessed(ctx, evt)
		return false
	}

	// 存量行无 account_id（本字段随本特性引入），无法重建渠道上下文。
	if evt.AccountID == "" {
		logger.Ctx(ctx).Warn().Uint("event_id", evt.ID).Str("platform", evt.Platform).Msg("[WebhookRecovery] legacy row without account_id, cannot replay")
		sc.markProcessed(ctx, evt)
		return false
	}

	job := &webhookJob{
		event:   evt,
		raw:     []byte(evt.RawData),
		header:  nil, // 验签发生在 Receive 落库前，重放无需原始 header
		channel: WebhookChannel(evt.Platform),
		account: evt.AccountID,
		payload: nil, // handleJob 内部从 raw 重新解析
	}

	// handleJob 对已知渠道的空 hub 结果会自行 markProcessed；
	// 这里只兜底它未覆盖的早退路径（解析失败等），保证不无限重放。
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[WebhookRecovery] replay panic event=%s: %v", evt.EventID, r)
			sc.markProcessed(ctx, evt)
		}
	}()
	latest, err := sc.eventRepo.GetByID(ctx, evt.ID)
	if err == nil && latest != nil && latest.Processed {
		return false // fast path 恰好已完成，跳过重放
	}
	sc.handleFn(ctx, job)
	if fresh, _ := sc.eventRepo.GetByID(ctx, evt.ID); fresh != nil && !fresh.Processed {
		// handleFn 早退（如 parse 失败未 markProcessed）→ 本轮算失败，留给毒丸计数收敛
		return false
	}
	return true
}

func (sc *webhookRecoveryScanner) incrRetry(ctx context.Context, eventID string) int64 {
	if eventID == "" {
		return 0
	}
	key := "mtk:webhook:retry:" + eventID
	n, err := cache.GetGlobalCache().Incr(context.Background(), key, webhookRecoveryRetryTTL)
	if err != nil {
		// Redis 异常时返回 0 放行：宁可重试也不误杀正常事件
		return 0
	}
	return n
}

func webhookEnvSeconds(name string, def time.Duration) time.Duration {
	n := webhookEnvInt(name, int(def/time.Second))
	if n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}
