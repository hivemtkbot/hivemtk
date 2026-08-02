package controller

// chat_ws_hub_stop_test.go ChatWSHub.Stop goroutine 等待测试
//
// 验证 修复: Stop 必须等待 Run goroutine 真正退出，
// 防止 Hub 关闭后 Run goroutine 残留。
//
// 不依赖 gorilla/websocket: 通过给 Client.Conn 传 nil, 避开 Conn.Close() 调用

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
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
	// 等待 Run 启动
	time.Sleep(20 * time.Millisecond)

	// 调用 Stop - 应阻塞直到 Run 真正退出
	done := make(chan struct{})
	go func() {
		hub.Stop()
		close(done)
	}()

	// 验证 Stop 在合理时间内返回 (Run 退出耗时 < 1s)
	select {
	case <-done:
		// OK, Stop 返回了
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s (likely not waiting for Run goroutine)")
	}

	// 此时再调用 Stop 也不应阻塞 (幂等性)
	done2 := make(chan struct{})
	go func() {
		hub.Stop()
		close(done2)
	}()
	select {
	case <-done2:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("second Stop() call did not return promptly")
	}
}

// TestHub_Stop_BeforeRun 验证 Run 未启动时 Stop 也安全
func TestHub_Stop_BeforeRun(t *testing.T) {
	hub := NewChatWSHub()
	// 不启动 Run

	done := make(chan struct{})
	go func() {
		hub.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK, Stop 安全返回
	case <-time.After(1 * time.Second):
		t.Fatal("Stop() before Run() should return promptly")
	}
}

// TestHub_NoGoroutineLeak 验证 Hub 关闭后无 goroutine 残留
//
// 使用 goleak.VerifyNone 验证 Stop 后无 ChatWSHub 相关 goroutine 残留。
// 注意: goleak 默认会忽略 testing 主 goroutine。
func TestHub_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t,
		// 忽略可能的 background goroutine (例如 runtime 监控)
		goleak.IgnoreTopFunction("github.com/rs/zerolog.AsyncWriter.func1"),
		// 忽略预存在的 SessionTTLCron (单例 cron, 进程级生命周期, 非 ChatWSHub 责任)
		goleak.IgnoreTopFunction("marketing/internal/service.(*SessionTTLCron).run"),
	)

	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	// 注册几个 client 触发更多内部 goroutine 状态
	clients := make([]*Client, 0, 5)
	for i := 0; i < 5; i++ {
		c := newTestClient(string(rune('a'+i)), "c")
		hub.Register(c)
		clients = append(clients, c)
	}
	time.Sleep(50 * time.Millisecond)

	// 注销所有 client
	for _, c := range clients {
		hub.Unregister(c)
	}
	time.Sleep(20 * time.Millisecond)

	// 关闭 Hub
	hub.Stop()

	// goleak.VerifyNone 会在 defer 中执行, 验证无残留
}

// TestHub_Stop_DoneChannelClosed 验证 Stop 后 done 通道被关闭
func TestHub_Stop_DoneChannelClosed(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	hub.Stop()

	// 验证 done channel 已被关闭
	select {
	case <-hub.done:
		// OK
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

	// 等待所有 Stop 调用完成
	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatalf("concurrent Stop() calls did not complete in 3s, counter=%d", atomic.LoadInt32(&counter))
	}

	if counter != N {
		t.Errorf("expected all %d Stop() calls to complete, got %d", N, counter)
	}
}
