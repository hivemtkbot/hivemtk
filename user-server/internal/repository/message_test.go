package repository

import (
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	_type "marketing/internal/pkg/utils/type"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupMessageTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.Message{},
	)
	db.SetTestDB(database)
	return database
}

func TestMessageRepository_New(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

func TestMessageRepository_Create(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	message := &model.Message{
		Status:    _type.UserStatusValid,
		TgID:      12345,
		AccountID: "account123",
		UserID:    "user123",
		Text:      "Test message",
	}

	err := repo.Create(message)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if message.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestMessageRepository_Create_EmptyText(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	message := &model.Message{
		Status:    _type.UserStatusValid,
		TgID:      12345,
		AccountID: "account123",
		UserID:    "user123",
		Text:      "",
	}

	err := repo.Create(message)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if message.ID == "" {
		t.Error("Expected non-empty ID after create")
	}
}

func TestMessageRepository_GetMessageList(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	for i := 0; i < 10; i++ {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
			UserID:    "user" + string(rune('0'+i)),
			Text:      "Message " + string(rune('0'+i)),
		}
		repo.Create(message)
	}

	messages, total, err := repo.GetMessageList(1, 5)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages, got %d", len(messages))
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}
}

func TestMessageRepository_GetMessageList_Empty(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	messages, total, err := repo.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(messages))
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}
}

func TestMessageRepository_GetMessageList_SecondPage(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	for i := 0; i < 10; i++ {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
			UserID:    "user" + string(rune('0'+i)),
			Text:      "Message " + string(rune('0'+i)),
		}
		repo.Create(message)
	}

	messages, _, err := repo.GetMessageList(2, 5)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages on page 2, got %d", len(messages))
	}
}

func TestMessageRepository_Create_MultipleMessages(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	// Create multiple messages for same user
	for i := 0; i < 5; i++ {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      12345,
			AccountID: "account123",
			UserID:    "user123",
			Text:      "Message " + string(rune('0'+i)),
		}
		repo.Create(message)
	}

	messages, total, err := repo.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 5 {
		t.Errorf("Expected 5 messages for same user, got %d", len(messages))
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}

func TestMessageRepository_Create_DifferentUsers(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	// Create messages for different users
	users := []string{"user1", "user2", "user3"}
	for i, userID := range users {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
			UserID:    userID,
			Text:      "Message from " + userID,
		}
		repo.Create(message)
	}

	messages, total, err := repo.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 3 {
		t.Errorf("Expected 3 messages for different users, got %d", len(messages))
	}

	if total != 3 {
		t.Errorf("Expected total 3, got %d", total)
	}
}

func TestMessageRepository_GetMessageList_WithStatus(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	// Create messages with different status
	for i := 0; i < 3; i++ {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
			UserID:    "user" + string(rune('0'+i)),
			Text:      "Valid Message " + string(rune('0'+i)),
		}
		repo.Create(message)
	}

	for i := 0; i < 2; i++ {
		message := &model.Message{
			Status:    _type.UserStatusInvalid,
			TgID:      int64(20000 + i),
			AccountID: "account" + string(rune('a'+i)),
			UserID:    "user" + string(rune('a'+i)),
			Text:      "Invalid Message " + string(rune('a'+i)),
		}
		repo.Create(message)
	}

	_, total, err := repo.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5 messages, got %d", total)
	}
}

func TestMessageRepository_Create_LargeBatch(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	// Create 100 messages
	for i := 0; i < 100; i++ {
		message := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i%10)),
			UserID:    "user" + string(rune('0'+i)),
			Text:      "Batch message " + string(rune('0'+i%10)),
		}
		repo.Create(message)
	}

	messages, total, err := repo.GetMessageList(1, 50)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if len(messages) != 50 {
		t.Errorf("Expected 50 messages, got %d", len(messages))
	}

	if total != 100 {
		t.Errorf("Expected total 100, got %d", total)
	}
}

func TestMessageRepository_Create_VariousTextLengths(t *testing.T) {
	setupMessageTestDB(t)

	repo := NewMessageRepository()

	// Create messages with various text lengths
	messages := []string{
		"Short",
		"This is a medium length message",
		"This is a very long message that contains many characters to test how the repository handles longer text content in the message field",
		"",
		"Special chars: !@#$%^&*()_+-=[]{}|;':\",./<>?",
	}

	for i, text := range messages {
		msg := &model.Message{
			Status:    _type.UserStatusValid,
			TgID:      int64(10000 + i),
			AccountID: "account" + string(rune('0'+i)),
			UserID:    "user" + string(rune('0'+i)),
			Text:      text,
		}
		repo.Create(msg)
	}

	_, total, err := repo.GetMessageList(1, 10)
	if err != nil {
		t.Fatalf("GetMessageList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}
}
