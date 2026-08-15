package websocket

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewHub(t *testing.T) {
	hub := NewHub()
	if hub == nil {
		t.Fatal("NewHub returned nil")
	}
	if hub.clients == nil {
		t.Error("Expected clients to be initialized")
	}
	if hub.register == nil {
		t.Error("Expected register channel to be initialized")
	}
	if hub.unregister == nil {
		t.Error("Expected unregister channel to be initialized")
	}
	if hub.broadcast == nil {
		t.Error("Expected broadcast channel to be initialized")
	}
}

func TestNewWSClient(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.agentID != "agent1" {
		t.Errorf("Expected agentID 'agent1', got %s", client.agentID)
	}
	if client.agentName != "Agent One" {
		t.Errorf("Expected agentName 'Agent One', got %s", client.agentName)
	}
}

func TestHub_Register(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	go hub.Run()
	defer func() {
		hub.Unregister(client)
	}()

	hub.Register(client)

	time.Sleep(100 * time.Millisecond)

	if !hub.IsAgentOnline("agent1") {
		t.Error("Expected agent to be online after registration")
	}
}

func TestHub_Unregister(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")
	_ = client

	go hub.Run()

	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	hub.Unregister(client)
	time.Sleep(50 * time.Millisecond)

	if hub.IsAgentOnline("agent1") {
		t.Error("Expected agent to be offline after unregistration")
	}
}

func TestHub_Broadcast(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")
	_ = client

	go hub.Run()
	defer hub.Unregister(client)

	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	payload := map[string]string{"test": "data"}
	err := hub.SendToAgent("agent1", "test_message", payload)
	if err != nil {
		t.Errorf("SendToAgent failed: %v", err)
	}
}

func TestHub_SendToAgent(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")

	go hub.Run()
	defer hub.Unregister(client)

	hub.Register(client)
	time.Sleep(50 * time.Millisecond)

	payload := map[string]any{"key": "value"}
	err := hub.SendToAgent("agent1", "new_message", payload)
	if err != nil {
		t.Errorf("SendToAgent failed: %v", err)
	}
}

func TestHub_BroadcastToMerchant(t *testing.T) {
	hub := NewHub()
	client1 := NewWSClient(hub, "agent1", "Agent One")
	client2 := NewWSClient(hub, "agent2", "Agent Two")
	client3 := NewWSClient(hub, "agent3", "Agent Three")

	go hub.Run()
	defer func() {
		hub.Unregister(client1)
		hub.Unregister(client2)
		hub.Unregister(client3)
	}()

	hub.Register(client1)
	hub.Register(client2)
	hub.Register(client3)
	time.Sleep(100 * time.Millisecond)

	payload := map[string]any{"status": "online"}
	err := hub.BroadcastToMerchant("merchant1", "agent_status", payload)
	if err != nil {
		t.Errorf("BroadcastToMerchant failed: %v", err)
	}
}

func TestHub_IsAgentOnline(t *testing.T) {
	hub := NewHub()

	if hub.IsAgentOnline("nonexistent") {
		t.Error("Expected nonexistent agent to be offline")
	}
}

func TestHub_GetOnlineAgents(t *testing.T) {
	hub := NewHub()
	client1 := NewWSClient(hub, "agent1", "Agent One")
	client2 := NewWSClient(hub, "agent2", "Agent Two")

	go hub.Run()
	defer func() {
		hub.Unregister(client1)
		hub.Unregister(client2)
	}()

	hub.Register(client1)
	hub.Register(client2)
	time.Sleep(100 * time.Millisecond)

	agents := hub.GetOnlineAgents("merchant1")
	if len(agents) != 2 {
		t.Errorf("Expected 2 online agents, got %d", len(agents))
	}
}

func TestHub_GetOnlineAgents_Empty(t *testing.T) {
	hub := NewHub()

	agents := hub.GetOnlineAgents("nonexistent")
	if len(agents) != 0 {
		t.Errorf("Expected 0 online agents, got %d", len(agents))
	}
}

func TestHub_GetOnlineCount(t *testing.T) {
	hub := NewHub()

	count := hub.GetOnlineCount()
	if count != 0 {
		t.Errorf("Expected 0 online clients, got %d", count)
	}
}

func TestGetHub(t *testing.T) {
	hub1 := GetHub()
	hub2 := GetHub()

	if hub1 != hub2 {
		t.Error("GetHub should return the same instance")
	}
}

func TestNotifyNewSession(t *testing.T) {
	sessionData := map[string]any{
		"session_id": "session1",
		"user_id":    "user1",
	}

	err := NotifyNewSession("agent1", sessionData)
	_ = err
}

func TestNotifyNewMessage(t *testing.T) {
	messageData := map[string]any{
		"message_id": "msg1",
		"content":    "Hello",
	}

	err := NotifyNewMessage("agent1", messageData)
	_ = err
}

func TestNotifySessionUpdate(t *testing.T) {
	sessionData := map[string]any{
		"session_id": "session1",
		"status":     "active",
	}

	err := NotifySessionUpdate("agent1", sessionData)
	_ = err
}

func TestNotifyAISuggestion(t *testing.T) {
	suggestionData := map[string]any{
		"suggestion": "Reply with greeting",
		"confidence": 0.9,
	}

	err := NotifyAISuggestion("agent1", suggestionData)
	_ = err
}

func TestBroadcastAgentStatus(t *testing.T) {
	statusData := map[string]any{
		"agent_id": "agent1",
		"status":   "online",
	}

	err := BroadcastAgentStatus(statusData)
	_ = err
}

func TestMessageJSON(t *testing.T) {
	payload := map[string]string{"key": "value"}
	payloadBytes, _ := json.Marshal(payload)

	msg := &Message{
		Type:    "test_type",
		AgentID: "agent1",
		Payload: payloadBytes,
	}

	if msg.Type != "test_type" {
		t.Errorf("Expected Type 'test_type', got %s", msg.Type)
	}
	if msg.AgentID != "agent1" {
		t.Errorf("Expected AgentID 'agent1', got %s", msg.AgentID)
	}
}

func TestClient_SendChannel(t *testing.T) {
	hub := NewHub()
	client := NewWSClient(hub, "agent1", "Agent One")
	_ = hub
	_ = client

	if client.send == nil {
		t.Error("Expected client send channel to be initialized")
	}
}

// Test concurrent access to hub
func TestHub_ConcurrentAccess(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			client := NewWSClient(hub, "agent"+string(rune(id)), "Agent")
			hub.Register(client)
			time.Sleep(50 * time.Millisecond)
			hub.Unregister(client)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

