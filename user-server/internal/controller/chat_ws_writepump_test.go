package controller

// chat_ws_writepump_test.go WebSocket writePump 超时测试
//
// 验证 writePump 内部 SetWriteDeadline 被正确设置:
//   - chatWSWriteWait 常量 = 10s (生产化要求)
//   - 所有 WriteMessage 调用前都设置写超时
//
// 测试策略:
//   - 使用 httptest + websocket.DefaultDialer 构造真实 client/server 连接对
//   - 启动 writePump, 注入 payload, 验证对端能收到
//   - 通过 timeout 参数验证 SetWriteDeadline 行为 (hung conn 应在 10s 内被踢掉)

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

// wsTestSetup 构造一对 WebSocket 连接 (server / client), 返回 clientConn 和 serverConn
//
// 服务端 upgrader 关闭 CheckOrigin (本地测试) 以避免和 白名单互相干扰。
// 返回的 (clientConn, serverConn, cleanup) 中:
//   - clientConn 由 writePump 写入
//   - serverConn 由测试代码读取验证
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

	// 创建 client (不依赖 conn.Close, writePump 自管)
	c := NewClient("s1", "c1", clientConn, "trace-1")
	ctx := context.Background()

	ctrl := &ChatWSController{}

	// 启动 writePump
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctrl.writePump(c, clientConn, ctx)
	}()

	// 注入 payload
	c.SendChan() <- []byte(`{"hello":"world"}`)

	// server 端应收到 (SetWriteDeadline 保证不被无限阻塞)
	_ = serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := serverConn.ReadMessage()
	if err != nil {
		t.Fatalf("server side read: %v", err)
	}
	if string(msg) != `{"hello":"world"}` {
		t.Errorf("expected payload to arrive, got %q", string(msg))
	}

	// 触发 close 让 writePump 退出
	c.Close()

	select {
	case <-done:
		// writePump 正常退出
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

	// 关闭 send channel (模拟 hub 关闭)
	c.Close()

	// server 端应收到 close 帧 (或 close error)
	_ = serverConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	mt, _, err := serverConn.ReadMessage()
	if err != nil {
		// gorilla 库可能直接返回 CloseError 而不是 CloseMessage, 也算正常
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
		// OK
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

	// 第一次写入: 成功 (对端 buffer 没满)
	c.SendChan() <- []byte("first")

	// 让 writePump 短暂 idle
	time.Sleep(50 * time.Millisecond)

	// 持续注入大量 payload, 让对端 TCP 接收缓冲最终填满, 迫使某次 WriteMessage 阻塞
	// 直至 chatWSWriteWait(10s) 触发 SetWriteDeadline 退出。
	// 注意: 必须持续(阻塞)喂数据, 不能写满 channel 就退出——否则 writePump 会很快把
	// 有限 payload 消费完并回到 select 空等, 永远触发不了写超时 (macOS 内核收缓冲可 auto-tune 到数 MB,
	// 仅往 64 槽缓冲塞几十个 8KB 块远不足以填满, WriteMessage 不会阻塞)。
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
			case c.SendChan() <- make([]byte, 32*1024):
				atomic.AddInt32(&writeCount, 1)
			}
		}
	}()

	// 给 writePump 10s, 期间应触达 deadline 退出
	select {
	case <-done:
		t.Logf("writePump exited (good - means SetWriteDeadline triggered)")
	case <-time.After(chatWSWriteWait + 2*time.Second):
		// 15s 还没退出, 说明 SetWriteDeadline 没生效
		close(stop)
		wg.Wait()
		c.Close()
		t.Error("writePump did not exit after chatWSWriteWait + 2s; SetWriteDeadline may not be effective")
		return // 直接返回避免下面的二次 close
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
