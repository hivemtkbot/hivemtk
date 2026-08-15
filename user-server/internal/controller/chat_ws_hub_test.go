package controller


import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hivemtk-user/internal/dto"
)

// newTestClient 创建测试用 Client (不依赖真实 websocket 连接)
func newTestClient(sessionID, customerID string) *Client {
	return NewClient(sessionID, customerID, nil, "trace-"+sessionID)
}

// drainSendChan 异步消费 client.send 通道, 避免测试时通道满阻塞
//
// 启动一个 goroutine 持续读取 client.send 直到 channel 被 Close 关闭。
// 返回一个 chan []byte 接收所有已发送的 payload (用于断言)。
func drainSendChan(c *Client) <-chan []byte {
	out := make(chan []byte, 1024)
	go func() {
		defer close(out)
		for payload := range c.send {
			out <- payload
		}
	}()
	return out
}

// drainSendChanN 消费 N 条消息后返回 (用于精准断言)
func drainSendChanN(c *Client, n int) [][]byte {
	out := make([][]byte, 0, n)
	for i := 0; i < n; i++ {
		select {
		case payload, ok := <-c.send:
			if !ok {
				return out
			}
			out = append(out, payload)
		case <-time.After(2 * time.Second):
			return out
		}
	}
	return out
}

// TestHub_NewChatWSHub 验证 Hub 初始状态
func TestHub_NewChatWSHub(t *testing.T) {
	hub := NewChatWSHub()
	if hub == nil {
		t.Fatal("expected non-nil hub")
	}
	if got := hub.ClientCount(); got != 0 {
		t.Errorf("expected empty hub, got %d clients", got)
	}
	if hub.IsOnline("any-id") {
		t.Error("expected IsOnline=false for empty hub")
	}
}

// TestHub_RegisterUnregister 验证注册和注销流程
func TestHub_RegisterUnregister(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	defer func() {
	}()

	if got := hub.ClientCount(); got != 0 {
		t.Errorf("expected empty hub, got %d clients", got)
	}

	c1 := newTestClient("s1", "c1")
	hub.Register(c1)
	time.Sleep(50 * time.Millisecond)
	if got := hub.ClientCount(); got != 1 {
		t.Errorf("expected 1 client after Register, got %d", got)
	}
	if !hub.IsOnline("s1") {
		t.Error("expected s1 to be online after Register")
	}

	c2 := newTestClient("s2", "c2")
	hub.Register(c2)
	time.Sleep(50 * time.Millisecond)
	if got := hub.ClientCount(); got != 2 {
		t.Errorf("expected 2 clients, got %d", got)
	}

	hub.Unregister(c1)
	time.Sleep(50 * time.Millisecond)
	if got := hub.ClientCount(); got != 1 {
		t.Errorf("expected 1 client after Unregister, got %d", got)
	}
	if hub.IsOnline("s1") {
		t.Error("expected s1 to be offline after Unregister")
	}
	if !hub.IsOnline("s2") {
		t.Error("expected s2 still online")
	}

	hub.Unregister(c2)
	time.Sleep(50 * time.Millisecond)
	if got := hub.ClientCount(); got != 0 {
		t.Errorf("expected 0 clients, got %d", got)
	}
}

// TestHub_Unregister_NilClient 测试 nil client 安全
func TestHub_Unregister_NilClient(t *testing.T) {
	hub := NewChatWSHub()
	hub.Unregister(nil)
}

