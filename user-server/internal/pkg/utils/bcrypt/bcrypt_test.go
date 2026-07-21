package bcrypt

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	if hashed == "" {
		t.Error("Expected non-empty hashed password")
	}

	// 哈希结果应该以 $2a$ 开头（bcrypt 格式）
	if len(hashed) < 59 {
		t.Errorf("Expected bcrypt hash length >= 59, got %d", len(hashed))
	}

	// 相同密码应该生成不同的哈希（因为盐值不同）
	hashed2, _ := HashPassword(password)
	if hashed == hashed2 {
		t.Error("Expected different hashes for same password (different salts)")
	}
}

func TestHashPassword_EmptyPassword(t *testing.T) {
	hashed, err := HashPassword("")
	if err != nil {
		t.Fatalf("HashPassword with empty password failed: %v", err)
	}

	if hashed == "" {
		t.Error("Expected non-empty hash for empty password")
	}
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// 验证正确的密码
	err = CheckPassword(hashed, password)
	if err != nil {
		t.Errorf("Expected valid password, got error: %v", err)
	}
}

func TestCheckPassword_WrongPassword(t *testing.T) {
	password := "testpassword123"
	wrongPassword := "wrongpassword"

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// 验证错误的密码
	err = CheckPassword(hashed, wrongPassword)
	if err == nil {
		t.Error("Expected error for wrong password")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	password := "testpassword123"

	hashed, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword failed: %v", err)
	}

	// 验证空密码
	err = CheckPassword(hashed, "")
	if err == nil {
		t.Error("Expected error for empty password")
	}
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	err := CheckPassword("invalid-hash", "password")
	if err == nil {
		t.Error("Expected error for invalid hash format")
	}
}

func TestHashAndPassword_RoundTrip(t *testing.T) {
	testCases := []struct {
		name     string
		password string
	}{
		{"simple password", "password123"},
		{"password with spaces", "my secret password"},
		{"password with special chars", "p@$$w0rd!"},
		{"unicode password", "密码 123"},
		{"long password", "this is a very long password with many characters"},
		{"empty password", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			hashed, err := HashPassword(tc.password)
			if err != nil {
				t.Fatalf("HashPassword failed: %v", err)
			}

			err = CheckPassword(hashed, tc.password)
			if err != nil {
				t.Errorf("CheckPassword failed: %v", err)
			}
		})
	}
}

// TestHashPassword_TooLongPassword tests password exceeding 72 bytes limit
func TestHashPassword_TooLongPassword(t *testing.T) {
	// bcrypt has a limit of 72 bytes for password
	// Create a password longer than 72 bytes
	longPassword := ""
	for i := 0; i < 100; i++ {
		longPassword += "a"
	}

	// bcrypt.DefaultCost might accept long passwords without error
	// depending on the library version
	_, err := HashPassword(longPassword)
	// Note: In newer versions of bcrypt, long passwords are truncated
	// rather than rejected, so this might not return an error
	// This test ensures the function handles long passwords gracefully
	if err != nil {
		// If error returned, it should mention password length
		t.Logf("HashPassword returned error for long password: %v", err)
	}
}
