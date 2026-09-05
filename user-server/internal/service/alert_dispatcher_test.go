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
	d.dispatchAt(base.Add(1*time.Minute), "rule:cpu", fn)
	d.dispatchAt(base.Add(4*time.Minute), "rule:cpu", fn)
	d.dispatchAt(base.Add(6*time.Minute), "rule:cpu", fn)
	d.dispatchAt(base.Add(6*time.Minute), "rule:mem", fn)

	if got := d.DedupedCount.Load(); got != 2 {
		t.Fatalf("DedupedCount = %d, want 2", got)
	}
	if got := len(d.ch); got != 3 {
		t.Fatalf("投递数 = %d, want 3", got)
	}
}

// TestAlertDispatcher_BufferFull 缓冲满非阻塞丢弃并计数
func TestAlertDispatcher_BufferFull(t *testing.T) {
	d := NewAsyncAlertDispatcher(1, 5*time.Minute)
	base := time.Now()

	d.dispatchAt(base, "k1", func() {})
	d.dispatchAt(base, "k2", func() {})
	d.dispatchAt(base, "k3", func() {})

	if got := d.DroppedCount.Load(); got != 2 {
		t.Fatalf("DroppedCount = %d, want 2", got)
	}
}

// TestAlertDispatcher_ConcurrentExec N=2 worker 并发执行 + panic recover + 执行计数
func TestAlertDispatcher_ConcurrentExec(t *testing.T) {
	d := NewAsyncAlertDispatcher(128, 5*time.Minute)
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

	d.Dispatch("panic_probe", func() { panic("boom") })
	total++

	for i := 0; i < 500 && executed.Load() < int64(total-1); i++ {
		time.Sleep(10 * time.Millisecond)
	}

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
	d.StopAndWait()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	d.StopAndWait()
	d.StopAndWait()
}
