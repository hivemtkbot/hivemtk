// Command bridge-mock 启动一个本地 mock HTTP server，演示 bridge HTTP 长轮询流程。
//
// 用法：
//
//	cd hivemtk/user-server
//	go run ./cmd/bridge-mock
//
// 然后用 curl 模拟扩展端请求：
//
//	# 1. 短请求（无 expect_reply）：立即返回，无 outbound_replies
//	curl -X POST 'http://localhost:18080/api/bridge/ingest?channel=xiaohongshu&account_id=xhs_001&conversation_id=conv1' \
//	  -H 'Content-Type: application/json' \
//	  -d '{"channel":"xiaohongshu","account_id":"xhs_001","messages":[{"event_id":"e1","content":"你好","sender_id":"u1","sender_type":"customer","msg_type":"text","conversation_id":"conv1","timestamp":1700000000000}]}'
//
//	# 2. 长轮询：AI 模拟 1.5s 后入 reply，扩展端阻塞到 reply 出现
//	curl -X POST 'http://localhost:18080/api/bridge/ingest?channel=xiaohongshu&account_id=xhs_002&conversation_id=conv2' \
//	  -H 'Content-Type: application/json' \
//	  -d '{"channel":"xiaohongshu","account_id":"xhs_002","messages":[{"event_id":"e2","content":"在吗","sender_id":"u2","sender_type":"customer","msg_type":"text","conversation_id":"conv2","timestamp":1700000000000}],"expect_reply":true,"timeout_ms":5000}'
//
//	# 3. 5min 内容去重：相同 content 第二次返回 duplicate
//	curl -X POST 'http://localhost:18080/api/bridge/ingest?channel=tiktok&account_id=tt_001&conversation_id=conv4' \
//	  -H 'Content-Type: application/json' \
//	  -d '{"channel":"tiktok","account_id":"tt_001","messages":[{"event_id":"e_dup","content":"重复消息","sender_id":"u4","sender_type":"customer","msg_type":"text","conversation_id":"conv4","timestamp":1700000000000}]}'
//
// 该程序不依赖 DB / Redis / AI 引擎，全部用 in-memory mock；适合本地快速验证 HTTP 长轮询行为。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"marketing/internal/bridge"
	"marketing/internal/model"
	"marketing/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	defaultPort         = "18080"
	defaultAIDelayMs    = 1500 // mock AI 推理延迟
	mockAIDefaultReply  = "AI 自动回复：你好，很高兴为你服务 :)"
	mockAccountIDPrefix = "mock"
)

// mockInbox 内存版 InboxIngressService：
//   - 5min 内容 hash 去重
//   - 触发 AI 后异步 n 毫秒入 reply queue
//   - 不依赖任何外部服务
type mockInbox struct {
	mu          sync.Mutex
	contentHash map[string]time.Time
	aiTriggered []string
	aiDelay     time.Duration
	aiReply     string
	aiReplyQ    chan mockReply
}

type mockReply struct {
	Channel        string
	AccountID      string
	ConversationID string
	Content        string
	ReplyToEventID string
}

func newMockInbox(aiDelay time.Duration, aiReply string) *mockInbox {
	return &mockInbox{
		contentHash: make(map[string]time.Time),
		aiDelay:     aiDelay,
		aiReply:     aiReply,
		aiReplyQ:    make(chan mockReply, 64),
	}
}

func (m *mockInbox) mockHandle(ctx context.Context, ev *model.MessageEvent) (*service.InboxIngressResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	hash := simpleHash(ev.Content)
	accountID, _ := ev.Extra["account_id"].(string)
	key := ev.Channel + ":" + accountID + ":" + ev.ConversationID + ":" + hash
	if last, ok := m.contentHash[key]; ok && time.Since(last) < 5*time.Minute {
		log.Printf("[mock] duplicate drop: ch=%s acc=%s conv=%s content=%q",
			ev.Channel, accountID, ev.ConversationID, ev.Content)
		return &service.InboxIngressResult{
			Accepted: false, QueuedForAI: false,
			SessionID: ev.SessionID, Reason: "duplicate content within 5min; dropped",
		}, nil
	}
	m.contentHash[key] = time.Now()
	m.aiTriggered = append(m.aiTriggered, ev.EventID)
	log.Printf("[mock] accepted + AI triggered: event=%s ch=%s acc=%s conv=%s content=%q (reply in %v)",
		ev.EventID, ev.Channel, accountID, ev.ConversationID, ev.Content, m.aiDelay)

	// 异步模拟 AI 推理 + deliverHTTP
	go func(channel, acc, conv, eventID, reply string) {
		select {
		case <-time.After(m.aiDelay):
			m.aiReplyQ <- mockReply{
				Channel:        channel,
				AccountID:      acc,
				ConversationID: conv,
				Content:        reply,
				ReplyToEventID: eventID,
			}
			log.Printf("[mock] AI reply pushed to buffer: event=%s content=%q", eventID, reply)
		case <-ctx.Done():
		}
	}(ev.Channel, accountID, ev.ConversationID, ev.EventID, m.aiReply)

	return &service.InboxIngressResult{
		Accepted: true, QueuedForAI: true,
		SessionID: ev.SessionID, Reason: "accepted; AI triggered",
	}, nil
}

