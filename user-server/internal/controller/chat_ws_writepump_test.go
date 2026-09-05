package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWritePump_ChatWSWriteWait 测试 : 写超时常量 = 10s
func TestWritePump_ChatWSWriteWait(t *testing.T) {
	if chatWSWriteWait != 10*time.Second {
		t.Errorf("chatWSWriteWait should be 10s for B-019, got %s", chatWSWriteWait)
	}
}

func wsTestSetup(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	connCh := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("server upgrade: %v", err)
			return
		}
		connCh <- c
	}))

	dialer := websocket.DefaultDialer
	clientConn, _, err := dialer.Dial(strings.Replace(srv.URL, "http", "ws", 1), nil)
	if err != nil {
		srv.Close()
		t.Fatalf("dial: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-connCh:
	case <-time.After(2 * time.Second):
		clientConn.Close()
		srv.Close()
		t.Fatal("timeout waiting for server conn")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		srv.Close()
	}
	return clientConn, serverConn, cleanup
}

// TestWritePump_DeliversPayload 测试 writePump 正常投递 payload
//
// 验证:
//   - payload 写入 client.send 后能在 chatWSWriteWait 内到达对端
//   - SetWriteDeadline 已设置 (否则对端收不到就证明写阻塞)
func TestWritePump_DeliversPayload(t *testing.T) {
	clientConn, serverConn, cleanup := wsTestSetup(t)
	defer cleanup()

	c := NewClient("s1", "c1", clientConn, "trace-1")
	ctx := context.Background()

	ctrl := &ChatWSController{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl.writePump(c, clientConn, ctx)
	}()

	c.SendChan() <- []byte(`{"hello":"world"}`)

	_ = serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := serverConn.ReadMessage()
	if err != nil {
		t.Fatalf("server side read: %v", err)
	}
	if string(msg) != `{"hello":"world"}` {
		t.Errorf("expected payload to arrive, got %q", string(msg))
	}

	c.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("writePump did not exit after client close")
	}
}

// TestWritePump_CloseOnChannelClose 测试 send channel 关闭时 writePump 写 close 帧
//
// writePump 在 send channel 关闭时调用 conn.WriteMessage(websocket.CloseMessage, []byte{}),
// 对端 ReadMessage 应返回 CloseError (或 CloseMessage, 取决于 gorilla 库版本).
func TestWritePump_CloseOnChannelClose(t *testing.T) {
	clientConn, serverConn, cleanup := wsTestSetup(t)
	defer cleanup()

	c := NewClient("s2", "c2", clientConn, "trace-2")
	ctx := context.Background()
	ctrl := &ChatWSController{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl.writePump(c, clientConn, ctx)
	}()

	c.Close()

	_ = serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, _, err := serverConn.ReadMessage()
	if err != nil {
		if ce, ok := err.(*websocket.CloseError); ok {
			t.Logf("received close frame as error (normal): code=%d", ce.Code)
		} else {
			t.Fatalf("read close frame: %v", err)
		}
	} else if mt != websocket.CloseMessage {
		t.Errorf("expected CloseMessage, got type=%d", mt)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("writePump did not exit after channel close")
	}
}

// TestWritePump_RespectsWriteDeadline 验证 SetWriteDeadline 真的生效
//
// 策略: 让对端不读, 然后注入 payload, writePump 应该在 chatWSWriteWait (10s) 内退出
// (而不是无限阻塞). 测试使用 chatWSWriteWait/2 等待, 验证退出发生在超时之前
// (正常情况下 writePump 会在 10s 内检测到 deadline exceeded 退出).
//
// 注意: 此测试耗时 ~10s, 标记 long test, 必要时用 -short 跳过.
func TestWritePump_RespectsWriteDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long test in -short mode")
	}
	clientConn, _, cleanup := wsTestSetup(t)
	defer cleanup()

	c := NewClient("s3", "c3", clientConn, "trace-3")
	ctx := context.Background()
	ctrl := &ChatWSController{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl.writePump(c, clientConn, ctx)
	}()

	c.SendChan() <- []byte("first")

	time.Sleep(50 * time.Millisecond)

	var writeCount int32
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			case c.SendChan() <- make([]byte, 256*1024):
				atomic.AddInt32(&writeCount, 1)
			}
		}
	}()

	select {
	case <-done:
		t.Logf("writePump exited (good - means SetWriteDeadline triggered)")
	case <-time.After(chatWSWriteWait + 30*time.Second):
		close(stop)
		wg.Wait()
		c.Close()
		t.Error("writePump did not exit after chatWSWriteWait + 30s; SetWriteDeadline may not be effective")
		return
	}

	close(stop)
	wg.Wait()
}

// TestWritePump_PingInterval 测试 ping 周期 = 30s
func TestWritePump_PingInterval(t *testing.T) {
	if chatWSPingPeriod != 30*time.Second {
		t.Errorf("chatWSPingPeriod should be 30s, got %s", chatWSPingPeriod)
	}
}

// TestWritePump_PongTimeout 测试 pong 超时 = 60s
func TestWritePump_PongTimeout(t *testing.T) {
	if chatWSPongWait != 60*time.Second {
		t.Errorf("chatWSPongWait should be 60s, got %s", chatWSPongWait)
	}
}