// TestHub_SendToClient 验证向指定 sessionID 发送
func TestHub_SendToClient(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	c1 := newTestClient("s1", "c1")
	c2 := newTestClient("s2", "c2")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(50 * time.Millisecond)

	drain1 := drainSendChan(c1)
	drain2 := drainSendChan(c2)

	if !hub.SendToClient("s1", []byte("hello s1")) {
		t.Error("expected SendToClient to succeed for s1")
	}
	select {
	case payload := <-drain1:
		if string(payload) != "hello s1" {
			t.Errorf("expected 'hello s1', got %q", string(payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for s1 to receive")
	}

	select {
	case payload := <-drain2:
		t.Errorf("expected s2 not receive, got %q", string(payload))
	case <-time.After(100 * time.Millisecond):
	}

	if hub.SendToClient("nonexistent", []byte("nobody")) {
		t.Error("expected SendToClient to return false for nonexistent sessionID")
	}
}

// TestHub_SendChunk 验证发送 StreamChunk
func TestHub_SendChunk(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	c1 := newTestClient("s1", "c1")
	hub.Register(c1)
	time.Sleep(50 * time.Millisecond)
	drain := drainSendChan(c1)

	chunk := &dto.StreamChunk{
		Type:    dto.ChunkTypeDelta,
		Text:    "hello",
		TraceID: "trace-1",
	}
	if !hub.SendChunk("s1", chunk) {
		t.Error("expected SendChunk to succeed")
	}
	select {
	case payload := <-drain:
		if len(payload) == 0 {
			t.Error("expected non-empty payload")
		}
		if !contains(string(payload), "hello") {
			t.Errorf("expected payload to contain 'hello', got %q", string(payload))
		}
		if !contains(string(payload), "delta") {
			t.Errorf("expected payload to contain 'delta', got %q", string(payload))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for chunk")
	}

	if hub.SendChunk("s1", nil) {
		t.Error("expected SendChunk(nil) to return false")
	}

	if hub.SendChunk("nonexistent", chunk) {
		t.Error("expected SendChunk to nonexistent to return false")
	}
}

// TestHub_Broadcast 验证广播
func TestHub_Broadcast(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	c1 := newTestClient("s1", "c1")
	c2 := newTestClient("s2", "c2")
	c3 := newTestClient("s3", "c3")
	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)
	time.Sleep(50 * time.Millisecond)

	drain1 := drainSendChan(c1)
	drain2 := drainSendChan(c2)
	drain3 := drainSendChan(c3)

	hub.Broadcast([]byte("broadcast_msg"))
	time.Sleep(50 * time.Millisecond)

	for i, drain := range []<-chan []byte{drain1, drain2, drain3} {
		select {
		case payload := <-drain:
			if string(payload) != "broadcast_msg" {
				t.Errorf("client %d: expected 'broadcast_msg', got %q", i+1, string(payload))
			}
		case <-time.After(2 * time.Second):
			t.Errorf("client %d: timeout waiting for broadcast", i+1)
		}
	}
}

// TestHub_BroadcastChunk 验证广播 StreamChunk
func TestHub_BroadcastChunk(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	c1 := newTestClient("s1", "c1")
	c2 := newTestClient("s2", "c2")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(50 * time.Millisecond)
	drain1 := drainSendChan(c1)
	drain2 := drainSendChan(c2)

	chunk := &dto.StreamChunk{Type: dto.ChunkTypeFinal, Text: "bye"}
	if !hub.BroadcastChunk(chunk) {
		t.Error("expected BroadcastChunk to return true")
	}
	time.Sleep(50 * time.Millisecond)

	for i, drain := range []<-chan []byte{drain1, drain2} {
		select {
		case payload := <-drain:
			if !contains(string(payload), "bye") {
				t.Errorf("client %d: expected payload to contain 'bye', got %q", i+1, string(payload))
			}
		case <-time.After(2 * time.Second):
			t.Errorf("client %d: timeout", i+1)
		}
	}

	if hub.BroadcastChunk(nil) {
		t.Error("expected BroadcastChunk(nil) to return false")
	}
}

// TestHub_ClientCount 验证客户端计数
func TestHub_ClientCount(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 initial, got %d", hub.ClientCount())
	}

	for i := 0; i < 5; i++ {
		c := newTestClient(string(rune('a'+i)), "c")
		hub.Register(c)
	}
	time.Sleep(80 * time.Millisecond)
	if got := hub.ClientCount(); got != 5 {
		t.Errorf("expected 5 clients, got %d", got)
	}
}

// TestHub_IsOnline 验证在线状态查询
func TestHub_IsOnline(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	if hub.IsOnline("s1") {
		t.Error("expected s1 to be offline initially")
	}

	c1 := newTestClient("s1", "c1")
	hub.Register(c1)
	time.Sleep(50 * time.Millisecond)

	if !hub.IsOnline("s1") {
		t.Error("expected s1 to be online after Register")
	}
	if hub.IsOnline("s2") {
		t.Error("expected s2 to be offline (never registered)")
	}

	hub.Unregister(c1)
	time.Sleep(50 * time.Millisecond)
	if hub.IsOnline("s1") {
		t.Error("expected s1 to be offline after Unregister")
	}
}

// TestHub_RegisterNoRun 测试 Run 未启动时, register 通道满后走 addClient 回退
func TestHub_RegisterNoRun(t *testing.T) {
	hub := NewChatWSHub()

	for i := 0; i < 17; i++ {
		c := newTestClient(string(rune('a'+i)), "c")
		hub.Register(c)
	}
	time.Sleep(20 * time.Millisecond)

	count := hub.ClientCount()
	if count < 1 {
		t.Errorf("expected at least 1 client (direct add path), got %d", count)
	}
	if count > 17 {
		t.Errorf("expected at most 17 clients, got %d", count)
	}
}

// TestClient_Close 验证 Client.Close 幂等
func TestClient_Close(t *testing.T) {
	c := newTestClient("s1", "c1")

	c.Close()
	c.Close()
	c.Close()

	select {
	case _, ok := <-c.send:
		if ok {
			t.Error("expected send channel to be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected send channel to be closed and readable")
	}
}

// TestClient_TraceID 验证 TraceID getter
func TestClient_TraceID(t *testing.T) {
	c := newTestClient("s1", "c1")
	if got := c.TraceID(); got != "trace-s1" {
		t.Errorf("expected trace-s1, got %s", got)
	}
}

// TestClient_SendChanRecvChan 验证 Send/Recv chan
func TestClient_SendChanRecvChan(t *testing.T) {
	c := newTestClient("s1", "c1")

	if c.SendChan() == nil {
		t.Error("expected non-nil SendChan")
	}
	if c.RecvChan() == nil {
		t.Error("expected non-nil RecvChan")
	}

	c.SendChan() <- []byte("test")
	select {
	case payload := <-c.RecvChan():
		if string(payload) != "test" {
			t.Errorf("expected 'test', got %q", string(payload))
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timeout")
	}
}

// TestHub_ConcurrentRegister 测试并发注册安全性
func TestHub_ConcurrentRegister(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			sessionID := string(rune('A'+(idx%26))) + string(rune('0'+(idx/26)))
			c := newTestClient("sess-"+sessionID, "c")
			hub.Register(c)
		}(i)
	}
	wg.Wait()
	time.Sleep(150 * time.Millisecond)

	count := hub.ClientCount()
	if count < 1 {
		t.Errorf("expected at least 1 client, got %d", count)
	}
	if count > N {
		t.Errorf("expected at most %d clients, got %d", N, count)
	}
}

// TestHub_SendToClient_FullChannel 测试 send 通道满时返回 false
func TestHub_SendToClient_FullChannel(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	c1 := newTestClient("s1", "c1")
	hub.Register(c1)
	time.Sleep(50 * time.Millisecond)

	failCount := int32(0)
	var wg sync.WaitGroup
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			if !hub.SendToClient("s1", []byte("x")) {
				atomic.AddInt32(&failCount, 1)
			}
		}()
	}
	wg.Wait()

	if failCount == 0 {
		t.Error("expected at least 1 failed send when channel is full")
	}

	go func() {
		for range c1.send {
		}
	}()
}

// TestHub_Broadcast_NoClient 测试空 Hub 广播
func TestHub_Broadcast_NoClient(t *testing.T) {
	hub := NewChatWSHub()
	go hub.Run()
	time.Sleep(20 * time.Millisecond)

	hub.Broadcast([]byte("nobody"))
	time.Sleep(20 * time.Millisecond)
	if hub.ClientCount() != 0 {
		t.Errorf("expected 0 clients, got %d", hub.ClientCount())
	}
}


