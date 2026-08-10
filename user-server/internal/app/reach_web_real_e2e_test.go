package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/testutil"
	"hivemtk-user/internal/websocket"
)

// ============================================================================
// reach.web.send 真实端到端打通测试（零 mock）
// ----------------------------------------------------------------------------
// 验收目标：智能体调用 reach.web.send 后，网页客服渠道真实完成以下闭环，
// 全程不使用任何 mock / 假实现：
//   1. 真实 PostgreSQL 中 customer_sessions 存在目标会话
//   2. 真实调用 IntegrationReachAdapter.SendWeb（非 NoOp、非 mock）
//   3. 消息真实落库到 session_messages（sender_type=agent, sender_name=客服）
//   4. 真实 WebSocket Hub 将消息实时推送给「在线访客前端」
//   5. 访客前端（真实 Client.send 通道）真实收到 type=message 帧且内容匹配
//
// 前置：需要可用 PostgreSQL（testutil 默认 127.0.0.1:5434）。
// ============================================================================

// TestReachWebSend_RealEndToEnd 真实打通网页客服渠道
func TestReachWebSend_RealEndToEnd(t *testing.T) {
	// ---- 1. 真实 PostgreSQL（含网页客服相关模型）----
	db := testutil.NewTestDB(t,
		&model.CustomerSession{},
		&model.SessionMessage{},
		&model.AgentStatus{},
		&model.AISuggestion{},
	)

	sessionID := "real-web-session-001"
	visitorID := "real-visitor-001"

	// 插入一条真实会话（智能体推送的目标）
	now := time.Now()
	session := &model.CustomerSession{
		SessionID:   sessionID,
		Platform:    "web",
		AccountID:   "acc-1",
		UserID:      visitorID,
		UserName:    "真实访客",
		Status:      model.SessionStatusAIHandling,
		HandlerType: model.HandlerTypeAI,
		CreatedAt:   now,
	}
	if err := db.Create(session).Error; err != nil {
		t.Fatalf("插入真实会话失败：%v", err)
	}

	// ---- 2. 真实 WebSocket Hub + 真实访客前端 Client（模拟网页已连接）----
	hub := websocket.GetHub()
	visitor := websocket.NewVisitorClient(hub, sessionID, visitorID, "default")
	websocket.RegisterVisitorClient(visitor)
	defer websocket.UnregisterVisitorClient(visitor)

	if !websocket.IsVisitorOnline(sessionID) {
		t.Fatalf("前置失败：访客应处于在线状态（真实 WebSocket 注册）")
	}

	// ---- 3. 真实适配器 SendWeb（非 NoOp / 非 mock）----
	adapter := NewIntegrationReachAdapterFromDB(db)
	const pushContent = "【智能体】您好，这款全自动纸吸管采用食品级原纸，可降解更环保～"
	msgID, err := adapter.SendWeb(context.Background(), sessionID, pushContent)
	if err != nil {
		t.Fatalf("SendWeb 真实调用失败：%v", err)
	}
	if msgID == "" {
		t.Fatalf("SendWeb 应返回非空 msgID")
	}
	t.Logf("✅ SendWeb 真实返回 msgID=%s", msgID)

	// ---- 4. 断言 DB 真实落库 ----
	var dbMsg model.SessionMessage
	if qErr := db.Where("session_id = ? AND content = ?", sessionID, pushContent).
		First(&dbMsg).Error; qErr != nil {
		t.Fatalf("消息未真实落库 session_messages：%v", qErr)
	}
	if dbMsg.SenderType != "agent" {
		t.Fatalf("落库消息 sender_type 应为 agent，实际 %q", dbMsg.SenderType)
	}
	if dbMsg.SenderName != "客服" {
		t.Fatalf("落库消息 sender_name 应为 客服，实际 %q", dbMsg.SenderName)
	}
	t.Logf("✅ 消息真实落库 session_messages(id=%d, sender=%s)", dbMsg.ID, dbMsg.SenderName)

	// 会话最后消息应被更新
	var updated model.CustomerSession
	if qErr := db.Where("session_id = ?", sessionID).First(&updated).Error; qErr != nil {
		t.Fatalf("查询会话失败：%v", qErr)
	}
	if updated.LastMessage != pushContent {
		t.Fatalf("会话 LastMessage 未被真实更新，实际 %q", updated.LastMessage)
	}
	if updated.HumanReplyCount != 1 {
		t.Fatalf("会话 HumanReplyCount 应 +1，实际 %d", updated.HumanReplyCount)
	}
	t.Logf("✅ 会话 LastMessage/HumanReplyCount 真实更新")

	// ---- 5. 断言访客前端（真实 Client.send 通道）真实收到推送 ----
	select {
	case raw := <-visitor.SendChan():
		var frame struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if uErr := json.Unmarshal(raw, &frame); uErr != nil {
			t.Fatalf("收到非法 WebSocket 帧：%v\nraw=%s", uErr, string(raw))
		}
		if frame.Type != websocket.TypeMessage {
			t.Fatalf("访客前端收到帧 type 应为 %q，实际 %q", websocket.TypeMessage, frame.Type)
		}
		if frame.Payload["content"] != pushContent {
			t.Fatalf("访客前端收到内容不匹配：%v", frame.Payload["content"])
		}
		if frame.Payload["sender_name"] != "客服" {
			t.Fatalf("访客前端收到 sender_name 应为 客服，实际 %v", frame.Payload["sender_name"])
		}
		t.Logf("✅ 访客前端 WebSocket 真实收到推送：%s", string(raw))
	case <-time.After(3 * time.Second):
		t.Fatalf("超时：访客前端未真实收到 WebSocket 推送（web 渠道未打通）")
	}

	// ---- 6. 错误路径：会话不存在应真实报错（非静默成功）----
	_, errNotFound := adapter.SendWeb(context.Background(), "non-existent-session", "内容")
	if errNotFound == nil {
		t.Fatalf("会话不存在时 SendWeb 应返回错误，实际静默成功")
	}
	t.Logf("✅ 会话不存在真实返回错误：%v", errNotFound)
}
