package bridge

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(channel, account string) *BridgeClient {
	return &BridgeClient{channel: channel, account: account, send: make(chan []byte, 16)}
}

func TestBridgeHub_RegisterOnlineDeliver(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	if old := hub.Register(c); old != nil {
		t.Fatal("unexpected old client")
	}
	if !hub.IsOnline(ChannelDouyinWeb, "acc1") {
		t.Fatal("expected online")
	}
	reply := &UnifiedReply{Channel: ChannelDouyinWeb, AccountID: "acc1", ConversationID: "conv1", Content: "hi"}
	if err := hub.Deliver(ChannelDouyinWeb, "acc1", reply); err != nil {
		t.Fatalf("deliver err: %v", err)
	}
	select {
	case msg := <-c.send:
		if len(msg) == 0 {
			t.Fatal("empty delivered message")
		}
	default:
		t.Fatal("expected a delivered message")
	}
}

func TestBridgeHub_OfflineDeliver(t *testing.T) {
	hub := NewBridgeHub()
	err := hub.Deliver(ChannelXHSWeb, "accX", &UnifiedReply{})
	if err != ErrBridgeOffline {
		t.Fatalf("expected ErrBridgeOffline, got %v", err)
	}
}

func TestBridgeHub_RegisterKickOld(t *testing.T) {
	hub := NewBridgeHub()
	oldC := newTestClient(ChannelTikTokWeb, "acc2")
	hub.Register(oldC)
	newC := newTestClient(ChannelTikTokWeb, "acc2")
	if ret := hub.Register(newC); ret != oldC {
		t.Fatal("expected old client returned for kick")
	}
	if !hub.IsOnline(ChannelTikTokWeb, "acc2") {
		t.Fatal("new client should be online")
	}
	hub.Unregister(newC)
	if hub.IsOnline(ChannelTikTokWeb, "acc2") {
		t.Fatal("should be offline after unregister")
	}
}

// TestBridgeHub_NextSeq_Monotonic 验证 seq 序号单调递增（用于下行帧排序/去重）
func TestBridgeHub_NextSeq_Monotonic(t *testing.T) {
	hub := NewBridgeHub()
	prev := hub.NextSeq()
	for i := 0; i < 100; i++ {
		cur := hub.NextSeq()
		if cur <= prev {
			t.Fatalf("seq not monotonic: prev=%d cur=%d", prev, cur)
		}
		prev = cur
	}
}

// TestBridgeHub_Shutdown_ClosesAll 验证 Shutdown 关闭所有活跃连接 + 幂等调用
func TestBridgeHub_Shutdown_ClosesAll(t *testing.T) {
	hub := NewBridgeHub()
	c1 := newTestClient(ChannelDouyinWeb, "acc1")
	c2 := newTestClient(ChannelXHSWeb, "acc2")
	hub.Register(c1)
	hub.Register(c2)

	hub.Shutdown()
	// 第一次 Shutdown 后所有连接应离线
	if hub.IsOnline(ChannelDouyinWeb, "acc1") {
		t.Fatal("acc1 应该在 Shutdown 后离线")
	}
	if hub.IsOnline(ChannelXHSWeb, "acc2") {
		t.Fatal("acc2 应该在 Shutdown 后离线")
	}
	// 再次调用应安全（幂等，不 panic）
	hub.Shutdown()
}

// TestBridgeHub_RateLimit_Bucket 验证超限返回 ErrBridgeRateLimited
func TestBridgeHub_RateLimit_Bucket(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)

	// 第一次：成功（60/min 容量内）
	if err := hub.Deliver(ChannelDouyinWeb, "acc1", &UnifiedReply{Content: "m1"}); err != nil {
		t.Fatalf("first deliver err: %v", err)
	}
	// 排空桶：连续取令牌直到触发限速
	var sawLimit bool
	for i := 0; i < 200; i++ {
		if err := hub.Deliver(ChannelDouyinWeb, "acc1", &UnifiedReply{Content: "x"}); err != nil {
			if err == ErrBridgeRateLimited {
				sawLimit = true
				break
			}
		}
	}
	if !sawLimit {
		t.Fatal("expected ErrBridgeRateLimited after bucket drained")
	}
}

