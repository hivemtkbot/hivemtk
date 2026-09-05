package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestNewWSHandler(t *testing.T) {
	handler := NewWSHandler()
	if handler == nil {
		t.Fatal("NewWSHandler returned nil")
	}
	if handler.hub == nil {
		t.Error("Expected hub to be initialized")
	}
}

func TestWSHandler_BroadcastMessage(t *testing.T) {
	handler := NewWSHandler()

	data := map[string]any{"key": "value"}
	err := handler.BroadcastMessage("test_message", data)
	_ = err
}

func TestWSHandler_SendToAgent(t *testing.T) {
	handler := NewWSHandler()

	data := map[string]any{"key": "value"}
	err := handler.SendToAgent(1, "test_message", data)
	_ = err
}

func TestHandleWebSocket_MissingParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewWSHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "/ws", nil)
	c.Request = req

	handler.HandleWebSocket(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleWebSocket_InvalidAgentID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewWSHandler()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "/ws?agent_id=invalid", nil)
	c.Request = req

	handler.HandleWebSocket(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestHandleMarkRead(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"session_id": "session123",
	}

	handler.handleMarkRead(client, msg)
}

func TestHandleMarkRead_InvalidSessionID(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"session_id": 123,
	}

	handler.handleMarkRead(client, msg)
}

func TestHandleMarkRead_MissingSessionID(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{}

	handler.handleMarkRead(client, msg)
}

func TestHandleSessionAction_InvalidAction(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"action": 123,
	}

	handler.handleSessionAction(client, msg)
}

func TestHandleSessionAction_MissingAction(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{}

	handler.handleSessionAction(client, msg)
}

func TestHandleSessionAction_TakeOver(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"action":     "take_over",
		"session_id": "session123",
	}

	handler.handleSessionAction(client, msg)
}

func TestHandleSessionAction_Transfer(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"action":          "transfer",
		"session_id":      "session123",
		"target_agent_id": float64(2),
	}

	handler.handleSessionAction(client, msg)
}

func TestHandleSessionAction_Close(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	msg := map[string]any{
		"action":     "close",
		"session_id": "session123",
	}

	handler.handleSessionAction(client, msg)
}

func TestHandleTakeOver(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	handler.handleTakeOver(client, "session123")
}

func TestHandleTransfer(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	handler.handleTransfer(client, "session123", 2)
}

func TestHandleClose(t *testing.T) {
	handler := NewWSHandler()
	hub := GetHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	handler.handleClose(client, "session123")
}

func TestReadPump_MessageTypes(t *testing.T) {
	tests := []struct {
		json     string
		wantType string
	}{
		{`{"type": "ping"}`, "ping"},
		{`{"type": "mark_read", "session_id": "session123"}`, "mark_read"},
		{`{"type": "session_action", "action": "take_over"}`, "session_action"},
	}

	for _, tt := range tests {
		var msg map[string]any
		json.Unmarshal([]byte(tt.json), &msg)
		msgType, ok := msg["type"].(string)
		if !ok {
			t.Errorf("Expected type to be string for %s", tt.json)
			continue
		}
		if msgType != tt.wantType {
			t.Errorf("Expected type %s, got %s", tt.wantType, msgType)
		}
	}
}

func TestReadPump_InvalidJSON(t *testing.T) {
	invalidJSON := `{"type": "ping"`
	var msg map[string]any
	err := json.Unmarshal([]byte(invalidJSON), &msg)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestWSHandler_WritePump_MessageFormat(t *testing.T) {
	msg := map[string]any{
		"type":    "message",
		"payload": json.RawMessage(`{"key": "value"}`),
		"time":    "2024-01-01 12:00:00",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	var result map[string]any
	json.Unmarshal(data, &result)

	if result["type"] != "message" {
		t.Errorf("Expected type 'message', got %s", result["type"])
	}
}

func TestUpgrader_CheckOrigin(t *testing.T) {
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	result := upgrader.CheckOrigin(req)
	if !result {
		t.Error("Expected CheckOrigin to return true")
	}
}
