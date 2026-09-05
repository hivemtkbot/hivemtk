package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"hivemtk-user/internal/pkg/utils/logger"
)

// 公开可调常量
const (
	// AlertDispatchBufferSize 投递缓冲容量
	AlertDispatchBufferSize = 64
	// AlertDispatchDedupeWindow 同 key 去抖窗口
	AlertDispatchDedupeWindow = 5 * time.Minute
	// AlertDispatchWorkers 并发 worker 数
	AlertDispatchWorkers = 2
)

type alertTask struct {
	key string
	fn  func()
}

// AsyncAlertDispatcher 告警并发去抖分发器
type AsyncAlertDispatcher struct {
	ch           chan alertTask
	DedupeWindow time.Duration

	mu     sync.Mutex
	dedupe map[string]time.Time

	DedupedCount  atomic.Int64
	DroppedCount  atomic.Int64
	ExecutedCount atomic.Int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewAsyncAlertDispatcher 构造（buffer/window 可由常量覆盖）
func NewAsyncAlertDispatcher(bufferSize int, dedupeWindow time.Duration) *AsyncAlertDispatcher {
	if bufferSize <= 0 {
		bufferSize = AlertDispatchBufferSize
	}
	if dedupeWindow <= 0 {
		dedupeWindow = AlertDispatchDedupeWindow
	}
	return &AsyncAlertDispatcher{
		ch:           make(chan alertTask, bufferSize),
		DedupeWindow: dedupeWindow,
		dedupe:       make(map[string]time.Time),
	}
}

// Dispatch 非阻塞投递：同 key 窗口内去重丢弃；缓冲满丢弃并计数。
func (d *AsyncAlertDispatcher) Dispatch(key string, fn func()) {
	d.dispatchAt(time.Now(), key, fn)
}

func (d *AsyncAlertDispatcher) dispatchAt(now time.Time, key string, fn func()) {
	if fn == nil {
		return
	}
	d.mu.Lock()

	for k, ts := range d.dedupe {
		if now.Sub(ts) >= d.DedupeWindow {
			delete(d.dedupe, k)
		}
	}
	if ts, ok := d.dedupe[key]; ok && now.Sub(ts) < d.DedupeWindow {
		d.DedupedCount.Add(1)
		d.mu.Unlock()
		return
	}
	d.dedupe[key] = now
	d.mu.Unlock()

	select {
	case d.ch <- alertTask{key: key, fn: fn}:
	default:
		d.DroppedCount.Add(1)
		logger.Warnf("[D-6] 告警投递缓冲满，丢弃 key=%s", key)
	}
}

// Start 启动 N 个 worker 并发消费（ctx 取消即退出，StopAndWait 可等待排空）
func (d *AsyncAlertDispatcher) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	d.mu.Lock()
	d.cancel = cancel
	d.mu.Unlock()
	for i := 0; i < AlertDispatchWorkers; i++ {
		d.wg.Add(1)
		go d.worker(ctx, i)
	}
	logger.Infof("[D-6] 告警去抖分发器已启动 workers=%d buffer=%d window=%v", AlertDispatchWorkers, cap(d.ch), d.DedupeWindow)
}

func (d *AsyncAlertDispatcher) worker(ctx context.Context, id int) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case task := <-d.ch:
			d.run(task)
		}
	}
}

func (d *AsyncAlertDispatcher) run(task alertTask) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[D-6] 告警任务 panic key=%s err=%v", task.key, r)
		}
		d.ExecutedCount.Add(1)
	}()
	task.fn()
}

// StopAndWait 幂等停止：未 Start 或重复调用均安全（CancelFunc 可重入；缓冲中未消费任务不保证执行）
func (d *AsyncAlertDispatcher) StopAndWait() {
	d.mu.Lock()
	cancel := d.cancel
	d.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	d.wg.Wait()
}

var (
	alertDispatcherOnce sync.Once
	alertDispatcher     *AsyncAlertDispatcher
)

// InitAlertDispatcher 初始化全局分发器并启动（main 装配阶段调用一次）
func InitAlertDispatcher(ctx context.Context) *AsyncAlertDispatcher {
	alertDispatcherOnce.Do(func() {
		alertDispatcher = NewAsyncAlertDispatcher(AlertDispatchBufferSize, AlertDispatchDedupeWindow)
		alertDispatcher.Start(ctx)
	})
	return alertDispatcher
}

// GetAlertDispatcher 获取全局分发器（未初始化返回 nil，调用方走既有同步路径）
func GetAlertDispatcher() *AsyncAlertDispatcher { return alertDispatcher }
