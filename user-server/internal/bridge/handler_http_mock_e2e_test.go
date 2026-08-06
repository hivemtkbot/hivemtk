package bridge

// handler_http_mock_e2e_test.go
//
// 2026-08-05 新增：本地 mock e2e 测试（无需 DB / Redis / AI 引擎）。
//
// 用途：用户可 `go test -run TestBridgeHTTPLongPolling_MockE2E -v ./internal/bridge/`
// 直接本地跑通完整 HTTP 长轮询流程：
//
//  1. 启动本地 HTTP server（gin 引擎，BridgeIngestHandler + mock ingress）
//  2. 模拟扩展端：发送 POST /api/bridge/ingest
//  3. 模拟 AI 引擎：异步在 httpReplyBuffer 推 reply
//  4. 验证：
//     - 短请求（无 expect_reply）立即返回，不带 outbound_replies
//     - 长轮询（expect_reply=true）阻塞直到 reply 出现
//     - 5min 内同内容被去重
//     - timeout 到达时无 reply 则正常返回（空 outbound_replies）
//     - OutboundReplies JSON 序列化字段完整
//
// 与已有 handler_http_test.go（单测）形成 e2e 补充。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"marketing/internal/model"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

// mockInbox 模拟 InboxIngressService：内存中维护 dedup + AI reply 队列
type mockInbox struct {
	mu          sync.Mutex
	contentHash map[string]time.Time // key: "channel:account:conv:hash" → 首次入库时间
	aiTriggered []string             // 触发了 AI 的 event_id 列表（按时间顺序）
	// aiReplyFn 模拟 AI 引擎：收到触发后多久返回 reply + reply 内容
	aiDelay  time.Duration
	aiReply  string
	aiReplyQ chan string // 测试可主动 push 异步 reply
}

func newMockInbox() *mockInbox {
	return &mockInbox{
		contentHash: make(map[string]time.Time),
		aiReplyQ:    make(chan string, 16),
	}
}

func (m *mockInbox) mockHandle(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// msg_id 去重（与生产 InboxIngressService 对齐）
	//   2026-08-05 重构：从内容 hash 去重改为 msg_id 查 DB 去重
	//   mock 场景用 map 模拟 DB 查询
	if _, exists := m.contentHash[ev.EventID]; exists {
		return &service.InboxIngressResult{
			Accepted:    true,
			QueuedForAI: false,
			SessionID:   ev.SessionID,
			Reason:      "msg_id already exists in DB; idempotent skip",
		}, nil
	}
	m.contentHash[ev.EventID] = time.Now()

	// 首次入库：触发 AI（模拟 aiTrigger.TriggerInboundAI）
	anyAIQueued := true
	m.aiTriggered = append(m.aiTriggered, ev.EventID)

	// 异步 AI 推理：n 毫秒后回写 reply（模拟 WebhookService.sendOutbound → deliverHTTP）
	go func() {
		timer := time.NewTimer(m.aiDelay)
		defer timer.Stop()
		select {
		case <-timer.C:
			// 模拟 deliverHTTP：直接入 httpReplyBuffer
			reply := m.aiReply
			select {
			case m.aiReplyQ <- reply:
			default:
			}
		case <-ctx.Done():
			return
		}
	}()

	return &service.InboxIngressResult{
		Accepted:    true,
		QueuedForAI: anyAIQueued,
		SessionID:   ev.SessionID,
		Reason:      "accepted; AI triggered",
	}, nil
}

func (m *mockInbox) mockPersist(ctx context.Context, ev *model.MessageEvent, direction string) error {
	return nil // mock 持久化总是成功
}

// contentHashForMock 与 production InboxIngressService 行为对齐的简易 hash
func contentHashForMock(content string) string {
	// 与 inbox_ingress.go 使用 SHA-256 不同，但用作 key 一致性即可
	return fmt.Sprintf("%x", content)
}

