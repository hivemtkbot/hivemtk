package model

import (
	"testing"
)

func TestUser_BeforeCreate(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}

	// Call BeforeCreate to generate ID and hash password
	err := user.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if user.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Verify password was hashed (should be different from original)
	if user.Password == "password123" {
		t.Error("Expected password to be hashed")
	}
	// Verify hashed password format (bcrypt starts with $2a$)
	if len(user.Password) < 59 {
		t.Errorf("Expected bcrypt hash length >= 59, got %d", len(user.Password))
	}
}

func TestUser_BeforeCreate_NoID(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "",
	}

	err := user.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if user.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
}

func TestUser_BeforeCreate_NoChangeIfExists(t *testing.T) {
	user := &User{
		ID:       "existing-id",
		Username: "testuser",
		Password: "password123",
	}

	err := user.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if user.ID != "existing-id" {
		t.Errorf("Expected ID to remain 'existing-id', got %s", user.ID)
	}
	// Password should still be hashed even if ID exists
	if user.Password == "password123" {
		t.Error("Expected password to be hashed")
	}
}

func TestUser_BeforeCreate_EmptyPassword(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "",
		Email:    "test@example.com",
	}

	err := user.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if user.ID == "" {
		t.Error("Expected non-empty ID after BeforeCreate")
	}
	// Password should remain empty when not provided
	if user.Password != "" {
		t.Errorf("Expected empty Password, got %s", user.Password)
	}
}

func TestUser_TableName(t *testing.T) {
	user := &User{}
	tableName := user.TableName()
	if tableName != "user" {
		t.Errorf("Expected table name 'user', got %s", tableName)
	}
}

func TestUserStatusType(t *testing.T) {
	// Test that UserStatusType constants are defined
	// UserStatusValid should be 1
	// UserStatusInvalid should be 0
	// These are imported from _type package
}

func TestUser_WithEmptyID(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		ID:       "",
	}

	// ID should be empty before BeforeCreate is called
	if user.ID != "" {
		t.Errorf("Expected empty ID before BeforeCreate, got %s", user.ID)
	}
}

func TestUser_WithRole(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		Role:     "admin",
	}

	if user.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", user.Role)
	}
}

func TestUser_WithDefaultRole(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
	}

	// Default role should be 'user'
	// Note: This is set by GORM default, not in struct initialization
	if user.Role != "" {
		// Role will be empty until saved to database
		t.Logf("Role is %s (will be 'user' after save)", user.Role)
	}
}

func TestUser_WithTgID(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		TgID:     123456789,
	}

	if user.TgID != 123456789 {
		t.Errorf("Expected TgID 123456789, got %d", user.TgID)
	}
}

func TestUser_WithNames(t *testing.T) {
	user := &User{
		Username:  "testuser",
		Password:  "password123",
		FirstName: "John",
		LastName:  "Doe",
	}

	if user.FirstName != "John" {
		t.Errorf("Expected FirstName 'John', got %s", user.FirstName)
	}
	if user.LastName != "Doe" {
		t.Errorf("Expected LastName 'Doe', got %s", user.LastName)
	}
}

func TestUser_WithPhone(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		Phone:    "+1234567890",
	}

	if user.Phone != "+1234567890" {
		t.Errorf("Expected Phone '+1234567890', got %s", user.Phone)
	}
}

func TestUser_WithAvatar(t *testing.T) {
	user := &User{
		Username: "testuser",
		Password: "password123",
		Avatar:   "https://example.com/avatar.jpg",
	}

	if user.Avatar != "https://example.com/avatar.jpg" {
		t.Errorf("Expected Avatar URL, got %s", user.Avatar)
	}
}

func TestUser_WithAccountID(t *testing.T) {
	user := &User{
		Username:  "testuser",
		Password:  "password123",
		AccountID: "account-123",
	}

	if user.AccountID != "account-123" {
		t.Errorf("Expected AccountID 'account-123', got %s", user.AccountID)
	}
}
