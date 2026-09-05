package controller

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHub_Stop_WaitForGoroutine 验证 Stop 阻塞等待 Run 退出
//
// 场景:
//  1. 启动 Run goroutine
//  2. 调用 Stop() - 应阻塞直到 Run 真正退出
//  3. Stop 返回后, Run goroutine 已不在运行 (无残留)
func TestHub_Stop_WaitForGoroutine(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		hub.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s (likely not waiting for Run goroutine)")
	}

	done2 := make(chan struct{})
	go func() {
		hub.Stop()
		close(done2)
	}()
	select {
	case <-done2:
	case <-time.After(1 * time.Second):
		t.Fatal("second Stop() call did not return promptly")
	}
}

// TestHub_Stop_BeforeRun 验证 Run 未启动时 Stop 也安全
func TestHub_Stop_BeforeRun(t *testing.T) {
	hub := NewChatWSHub()

	done := make(chan struct{})
	go func() {
		hub.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() before Run() should return promptly")
	}
}

// TestHub_NoGoroutineLeak 验证 Hub 关闭后无 goroutine 残留
//
// 使用 goleak.VerifyNone 验证 Stop 后无 ChatWSHub 相关 goroutine 残留。
// 注意: goleak 默认会忽略 testing 主 goroutine。
func TestHub_NoGoroutineLeak(t *testing.T) {

	baseline := runtime.NumGoroutine()

	hub := NewChatWSHub()
	go hub.Run()
	waitFor(t, func() bool { return runtime.NumGoroutine() >= baseline+1 })

	clients := make([]*Client, 0, 5)
	for i := 0; i < 5; i++ {
		c := newTestClient(string(rune('a'+i)), "c")
		hub.Register(c)
		clients = append(clients, c)
	}
	time.Sleep(50 * time.Millisecond)

	for _, c := range clients {
		hub.Unregister(c)
	}
	hub.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if n := runtime.NumGoroutine(); n <= baseline+1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("goroutine 未回落: baseline=%d now=%d（疑似泄漏）", baseline, runtime.NumGoroutine())
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHub_Stop_DoneChannelClosed 验证 Stop 后 done 通道被关闭
func TestHub_Stop_DoneChannelClosed(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	hub.Stop()

	select {
	case <-hub.done:
	default:
		t.Error("expected done channel to be closed after Stop")
	}
}

// TestHub_Stop_ConcurrentCalls 验证并发 Stop 调用的安全性
func TestHub_Stop_ConcurrentCalls(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	const N = 10
	var counter int32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			hub.Stop()
			atomic.AddInt32(&counter, 1)
		}()
	}

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Fatalf("concurrent Stop() calls did not complete in 3s, counter=%d", atomic.LoadInt32(&counter))
	}

	if counter != N {
		t.Errorf("expected all %d Stop() calls to complete, got %d", N, counter)
	}
}