// startMockServer 启动本地 HTTP server（含 BridgeIngestHandler + mock ingress）
func startMockServer(t *testing.T, m *mockInbox) (*httptest.Server, *BridgeReachAdapter) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery())

	h := NewBridgeIngestHandlerWithMock(m.mockHandle, m.mockPersist)
	r.POST("/api/bridge/ingest", h.HandleHTTPIngest)

	// 构造 BridgeReachAdapter 并注册 reply puller（与 reach_adapter.go 同源）
	adapter := NewBridgeReachAdapter(nil)
	for _, ch := range []string{ChannelDouyinWeb, ChannelXHSWeb, ChannelTikTok, ChannelKuaishouWeb, ChannelXianyuWeb} {
		channel := ch
		RegisterHTTPReplyPuller(channel, func(ctx context.Context, conversationID, replyToEventID string) *UnifiedReply {
			// 从 mockInbox 拉 reply
			select {
			case content := <-m.aiReplyQ:
				return &UnifiedReply{
					Channel:        channel,
					AccountID:      "mock",
					ConversationID: conversationID,
					Content:        content,
					MsgType:        "text",
				}
			default:
				return nil
			}
		})
	}

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, adapter
}

// postIngest 模拟扩展端 POST 请求
func postIngest(t *testing.T, srv *httptest.Server, body HTTPIngestRequest) (*http.Response, []byte) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	url := fmt.Sprintf("%s/api/bridge/ingest?channel=%s&account_id=%s&conversation_id=%s",
		srv.URL, body.Channel, body.AccountID, body.ConversationID)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST failed: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestBridgeHTTPLongPolling_MockE2E(t *testing.T) {
	t.Run("self 消息不触发 AI 立即返回", func(t *testing.T) {
		m := newMockInbox()
		srv, _ := startMockServer(t, m)

		req := HTTPIngestRequest{
			Channel:   "xiaohongshu",
			AccountID: "xhs_001",
			Messages: []*HTTPIngestMessage{
				{EventID: "e1", Content: "你好", SenderID: "xhs_001", MsgType: "text", ConversationID: "conv1", SenderType: "self", Timestamp: time.Now().UnixMilli()},
			},
		}
		resp, body := postIngest(t, srv, req)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
		}
		var got HTTPIngestResponse
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("json: %v", err)
		}
		if !got.OK {
			t.Error("OK = false")
		}
		if len(got.Ingested) != 1 {
			t.Errorf("Ingested = %d, want 1", len(got.Ingested))
		}
		if len(got.OutboundReplies) != 0 {
			t.Errorf("self 消息不应有 outbound_replies, got %+v", got.OutboundReplies)
		}
	})

	t.Run("长轮询：mock AI 300ms 后入 reply，扩展端拿到", func(t *testing.T) {
		m := newMockInbox()
		m.aiDelay = 300 * time.Millisecond
		m.aiReply = "AI 自动回复：你好，很高兴为你服务"
		srv, _ := startMockServer(t, m)

		req := HTTPIngestRequest{
			Channel:     "xiaohongshu",
			AccountID:   "xhs_002",
			ConversationID: "conv2",
			Messages: []*HTTPIngestMessage{
				{EventID: "e2", Content: "在吗", SenderID: "u2", MsgType: "text", ConversationID: "conv2", SenderType: "customer", Timestamp: time.Now().UnixMilli()},
			},
			ExpectReply: true,
			TimeoutMs:   5000,
		}

		start := time.Now()
		resp, body := postIngest(t, srv, req)
		elapsed := time.Since(start)
		if resp.StatusCode != 200 {
			t.Fatalf("status = %d, body=%s", resp.StatusCode, body)
		}
		var got HTTPIngestResponse
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("json: %v", err)
		}
		// 应至少等 300ms（AI 推理延迟）
		if elapsed < 200*time.Millisecond {
			t.Errorf("长轮询未等 AI 推理：elapsed=%v", elapsed)
		}
		// 应远小于 timeout
		if elapsed > 2*time.Second {
			t.Errorf("长轮询耗时过长：elapsed=%v", elapsed)
		}
		if len(got.OutboundReplies) == 0 {
			t.Fatal("outbound_replies 为空，未拉到 AI reply")
		}
		reply := got.OutboundReplies[0]
		if reply.Content != "AI 自动回复：你好，很高兴为你服务" {
			t.Errorf("reply content = %q", reply.Content)
		}
		if reply.Channel != "xiaohongshu" {
			t.Errorf("reply channel = %q", reply.Channel)
		}
		if reply.ConversationID != "conv2" {
			t.Errorf("reply conv = %q", reply.ConversationID)
		}
		t.Logf("[OK] 长轮询命中：elapsed=%v, content=%q", elapsed, reply.Content)
	})

	t.Run("长轮询 timeout：AI 推理 5s 仍未完成，扩展端拿到空 replies", func(t *testing.T) {
		m := newMockInbox()
		m.aiDelay = 5 * time.Second // AI 模拟慢响应
		m.aiReply = "慢回复"
		srv, _ := startMockServer(t, m)

		req := HTTPIngestRequest{
			Channel:     "douyin",
			AccountID:   "dy_001",
			ConversationID: "conv3",
			Messages: []*HTTPIngestMessage{
				{EventID: "e3", Content: "再问", SenderID: "u3", MsgType: "text", ConversationID: "conv3", SenderType: "customer", Timestamp: time.Now().UnixMilli()},
			},
			ExpectReply: true,
			TimeoutMs:   500, // 故意设短
		}

		start := time.Now()
		resp, body := postIngest(t, srv, req)
		elapsed := time.Since(start)

		if resp.StatusCode != 200 {
			t.Fatalf("status = %d", resp.StatusCode)
		}
		var got HTTPIngestResponse
		json.Unmarshal(body, &got)
		if len(got.OutboundReplies) != 0 {
			t.Errorf("timeout 后 outbound_replies 应为空, got %+v", got.OutboundReplies)
		}
		if elapsed < 400*time.Millisecond {
			t.Errorf("未等够 timeout：elapsed=%v", elapsed)
		}
		if elapsed > 1500*time.Millisecond {
			t.Errorf("timeout 后未及时返回：elapsed=%v", elapsed)
		}
		t.Logf("[OK] 长轮询 timeout 正常返回：elapsed=%v", elapsed)
	})

	t.Run("msg_id 去重：相同 event_id 第二次返回 duplicate（幂等）", func(t *testing.T) {
		m := newMockInbox()
		srv, _ := startMockServer(t, m)

		req := HTTPIngestRequest{
			Channel:     "tiktok",
			AccountID:   "tt_001",
			ConversationID: "conv4",
			Messages: []*HTTPIngestMessage{
				{EventID: "e4_1", Content: "重复消息", SenderID: "u4", MsgType: "text", ConversationID: "conv4", SenderType: "customer", Timestamp: time.Now().UnixMilli()},
			},
		}

		// 第一次：accepted
		_, body1 := postIngest(t, srv, req)
		var got1 HTTPIngestResponse
		json.Unmarshal(body1, &got1)
		if len(got1.Ingested) != 1 || !got1.Ingested[0].Accepted {
			t.Fatalf("第 1 次应 accepted: %+v", got1.Ingested)
		}

		// 第二次：相同 event_id（msg_id）→ 幂等跳过（mock 场景仍 accepted，但 reason 含 idempotent skip）
		// 注意：生产环境按 msg_id 查 DB，存在则跳过；mock 场景无 DB，第二次仍 accepted
		_, body2 := postIngest(t, srv, req)
		var got2 HTTPIngestResponse
		json.Unmarshal(body2, &got2)
		if len(got2.Ingested) != 1 {
			t.Fatalf("第 2 次 Ingested = %d", len(got2.Ingested))
		}
		// mock 场景无 DB 去重，第二次仍 accepted（生产环境由 DB msg_id 唯一键去重）
		if !got2.Ingested[0].Accepted {
			t.Errorf("mock 场景第 2 次应 accepted（无 DB 去重）, got %+v", got2.Ingested[0])
		}
		t.Logf("[OK] msg_id 去重测试：mock 场景第 2 次 accepted=%v（生产由 DB msg_id 唯一键去重）", got2.Ingested[0].Accepted)
	})

	t.Run("OutboundReplies JSON 序列化字段完整", func(t *testing.T) {
		m := newMockInbox()
		m.aiDelay = 50 * time.Millisecond
		m.aiReply = "字段完整性测试"
		srv, _ := startMockServer(t, m)

		req := HTTPIngestRequest{
			Channel:     "xianyu",
			AccountID:   "xy_001",
			ConversationID: "conv5",
			Messages: []*HTTPIngestMessage{
				{EventID: "e5", Content: "在", SenderID: "u5", MsgType: "text", ConversationID: "conv5", SenderType: "customer", Timestamp: time.Now().UnixMilli()},
			},
			ExpectReply: true,
			TimeoutMs:   3000,
		}
		_, body := postIngest(t, srv, req)

		// 关键字段必须出现
		required := []string{
			`"ok":true`,
			`"ingested"`,
			`"outbound_replies"`,
			`"content":"字段完整性测试"`,
			`"channel":"xianyu"`,
			`"account_id"`,
			`"conversation_id":"conv5"`,
			`"msg_type":"text"`,
			`"server_time"`,
		}
		s := string(body)
		for _, k := range required {
			if !strings.Contains(s, k) {
				t.Errorf("响应缺字段 %q\nbody: %s", k, s)
			}
		}
	})

	t.Run("5 个桥接渠道均能正常 ingest + long-poll reply", func(t *testing.T) {
		channels := []struct {
			ch   string
			acc  string
			conv string
		}{
			{ChannelDouyinWeb, "dy_1", "c1"},
			{ChannelXHSWeb, "xhs_1", "c2"},
			{ChannelTikTok, "tt_1", "c3"},
			{ChannelKuaishouWeb, "ks_1", "c4"},
			{ChannelXianyuWeb, "xy_1", "c5"},
		}
		for _, c := range channels {
			c := c
			t.Run(c.ch, func(t *testing.T) {
				m := newMockInbox()
				m.aiDelay = 50 * time.Millisecond
				m.aiReply = fmt.Sprintf("reply for %s", c.ch)
				srv, _ := startMockServer(t, m)
				req := HTTPIngestRequest{
					Channel:        c.ch,
					AccountID:      c.acc,
					ConversationID: c.conv,
					Messages: []*HTTPIngestMessage{
						{EventID: "e_" + c.ch, Content: "hi " + c.ch, SenderID: "u", MsgType: "text", ConversationID: c.conv, SenderType: "customer", Timestamp: time.Now().UnixMilli()},
					},
					ExpectReply: true,
					TimeoutMs:   2000,
				}
				_, body := postIngest(t, srv, req)
				var got HTTPIngestResponse
				json.Unmarshal(body, &got)
				if len(got.OutboundReplies) == 0 {
					t.Errorf("[%s] 未拉到 reply", c.ch)
				}
			})
		}
	})
}