// TestBridgeClient_replyPong 验证服务端对 JSON pong 帧回一帧 JSON pong。
// 修复背景：客户端 bridge-client.js 的 alive 标志只在收到「下行 JSON 帧」时重置，
// 若服务端不回，客户端每 ~serverIdleTimeoutMs 误判离线强制重连（50s 抖动）。
func TestBridgeClient_replyPong(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	c.hub = hub
	hub.Register(c)

	c.replyPong()

	select {
	case msg := <-c.send:
		var f Frame
		if err := json.Unmarshal(msg, &f); err != nil {
			t.Fatalf("replyPong 帧无法解析: %v", err)
		}
		if f.Type != FramePong {
			t.Fatalf("replyPong 应回 FramePong，实际 %q", f.Type)
		}
	default:
		t.Fatal("replyPong 未向 send 通道写入 JSON pong 帧")
	}
}

// TestBridgeClient_replyPong_AfterClose 验证已关闭连接回 pong 不 panic（幂等安全）
func TestBridgeClient_replyPong_AfterClose(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	c.hub = hub
	hub.Register(c)
	hub.Unregister(c) // CloseSend
	// 已关闭后调用必须安全（select default 不写已关闭 channel）
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("replyPong 在已关闭连接上 panic: %v", r)
			}
		}()
		c.replyPong()
	}()
}

// TestBridgeHub_Unregister_ClosesSendChannel 验证 Unregister 关闭 send channel（防 close-after-send panic）
func TestBridgeHub_Unregister_ClosesSendChannel(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)

	if c.IsClosed() {
		t.Fatal("new client should not be closed")
	}
	hub.Unregister(c)
	if !c.IsClosed() {
		t.Fatal("Unregister should mark client as closed")
	}
	// 已关闭的 send channel 第二次 Unregister 仍然安全（幂等）
	hub.Unregister(c)
}

// TestBridgeHub_Concurrent_Deliver 验证高并发 Deliver 不 panic 且不丢消息
func TestBridgeHub_Concurrent_Deliver(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)

	const N = 200
	var ok, failed atomic.Int32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if err := hub.Deliver(ChannelDouyinWeb, "acc1", &UnifiedReply{Content: "x"}); err == nil {
				ok.Add(1)
			} else {
				failed.Add(1)
			}
		}()
	}
	wg.Wait()
	t.Logf("concurrent deliver: ok=%d failed=%d", ok.Load(), failed.Load())
	// 60/min 容量应允许大部分通过，剩余触限；二者之和应等于 N
	if int(ok.Load()+failed.Load()) != N {
		t.Fatalf("expected %d total, got ok=%d failed=%d", N, ok.Load(), failed.Load())
	}
}

// TestBridgeHub_Janitor_CleansIdle 验证 janitor 清理空闲 rateBucket
func TestBridgeHub_Janitor_CleansIdle(t *testing.T) {
	hub := NewBridgeHub()
	c := newTestClient(ChannelDouyinWeb, "acc1")
	hub.Register(c)
	// 触发桶创建
	_ = hub.Deliver(ChannelDouyinWeb, "acc1", &UnifiedReply{Content: "m"})

	hub.rateMu.Lock()
	if len(hub.rateBuckets) == 0 {
		hub.rateMu.Unlock()
		t.Fatal("rate bucket should exist after Deliver")
	}
	hub.rateMu.Unlock()

	// 强制让桶标记为已空闲（直接修改 lastHit）
	hub.rateMu.Lock()
	for _, b := range hub.rateBuckets {
		b.mu.Lock()
		b.lastHit = time.Now().Add(-1 * time.Hour)
		b.mu.Unlock()
	}
	hub.rateMu.Unlock()

	// 启动 janitor，缩短间隔和空闲阈值，便于快速验证
	hub.StartJanitorWith(20*time.Millisecond, 100*time.Millisecond)
	// 等待 janitor 跑一轮
	time.Sleep(120 * time.Millisecond)
	hub.Shutdown() // 触发 janitor 退出

	hub.rateMu.Lock()
	remaining := len(hub.rateBuckets)
	hub.rateMu.Unlock()
	if remaining != 0 {
		t.Fatalf("janitor should have cleaned idle buckets, remaining=%d", remaining)
	}
}
