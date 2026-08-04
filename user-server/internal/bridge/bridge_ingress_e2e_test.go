package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"marketing/internal/service"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
)

// e2eFakeTrigger 模拟 AI 客服：收到新消息后，把回复经 BridgeHub 通过 WebSocket 回写扩展
type e2eFakeTrigger struct {
	hub         *BridgeHub
	calls       int32 // atomic
	lastChannel string
	lastAccount string
	lastConv    string
	lastCust    string
	lastContent string
	lastEventID string
}

func (f *e2eFakeTrigger) TriggerInboundAI(ctx context.Context, channel, accountID, conversationID, customerID, content, eventID string, opts ...service.TriggerInboundOption) {
	atomic.AddInt32(&f.calls, 1)
	f.lastChannel = channel
	f.lastAccount = accountID
	f.lastConv = conversationID
	f.lastCust = customerID
	f.lastContent = content
	f.lastEventID = eventID
	reply := &UnifiedReply{
		Channel:        channel,
		AccountID:      accountID,
		ConversationID: conversationID,
		Content:        "AI自动回复:" + content,
		MsgType:        "text",
		ReplyToEventID: eventID,
	}
	_ = f.hub.Deliver(channel, accountID, reply)
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// TestBridgeHandler_HistoryFrameDoesNotTriggerAI 验证历史消息帧（history）只落库不触发 AI 路由。
//
// 独立测试以避免读 goroutine 超时关闭连接副作用与 inbound 测试耦合。
func TestBridgeHandler_HistoryFrameDoesNotTriggerAI(t *testing.T) {
	hub := NewBridgeHub()
	ingress := service.NewInboxIngressServiceWithDB(nil, nil)
	tr := &e2eFakeTrigger{hub: hub}
	ingress.SetAITrigger(tr)
	handler := NewBridgeWSHandler(hub, ingress)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/ws/bridge", handler.HandleWebSocket)
	srv := httptest.NewServer(engine)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/bridge?channel=douyin_web&account_id=acc1"
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("扩展建立 WebSocket 连接失败: %v", err)
	}
	defer conn.Close()

	histFrame := map[string]any{"type": FrameHistory, "message": map[string]any{
		"event_id":        "evt-hist-1",
		"channel":         "douyin_web",
		"account_id":      "acc1",
		"conversation_id": "conv1",
		"sender_id":       "cust1",
		"sender_type":     "customer",
		"content":         "历史消息A",
		"direction":       "inbound",
	}}
	if err := conn.WriteMessage(gorilla.TextMessage, mustJSON(histFrame)); err != nil {
		t.Fatalf("发送历史帧失败: %v", err)
	}
	// 给服务端 readPump 600ms 时间处理；AI 不应被触发
	time.Sleep(600 * time.Millisecond)
	if got := atomic.LoadInt32(&tr.calls); got != 0 {
		t.Fatalf("历史消息不应触发 AI；fakeTrigger.calls=%d (期望 0)", got)
	}
	t.Log("✅ 历史消息帧不触发 AI，符合 history/inbound 帧语义分离")
}