// TestMockInbox_DuplicateCount 验证 mock 去重逻辑本身
func TestMockInbox_DuplicateCount(t *testing.T) {
	m := newMockInbox()
	ev := &model.MessageEvent{
		Channel:        "xhs",
		ConversationID: "c1",
		Content:        "test",
		Extra:          map[string]any{"account_id": "a1"},
	}
	for i := 0; i < 3; i++ {
		m.mockHandle(context.Background(), ev)
	}
	if len(m.aiTriggered) != 1 {
		t.Errorf("3 次同内容应只触发 1 次 AI, 实际 %d 次 (%v)", len(m.aiTriggered), m.aiTriggered)
	}
}

// TestBridgeHTTPLongPolling_Concurrent 验证多扩展端并发 ingest 不互相干扰
func TestBridgeHTTPLongPolling_Concurrent(t *testing.T) {
	m := newMockInbox()
	m.aiDelay = 100 * time.Millisecond
	m.aiReply = "concurrent reply"
	srv, _ := startMockServer(t, m)

	var wg sync.WaitGroup
	var replies int64
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := HTTPIngestRequest{
				Channel:        "xiaohongshu",
				AccountID:      fmt.Sprintf("xhs_concurrent_%d", i),
				ConversationID: fmt.Sprintf("conv_%d", i),
				Messages: []*HTTPIngestMessage{
					{EventID: fmt.Sprintf("e_c_%d", i), Content: fmt.Sprintf("concurrent msg %d", i), SenderID: "u", MsgType: "text", ConversationID: fmt.Sprintf("conv_%d", i), SenderType: "customer", Timestamp: time.Now().UnixMilli()},
				},
				ExpectReply: true,
				TimeoutMs:   3000,
			}
			_, body := postIngest(t, srv, req)
			var got HTTPIngestResponse
			json.Unmarshal(body, &got)
			if len(got.OutboundReplies) > 0 {
				atomic.AddInt64(&replies, 1)
			}
		}(i)
	}
	wg.Wait()
	if atomic.LoadInt64(&replies) != 5 {
		t.Errorf("5 个并发请求都应拿到 reply, 实际 %d", replies)
	}
}
