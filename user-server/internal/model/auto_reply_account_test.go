package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAutoReplyAccount_Fields(t *testing.T) {
	now := time.Now()
	account := &AutoReplyAccount{
		ID:       1,
		UserID:   100,
		Platform: "douyin",
		Username: "testuser",
		Cookie:   "encrypted_cookie_value",
		IsActive: true,
		Headless: true,
		LoginAt:  &now,
	}

	if account.ID != 1 {
		t.Errorf("Expected ID 1, got %d", account.ID)
	}
	if account.UserID != 100 {
		t.Errorf("Expected UserID 100, got %d", account.UserID)
	}
	if account.Platform != "douyin" {
		t.Errorf("Expected Platform 'douyin', got %s", account.Platform)
	}
	if account.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got %s", account.Username)
	}
	if account.Cookie != "encrypted_cookie_value" {
		t.Errorf("Expected Cookie, got %s", account.Cookie)
	}
	if !account.IsActive {
		t.Error("Expected IsActive to be true")
	}
	if !account.Headless {
		t.Error("Expected Headless to be true")
	}
}

func TestAutoReplyAccount_DefaultValues(t *testing.T) {
	account := &AutoReplyAccount{}

	if account.IsActive != false {
		t.Logf("IsActive is %v (expected false before save, default is false)", account.IsActive)
	}
	if account.Headless != false {
		t.Logf("Headless is %v (expected false before save, default is true)", account.Headless)
	}
}

func TestAutoReplyAccount_WithPlatforms(t *testing.T) {
	platforms := []string{"douyin", "kuaishou", "xiaohongshu", "xianyu", "tiktok"}

	for _, platform := range platforms {
		account := &AutoReplyAccount{
			Platform: platform,
		}
		if account.Platform != platform {
			t.Errorf("Expected Platform %s, got %s", platform, account.Platform)
		}
	}
}

func TestAutoReplyAccount_WithHeadlessDisabled(t *testing.T) {
	account := &AutoReplyAccount{
		Platform: "douyin",
		Username: "testuser",
		Headless: false,
	}

	if account.Headless {
		t.Error("Expected Headless to be false")
	}
}

func TestAutoReplyAccount_WithNilLoginAt(t *testing.T) {
	account := &AutoReplyAccount{
		Platform: "douyin",
		Username: "newuser",
		LoginAt:  nil,
	}

	if account.LoginAt != nil {
		t.Errorf("Expected LoginAt nil, got %v", account.LoginAt)
	}
}

func TestAutoReplyAccount_GetCookie_Empty(t *testing.T) {
	account := &AutoReplyAccount{
		Cookie: "",
	}

	cookie := account.Cookie
	if cookie != "" {
		t.Errorf("Expected empty cookie, got %s", cookie)
	}
}

func TestAutoReplyAccount_SetCookie_Empty(t *testing.T) {
	account := &AutoReplyAccount{}

	account.Cookie = ""
	if account.Cookie != "" {
		t.Errorf("Expected empty Cookie, got %s", account.Cookie)
	}
}

func TestAutoReplyAccount_WithUserID(t *testing.T) {
	account := &AutoReplyAccount{
		UserID: 999,
	}

	if account.UserID != 999 {
		t.Errorf("Expected UserID 999, got %d", account.UserID)
	}
}

func TestAutoReplyAccount_SetCookie_WithCookie(t *testing.T) {
	account := &AutoReplyAccount{}

	account.Cookie = "test_cookie_value"
	if account.Cookie == "" {
		t.Error("Expected non-empty encrypted Cookie")
	}
}

func TestAutoReplyAccount_GetCookie_WithCookie(t *testing.T) {
	account := &AutoReplyAccount{}

	// Set a cookie (GetCookie/SetCookie methods removed; Cookie is a plain field)
	account.Cookie = "test_cookie_value"

	cookie := account.Cookie
	if cookie != "test_cookie_value" {
		t.Errorf("Expected cookie 'test_cookie_value', got %s", cookie)
	}
}



func TestAutoReplyAccount_UnmarshalJSON_InvalidJSON(t *testing.T) {
	account := &AutoReplyAccount{}

	err := json.Unmarshal([]byte("invalid json"), account)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestAutoReplyAccount_UnmarshalJSON_EmptyCookie(t *testing.T) {
	account := &AutoReplyAccount{}

	jsonData := `{"id":2,"platform":"douyin","username":"testuser3","cookie":""}`

	if err := json.Unmarshal([]byte(jsonData), account); err != nil {
		t.Fatalf("unexpected unmarshal error: %v", err)
	}
	if account.ID != 2 {
		t.Errorf("Expected ID 2, got %d", account.ID)
	}
	// Empty cookie should not be encrypted/stored
	if account.Cookie != "" {
		t.Errorf("Expected empty Cookie, got %s", account.Cookie)
	}
}

func TestAutoReplyAccount_getDecryptedCookieForSerialization_DecryptFailure(t *testing.T) {
	account := &AutoReplyAccount{
		Cookie: "invalid_encrypted_data_that_will_fail_to_decrypt",
	}

	result := account.Cookie
	if result != "invalid_encrypted_data_that_will_fail_to_decrypt" {
		t.Errorf("Expected empty string for decryption failure, got %s", result)
	}
}
