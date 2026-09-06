package service

// reach_send_pipeline_quiethours.go 全渠道静默时段守卫（R-4）：
// CST 22:00-8:00 窗口判断、次日开始时间计算与进程内延迟重发队列。

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/pkg/utils"
	"hivemtk-user/internal/pkg/utils/logger"
)

var globalQuietHoursOnce sync.Once

func GetGlobalQuietHoursQueue() *MemoryQuietHoursQueue {
	globalQuietHoursOnce.Do(func() {
		globalQuietHoursQueue = NewMemoryQuietHoursQueue()
	})
	return globalQuietHoursQueue
}

var globalQuietHoursQueue *MemoryQuietHoursQueue

const (
	quietHoursStartHour = 22
	quietHoursEndHour   = 8

	nextDayFirstSendHour = quietHoursEndHour
)

func inQuietHoursWindow(t time.Time, startHour, endHour int) bool {
	h := t.In(cstZone).Hour()
	if startHour > endHour {
		return h >= startHour || h < endHour
	}
	return h >= startHour && h < endHour
}

func nextQuietHoursRelease(t time.Time, endHour int) time.Time {
	local := t.In(cstZone)
	release := time.Date(local.Year(), local.Month(), local.Day(), endHour, 0, 0, 0, cstZone)
	if !local.Before(release) {
		release = release.AddDate(0, 0, 1)
	}
	return release
}

// SendQuietHoursDeferrer R-4：quiet hours 命中后的延迟入队接口
type SendQuietHoursDeferrer interface {
	Defer(ctx context.Context, req *ReachSendRequest, sendAt time.Time) error
}

// MemoryQuietHoursQueue 进程内 quiet hours 延迟队列（R-4 最小实现）。
// Defer 入队；Start 启动后台循环将到期消息经原 pipeline 重发。
// 注意：进程重启丢队（与 MemorySendAuditLogger 同级语义）；多实例共享需迁 Redis ZSET，量级未到不做。
type MemoryQuietHoursQueue struct {
	mu      sync.Mutex
	items   []deferredSendItem
	wake    chan struct{}
	started atomic.Bool
}

type deferredSendItem struct {
	req    *ReachSendRequest
	sendAt time.Time
}

func NewMemoryQuietHoursQueue() *MemoryQuietHoursQueue {
	return &MemoryQuietHoursQueue{wake: make(chan struct{}, 1)}
}

// Defer 实现 SendQuietHoursDeferrer
func (q *MemoryQuietHoursQueue) Defer(_ context.Context, req *ReachSendRequest, sendAt time.Time) error {
	q.mu.Lock()
	q.items = append(q.items, deferredSendItem{req: req, sendAt: sendAt})
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
	logger.Warnf("[R-4] 触达命中全渠道 quiet hours，进入延迟队列 channel=%s recipient=%s send_at=%s",
		req.Channel, req.RecipientID, sendAt.Format(time.RFC3339))
	return nil
}

// Len 当前队列长度（测试/运维）
func (q *MemoryQuietHoursQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}

// Start 启动到期重发循环；每秒扫描一次，到期项经 pipeline.Send 重发。
func (q *MemoryQuietHoursQueue) Start(ctx context.Context, pipeline SendPipeline) {
	if !q.started.CompareAndSwap(false, true) {
		return
	}

	utils.SafeGo(ctx, "reach_send_pipeline.quiet_hours_queue", func(ctx context.Context) {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-q.wake:
			case <-ticker.C:
			}
			due := time.Now()
			var pending []deferredSendItem
			q.mu.Lock()
			kept := q.items[:0]
			for _, it := range q.items {
				if it.sendAt.After(due) {
					kept = append(kept, it)
				} else {
					pending = append(pending, it)
				}
			}
			q.items = kept
			q.mu.Unlock()
			for _, it := range pending {
				pipeline.Send(ctx, it.req)
			}
		}
	})
}
