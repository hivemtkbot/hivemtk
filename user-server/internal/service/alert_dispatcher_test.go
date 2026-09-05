package service

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// TestAlertDispatcher_DedupeWindow 同 key 去抖窗口内丢弃，窗口过后放行
func TestAlertDispatcher_DedupeWindow(t *testing.T) {
	d := NewAsyncAlertDispatcher(8, 5*time.Minute)
	base := time.Now()
	calls := atomic.Int64{}
	fn := func() { calls.Add(1) }

	d.dispatchAt(base, "rule:cpu", fn)
	d.dispatchAt(base.Add(1*time.Minute), "rule:cpu", fn) // 窗口内 → 丢弃
	d.dispatchAt(base.Add(4*time.Minute), "rule:cpu", fn) // 窗口内 → 丢弃
	d.dispatchAt(base.Add(6*time.Minute), "rule:cpu", fn) // 窗口过 → 放行
	d.dispatchAt(base.Add(6*time.Minute), "rule:mem", fn) // 不同 key → 放行

	if got := d.DedupedCount.Load(); got != 2 {
		t.Fatalf("DedupedCount = %d, want 2", got)
	}
	if got := len(d.ch); got != 3 {
		t.Fatalf("投递数 = %d, want 3", got)
	}
}

// TestAlertDispatcher_BufferFull 缓冲满非阻塞丢弃并计数
func TestAlertDispatcher_BufferFull(t *testing.T) {
	d := NewAsyncAlertDispatcher(1, 5*time.Minute) // 不 Start，任务滞留缓冲
	base := time.Now()

	d.dispatchAt(base, "k1", func() {})
	d.dispatchAt(base, "k2", func() {}) // 缓冲满 → 丢弃
	d.dispatchAt(base, "k3", func() {}) // 缓冲满 → 丢弃

	if got := d.DroppedCount.Load(); got != 2 {
		t.Fatalf("DroppedCount = %d, want 2", got)
	}
}

// TestAlertDispatcher_ConcurrentExec N=2 worker 并发执行 + panic recover + 执行计数
func TestAlertDispatcher_ConcurrentExec(t *testing.T) {
	d := NewAsyncAlertDispatcher(128, 5*time.Minute) // buffer > 任务数，全部入队
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.StopAndWait()

	var executed atomic.Int64
	total := 100
	for i := 0; i < total; i++ {
		d.Dispatch("key_"+strconv.Itoa(i), func() {
			executed.Add(1)
		})
	}
	// 混入一个 panic 任务，验证 recover 不拖垮 worker 且同样计数
	d.Dispatch("panic_probe", func() { panic("boom") })
	total++

	// 等待条件必须覆盖 ExecutedCount==total：executed 到满时 panic 任务可能刚被取走
	// 尚未走完 run 的 defer 计数（历史 flaky 根因），ExecutedCount 在 defer 里自增，
	// 等它到 total 才能保证两个断言都确定性成立。
	for i := 0; i < 500 && d.ExecutedCount.Load() < int64(total); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	// panic 任务的 fn 首行即 panic，不计入业务计数；total-1 为正常任务数
	if got := executed.Load(); got != int64(total-1) {
		t.Fatalf("业务执行数 = %d, want %d", got, total-1)
	}
	if got := d.ExecutedCount.Load(); got != int64(total) {
		t.Fatalf("ExecutedCount = %d, want %d（panic 任务也应计数）", got, total)
	}
}

// TestAlertDispatcher_StopIdempotent Stop 幂等：未 Start、重复调用均安全
func TestAlertDispatcher_StopIdempotent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Stop 不应 panic: %v", r)
		}
	}()
	d := NewAsyncAlertDispatcher(8, 5*time.Minute)
	d.StopAndWait() // 未 Start 直接停

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	d.StopAndWait()
	d.StopAndWait() // 重复调用安全
}
