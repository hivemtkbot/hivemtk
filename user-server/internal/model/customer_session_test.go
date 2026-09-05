package model

import (
	"testing"
	"time"
)

func TestSessionStatus_Constants(t *testing.T) {
	statuses := map[SessionStatus]string{
		SessionStatusPending:       "pending",
		SessionStatusAIHandling:    "ai_handling",
		SessionStatusHumanHandling: "human_handling",
		SessionStatusWaiting:       "waiting",
		SessionStatusResolved:      "resolved",
		SessionStatusClosed:        "closed",
	}

	for status, expected := range statuses {
		if string(status) != expected {
			t.Errorf("Expected SessionStatus %s, got %s", expected, status)
		}
	}
}

func TestHandlerType_Constants(t *testing.T) {
	handlerTypes := map[HandlerType]string{
		HandlerTypeAI:    "ai",
		HandlerTypeHuman: "human",
	}

	for handlerType, expected := range handlerTypes {
		if string(handlerType) != expected {
			t.Errorf("Expected HandlerType %s, got %s", expected, handlerType)
		}
	}
}

func TestCustomerSession_TableName(t *testing.T) {
	session := &CustomerSession{}
	tableName := session.TableName()
	if tableName != "customer_sessions" {
		t.Errorf("Expected table name 'customer_sessions', got %s", tableName)
	}
}

func TestCustomerSession_BasicFields(t *testing.T) {
	now := time.Now()

	session := &CustomerSession{
		ID:              1,
		SessionID:       "sess-001",
		Platform:        PlatformDouyin,
		AccountID:       "account-001",
		UserID:          "user-001",
		UserName:        "Test User",
		UserAvatar:      "https://example.com/avatar.jpg",
		Status:          SessionStatusPending,
		HandlerType:     HandlerTypeAI,
		Priority:        1,
		LastMessage:     "Hello, I need help",
		LastMessageAt:   &now,
		LastMessageBy:   "user",
		MessageCount:    5,
		AIReplyCount:    3,
		HumanReplyCount: 2,
		AvgResponseTime: 30,
		Rating:          5,
		RatingComment:   "Great service!",
		Tags:            `["vip", "priority"]`,
	}

	if session.ID != 1 {
		t.Errorf("Expected ID 1, got %d", session.ID)
	}
	if session.SessionID != "sess-001" {
		t.Errorf("Expected SessionID 'sess-001', got %s", session.SessionID)
	}
	if session.Platform != PlatformDouyin {
		t.Errorf("Expected Platform 'douyin', got %s", session.Platform)
	}
	if session.Status != SessionStatusPending {
		t.Errorf("Expected Status 'pending', got %s", session.Status)
	}
	if session.Priority != 1 {
		t.Errorf("Expected Priority 1, got %d", session.Priority)
	}
	if session.Rating != 5 {
		t.Errorf("Expected Rating 5, got %d", session.Rating)
	}
}

func TestSessionMessage_TableName(t *testing.T) {
	message := &SessionMessage{}
	tableName := message.TableName()
	if tableName != "session_messages" {
		t.Errorf("Expected table name 'session_messages', got %s", tableName)
	}
}

func TestSessionMessage_BasicFields(t *testing.T) {
	now := time.Now()

	message := &SessionMessage{
		ID:           1,
		SessionID:    "sess-001",
		Content:      "Hello, how can I help you?",
		ContentType:  MessageTypeText,
		SenderType:   "ai",
		SenderID:     "ai-001",
		SenderName:   "AI Assistant",
		AIConfidence: 0.95,
		AISource:     "llm",
		IsRead:       true,
		ReadAt:       &now,
	}

	if message.ID != 1 {
		t.Errorf("Expected ID 1, got %d", message.ID)
	}
	if message.Content != "Hello, how can I help you?" {
		t.Errorf("Expected Content, got %s", message.Content)
	}
	if message.ContentType != MessageTypeText {
		t.Errorf("Expected ContentType 'text', got %s", message.ContentType)
	}
	if message.AIConfidence != 0.95 {
		t.Errorf("Expected AIConfidence 0.95, got %f", message.AIConfidence)
	}
	if !message.IsRead {
		t.Error("Expected IsRead to be true")
	}
}