func (m *mockInbox) mockPersist(_ context.Context, _ *model.MessageEvent, _ string) error {
	return nil
}

func simpleHash(s string) string {
	// 简易 hash（仅用于 mock key，不参与生产）
	var sum uint64 = 1469598103934665603
	for _, c := range []byte(s) {
		sum ^= uint64(c)
		sum *= 1099511628211
	}
	return fmt.Sprintf("%016x", sum)
}

func main() {
	port := os.Getenv("MOCK_PORT")
	if port == "" {
		port = defaultPort
	}
	aiDelay := defaultAIDelayMs * time.Millisecond
	if v := os.Getenv("MOCK_AI_DELAY_MS"); v != "" {
		var d int
		fmt.Sscanf(v, "%d", &d)
		if d > 0 {
			aiDelay = time.Duration(d) * time.Millisecond
		}
	}
	aiReply := mockAIDefaultReply
	if v := os.Getenv("MOCK_AI_REPLY"); v != "" {
		aiReply = v
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(func(c *gin.Context) {
		// 入口日志：完整 URL + 全部 query + body 预览（与生产 handler 对齐）
		body, _ := readBodyPreview(c.Request.Body, 4096)
		c.Request.Body = readCloser{strings.NewReader(body)}
		log.Printf("[MOCK-INGEST] POST %s?%s body=%s", c.Request.URL.Path, c.Request.URL.RawQuery, body)
		c.Next()
	})

	m := newMockInbox(aiDelay, aiReply)
	h := bridge.NewBridgeIngestHandlerWithMock(m.mockHandle, m.mockPersist)
	r.POST("/api/bridge/ingest", h.HandleHTTPIngest)

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "ai_delay_ms": aiDelay.Milliseconds()})
	})
	// 打印当前 mock 状态
	r.GET("/mock/state", func(c *gin.Context) {
		m.mu.Lock()
		defer m.mu.Unlock()
		hashes := make(map[string]string)
		for k, t := range m.contentHash {
			hashes[k] = t.Format(time.RFC3339)
		}
		c.JSON(http.StatusOK, gin.H{
			"ai_triggered": m.aiTriggered,
			"content_hash": hashes,
		})
	})

	// 注意：2026-08-06 三通道架构后，AI 回复不再经内联长轮询（HTTPReplyPullerRegistry 已移除），
	// 而是落库 message_hub(status=pending) 由桥接扩展 GET /api/bridge/outbox 拉取。
	// 本 mock 仅验证 ingest 上行链路；AI 回复的 outbox 下发需真实 DB/AI 引擎，mock 不实现。

	srv := &http.Server{Addr: ":" + port, Handler: r}
	go func() {
		log.Printf("============================================================")
		log.Printf("[bridge-mock] HTTP server listening on http://localhost:%s", port)
		log.Printf("  AI delay: %v  |  default reply: %q", aiDelay, aiReply)
		log.Printf("============================================================")
		log.Printf("Endpoints:")
		log.Printf("  POST /api/bridge/ingest   (HTTP long-poll ingest)")
		log.Printf("  GET  /healthz              (liveness)")
		log.Printf("  GET  /mock/state           (mock state inspection)")
		log.Printf("")
		log.Printf("Tunables (env):")
		log.Printf("  MOCK_PORT          (default %s)", defaultPort)
		log.Printf("  MOCK_AI_DELAY_MS   (default %d)", defaultAIDelayMs)
		log.Printf("  MOCK_AI_REPLY      (default %q)", mockAIDefaultReply)
		log.Printf("============================================================")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[bridge-mock] server error: %v", err)
		}
	}()

	// 优雅退出
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Printf("[bridge-mock] shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Printf("[bridge-mock] bye")
}

// readBodyPreview 读取 body 全部内容 + 前 N 字节预览（与生产 handler 同源）
func readBodyPreview(rc interface {
	Read(p []byte) (n int, err error)
	Close() error
}, n int) (string, error) {
	if rc == nil {
		return "", nil
	}
	buf := make([]byte, 1024*1024)
	total := 0
	for total < len(buf) {
		nn, err := rc.Read(buf[total:])
		total += nn
		if err != nil {
			break
		}
		if nn == 0 {
			break
		}
	}
	_ = rc.Close()
	preview := string(buf[:total])
	if len(preview) > n {
		preview = preview[:n] + fmt.Sprintf("... [truncated, total=%d bytes]", total)
	}
	return preview, nil
}

type readCloser struct {
	r interface{ Read(p []byte) (n int, err error) }
}

func (rc readCloser) Read(p []byte) (int, error) { return rc.r.Read(p) }
func (rc readCloser) Close() error               { return nil }

// 编译期断言：用到相关包避免被优化
var _ = json.Marshal