// TestBridgeHandler_InboundTriggersAI 验证核心端到端流程：
//  实时新消息（inbound_message 帧）→ 触发 AI → AI 回复经 WebSocket 下行到扩展
//
// 本测试**不依赖真实 Postgres**：service 用 nil DB（hubRepo 短路，persistMessage 为 no-op），
// 聚焦验证：
//   - WebSocket 协议层 inbound / outbound_reply 帧正确编解码
//   - 帧路由：inbound 走 HandleIngressMessage
//   - AI 触发器被调用且参数正确
//   - AI 回复经 BridgeHub 通过 WebSocket 原路回写到连接的对端
func TestBridgeHandler_InboundTriggersAI(t *testing.T) {
	hub := NewBridgeHub()
	// nil DB：hubRepo == nil 时 persistMessage 为 no-op 短路，
	// 避免依赖外部 PG；核心验证在 AI 触发 + WS 下行。
	ingress := service.NewInboxIngressServiceWithDB(nil, nil)
	tr := &e2eFakeTrigger{hub: hub}
	ingress.SetAITrigger(tr)
	handler := NewBridgeWSHandler(hub, ingress)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/ws/bridge", handler.HandleWebSocket)
	srv := httptest.NewServer(engine)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/bridge?channel=douyin_web&account_id=acc1"
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("扩展建立 WebSocket 连接失败: %v", err)
	}
	defer conn.Close()

	inFrame := map[string]any{"type": FrameInbound, "message": map[string]any{
		"event_id":        "evt-in-1",
		"channel":         "douyin_web",
		"account_id":      "acc1",
		"conversation_id": "conv1",
		"sender_id":       "cust1",
		"content":         "你好我要咨询",
	}}
	if err := conn.WriteMessage(gorilla.TextMessage, mustJSON(inFrame)); err != nil {
		t.Fatalf("发送入站帧失败: %v", err)
	}
	reply, err := readReplyWithin(conn, 3*time.Second)
	if err != nil {
		t.Fatalf("扩展未收到 AI 回复: %v", err)
	}
	if reply.Content != "AI自动回复:你好我要咨询" {
		t.Fatalf("AI 回复内容错误: %q", reply.Content)
	}
	if reply.ConversationID != "conv1" {
		t.Fatalf("AI 回复会话ID错误: %q", reply.ConversationID)
	}
	if reply.Channel != "douyin_web" {
		t.Fatalf("AI 回复渠道错误: %q", reply.Channel)
	}
	if reply.AccountID != "acc1" {
		t.Fatalf("AI 回复账号错误: %q", reply.AccountID)
	}
	if reply.ReplyToEventID != "evt-in-1" {
		t.Fatalf("AI 回复关联原 eventID 错误: %q", reply.ReplyToEventID)
	}
	if got := atomic.LoadInt32(&tr.calls); got != 1 {
		t.Fatalf("期望 fakeTrigger 被调用 1 次，实际 %d", got)
	}
	if tr.lastChannel != "douyin_web" || tr.lastAccount != "acc1" || tr.lastConv != "conv1" ||
		tr.lastCust != "cust1" || tr.lastContent != "你好我要咨询" || tr.lastEventID != "evt-in-1" {
		t.Fatalf("AI 触发参数错误: %+v", tr)
	}
	t.Logf("✅ 实时新消息触发 AI 并原路回写扩展: %s", reply.Content)
}

// readReplyWithin 在 conn 上持续读取帧并按 type 过滤，超时或读到目标帧时返回。
//
// 实现：使用 channel + select 控制超时，避免 conn.SetReadDeadline 反复重置 deadline 引起的
// gorilla/websocket 内部 timer 状态混乱（曾出现"立即 i/o timeout"问题）。每次调用启动一个
// 独立 goroutine 读 conn，由调用方在超时后通过 close 终止。
func readReplyWithin(conn *gorilla.Conn, timeout time.Duration) (*UnifiedReply, error) {
	type result struct {
		reply *UnifiedReply
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				ch <- result{nil, err}
				return
			}
			var f struct {
				Type  string       `json:"type"`
				Reply *UnifiedReply `json:"reply"`
			}
			if json.Unmarshal(raw, &f) != nil {
				continue
			}
			if f.Type == FrameOutboundReply && f.Reply != nil {
				ch <- result{f.Reply, nil}
				return
			}
		}
	}()
	select {
	case r := <-ch:
		return r.reply, r.err
	case <-time.After(timeout):
		// 超时：主动关闭连接，使读 goroutine 立即退出（i/o EOF），避免泄漏到下一个 readReplyWithin。
		_ = conn.Close()
		return nil, fmt.Errorf("read reply timeout after %s", timeout)
	}
}