func TestAgentStatus_TableName(t *testing.T) {
	status := &AgentStatus{}
	tableName := status.TableName()
	if tableName != "agent_statuses" {
		t.Errorf("Expected table name 'agent_statuses', got %s", tableName)
	}
}

func TestAgentStatus_BasicFields(t *testing.T) {
	now := time.Now()

	status := &AgentStatus{
		ID:              1,
		AgentID:         100,
		AgentName:       "Agent Smith",
		Status:          "online",
		MaxSessions:     5,
		ActiveSessions:  3,
		TodaySessions:   10,
		TodayMessages:   50,
		AvgResponseTime: 25,
		OnlineAt:        &now,
		LastActiveAt:    &now,
	}

	if status.ID != 1 {
		t.Errorf("Expected ID 1, got %d", status.ID)
	}
	if status.AgentID != 100 {
		t.Errorf("Expected AgentID 100, got %d", status.AgentID)
	}
	if status.Status != "online" {
		t.Errorf("Expected Status 'online', got %s", status.Status)
	}
	if status.MaxSessions != 5 {
		t.Errorf("Expected MaxSessions 5, got %d", status.MaxSessions)
	}
}

func TestAISuggestion_TableName(t *testing.T) {
	suggestion := &AISuggestion{}
	tableName := suggestion.TableName()
	if tableName != "ai_suggestions" {
		t.Errorf("Expected table name 'ai_suggestions', got %s", tableName)
	}
}

func TestAISuggestion_BasicFields(t *testing.T) {
	now := time.Now()

	suggestion := &AISuggestion{
		ID:         1,
		SessionID:  "sess-001",
		MessageID:  5,
		Suggestion: "Please check the order status",
		Confidence: 0.85,
		Source:     "rag",
		IsUsed:     true,
		UsedBy:     100,
		UsedAt:     &now,
	}

	if suggestion.ID != 1 {
		t.Errorf("Expected ID 1, got %d", suggestion.ID)
	}
	if suggestion.Suggestion != "Please check the order status" {
		t.Errorf("Expected Suggestion, got %s", suggestion.Suggestion)
	}
	if suggestion.Confidence != 0.85 {
		t.Errorf("Expected Confidence 0.85, got %f", suggestion.Confidence)
	}
	if !suggestion.IsUsed {
		t.Error("Expected IsUsed to be true")
	}
}

func TestQuickReply_TableName(t *testing.T) {
	reply := &QuickReply{}
	tableName := reply.TableName()
	if tableName != "quick_replies" {
		t.Errorf("Expected table name 'quick_replies', got %s", tableName)
	}
}

func TestQuickReply_BasicFields(t *testing.T) {
	reply := &QuickReply{
		ID:        1,
		Category:  "greeting",
		Title:     "Welcome Message",
		Content:   "Hello! Welcome to our store. How can I help you?",
		SortOrder: 1,
		IsPublic:  true,
		CreatedBy: 100,
	}

	if reply.ID != 1 {
		t.Errorf("Expected ID 1, got %d", reply.ID)
	}
	if reply.Category != "greeting" {
		t.Errorf("Expected Category 'greeting', got %s", reply.Category)
	}
	if reply.Title != "Welcome Message" {
		t.Errorf("Expected Title 'Welcome Message', got %s", reply.Title)
	}
	if !reply.IsPublic {
		t.Error("Expected IsPublic to be true")
	}
}

func TestSessionTag_TableName(t *testing.T) {
	tag := &SessionTag{}
	tableName := tag.TableName()
	if tableName != "session_tags" {
		t.Errorf("Expected table name 'session_tags', got %s", tableName)
	}
}

func TestSessionTag_BasicFields(t *testing.T) {
	tag := &SessionTag{
		ID:        1,
		Name:      "VIP Customer",
		Color:     "#FF5722",
		SortOrder: 1,
	}

	if tag.ID != 1 {
		t.Errorf("Expected ID 1, got %d", tag.ID)
	}
	if tag.Name != "VIP Customer" {
		t.Errorf("Expected Name 'VIP Customer', got %s", tag.Name)
	}
	if tag.Color != "#FF5722" {
		t.Errorf("Expected Color '#FF5722', got %s", tag.Color)
	}
}
