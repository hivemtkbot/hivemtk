package model

import (
	_type "marketing/internal/pkg/utils/type"
	"testing"
)

func TestMessage_TableName(t *testing.T) {
	message := &Message{}
	tableName := message.TableName()
	if tableName != "message" {
		t.Errorf("Expected table name 'message', got %s", tableName)
	}
}

func TestMessage_BasicFields(t *testing.T) {
	message := &Message{
		Status:     _type.UserStatusType(1),
		TgID:       123456789,
		CreateTime: 1234567890,
		AccountID:  "account-123",
		UserID:     "user-456",
		Text:       "Hello, World!",
	}

	if message.Status != 1 {
		t.Errorf("Expected Status 1, got %d", message.Status)
	}
	if message.TgID != 123456789 {
		t.Errorf("Expected TgID 123456789, got %d", message.TgID)
	}
	if message.CreateTime != 1234567890 {
		t.Errorf("Expected CreateTime 1234567890, got %d", message.CreateTime)
	}
	if message.AccountID != "account-123" {
		t.Errorf("Expected AccountID 'account-123', got %s", message.AccountID)
	}
	if message.UserID != "user-456" {
		t.Errorf("Expected UserID 'user-456', got %s", message.UserID)
	}
	if message.Text != "Hello, World!" {
		t.Errorf("Expected Text 'Hello, World!', got %s", message.Text)
	}
}

func TestMessage_WithEmptyID(t *testing.T) {
	message := &Message{
		Text: "Test message",
		ID:   "",
	}

	// ID should be empty before BeforeCreate is called
	if message.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", message.ID)
	}
}

func TestMessage_WithStatusValues(t *testing.T) {
	statuses := []_type.UserStatusType{0, 1, 2}

	for _, status := range statuses {
		message := &Message{
			Status: status,
		}
		if message.Status != status {
			t.Errorf("Expected Status %d, got %d", status, message.Status)
		}
	}
}

func TestMessage_WithLongText(t *testing.T) {
	longText := "This is a very long message that exceeds the normal length limit of 255 characters. It should still be stored in the database but might be truncated depending on the database configuration."
	message := &Message{
		Text: longText,
	}

	if message.Text != longText {
		t.Error("Expected long text to be stored")
	}
}

func TestMessage_WithEmptyText(t *testing.T) {
	message := &Message{
		Text: "",
	}

	if message.Text != "" {
		t.Errorf("Expected empty Text, got %s", message.Text)
	}
}

func TestMessage_WithTgID(t *testing.T) {
	message := &Message{
		TgID: 987654321,
	}

	if message.TgID != 987654321 {
		t.Errorf("Expected TgID 987654321, got %d", message.TgID)
	}
}

func TestMessage_BeforeCreate(t *testing.T) {
	message := &Message{
		Text: "Test message",
	}

	// BeforeCreate should generate an ID
	err := message.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if message.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Verify it's a valid UUID format
	if len(message.ID) != 36 {
		t.Errorf("Expected ID length 36 (UUID), got %d", len(message.ID))
	}
}
