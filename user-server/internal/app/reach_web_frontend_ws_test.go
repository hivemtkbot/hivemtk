package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	visitorws "hivemtk-user/internal/websocket"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// ============================================================================
// reach.web.send 真实前端 WebSocket 连通测试（零 mock，模拟浏览器访客）
// ----------------------------------------------------------------------------
// 用真实 gin engine + 真实 VisitorWSHandler + 真实 gorilla 客户端，完整模拟
// 「浏览器访客」通过 GET /api/ws/visitor 建立连接，再由 智能体 reach.web.send
// 推送，断言浏览器端真实收到 message 帧。
//
// 全程零 mock：真实 PG + 真实 Hub + 真实 WS Handler + 真实 gorilla 客户端
// + 真实 IntegrationReachAdapter.SendWeb。
// ============================================================================

func TestReachWebSend_FrontendWebSocket(t *testing.T) {
	db := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
	)

	sessionID := "ws-frontend-session-001"
	visitorID := "ws-frontend-visitor-001"

	now := time.Now()
	session := &model.CustomerSession{
		SessionID:   sessionID,
		Platform:    "web",
		AccountID:   "acc-ws",
		UserID:      visitorID,
		UserName:    "浏览器访客",
		Status:      model.SessionStatusAIHandling,
		HandlerType: model.HandlerTypeAI,
		CreatedAt:   now,
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("插入真实会话失败：%v", err)
	}

	// ---- 真实 gin engine + 真实 VisitorWSHandler（模拟网页客服后端）----
	handler := visitorws.NewVisitorWSHandler(db)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/ws/visitor", func(c *gin.Context) {
		q := c.Request.URL.Query()
		q.Set("session_id", sessionID)
		q.Set("visitor_id", visitorID)
		q.Set("channel_id", "default")
		c.Request.URL.RawQuery = q.Encode()
		handler.HandleVisitorWebSocket(c)
	})
	srv := httptest.NewServer(engine)
	defer srv.Close()

	// ---- 真实 gorilla 客户端（模拟浏览器访客前端）----
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/api/ws/visitor"
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("浏览器访客建立 WebSocket 连接失败：%v", err)
	}
	defer conn.Close()
	t.Logf("✅ 浏览器访客 WebSocket 真实连接成功")

	// 读取 welcome 帧（连接建立后服务端推送）
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("读取 welcome 帧失败：%v", err)
	}

	// ---- 真实适配器推送（智能体 reach.web.send）----
	adapter := NewIntegrationReachAdapterFromDB(db)
	const pushContent = "【智能体】全自动纸吸管现已上市，下单享 9 折～"
	if _, err := adapter.SendWeb(context.Background(), sessionID, pushContent); err != nil {
		t.Fatalf("SendWeb 真实调用失败：%v", err)
	}

	// ---- 断言浏览器前端（gorilla 客户端）真实收到推送 ----
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("浏览器前端未真实收到推送：%v", err)
	}
	var frame struct {
		Type    string         `json:"type"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatalf("收到非法帧：%v\nraw=%s", err, string(raw))
	}
	if frame.Type != visitorws.TypeMessage {
		t.Fatalf("前端收到帧 type 应为 %q，实际 %q", visitorws.TypeMessage, frame.Type)
	}
	if frame.Payload["content"] != pushContent {
		t.Fatalf("前端收到内容不匹配：%v", frame.Payload["content"])
	}
	t.Logf("✅ 浏览器访客前端真实收到 智能体推送：%s", string(raw))

	// ---- 断言 DB 真实落库 ----
	var cnt int64
	if err := db.Model(&model.SessionMessage{}).
		Where("session_id = ? AND content = ?", sessionID, pushContent).
		Count(&cnt).Error; err != nil {
		t.Fatalf("统计落库消息失败：%v", err)
	}
	if cnt != 1 {
		t.Fatalf("消息应真实落库 1 条，实际 %d", cnt)
	}
	t.Logf("✅ 消息真实落库 session_messages（count=%d）", cnt)
}

var _ = gorm.ErrRecordNotFound
