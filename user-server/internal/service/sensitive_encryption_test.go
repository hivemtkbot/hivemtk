package service

import (
	"context"
	"os"
	"testing"
)

func setupEncryptionTest(t *testing.T) *SensitiveFieldEncryption {
	t.Helper()
	// 设置 32 字符以上的测试密钥
	_ = os.Setenv("COOKIE_ENCRYPTION_KEY", "test_cookie_encryption_key_with_at_least_32_chars")
	return NewSensitiveFieldEncryption()
}

func TestSensitiveEncryption_EncryptDecrypt(t *testing.T) {
	e := setupEncryptionTest(t)
	plain := "test_plaintext_13800138000"
	encrypted, err := e.Encrypt(context.Background(), plain)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}
	if encrypted == plain {
		t.Error("encrypted should differ from plain")
	}
	decrypted, err := e.Decrypt(context.Background(), encrypted)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}
	if decrypted != plain {
		t.Errorf("decrypted mismatch: got %q, want %q", decrypted, plain)
	}
}

func TestSensitiveEncryption_Empty(t *testing.T) {
	e := setupEncryptionTest(t)
	encrypted, _ := e.Encrypt(context.Background(), "")
	if encrypted != "" {
		t.Errorf("empty should return empty, got %q", encrypted)
	}
	decrypted, _ := e.Decrypt(context.Background(), "")
	if decrypted != "" {
		t.Errorf("empty should return empty, got %q", decrypted)
	}
}

func TestSensitiveEncryption_AccountCookie(t *testing.T) {
	e := setupEncryptionTest(t)
	platform := "wechat"
	cookie := "session_id=abc123; user_id=test_001"
	encrypted, err := e.EncryptAccountCookie(context.Background(), platform, cookie)
	if err != nil {
		t.Fatalf("encrypt cookie failed: %v", err)
	}
	gotPlatform, gotCookie, err := e.DecryptAccountCookie(context.Background(), encrypted)
	if err != nil {
		t.Fatalf("decrypt cookie failed: %v", err)
	}
	if gotPlatform != platform {
		t.Errorf("platform mismatch: got %q, want %q", gotPlatform, platform)
	}
	if gotCookie != cookie {
		t.Errorf("cookie mismatch: got %q, want %q", gotCookie, cookie)
	}
}

func TestSensitiveEncryption_Phone(t *testing.T) {
	e := setupEncryptionTest(t)
	phone := "13800138000"
	encrypted, _ := e.EncryptPhone(context.Background(), phone)
	if encrypted == phone {
		t.Error("phone should be encrypted")
	}
	decrypted, _ := e.DecryptPhone(context.Background(), encrypted)
	if decrypted != phone {
		t.Errorf("phone roundtrip failed: got %q, want %q", decrypted, phone)
	}
}

func TestSensitiveEncryption_DifferentNonces(t *testing.T) {
	e := setupEncryptionTest(t)
	plain := "same_plaintext"
	c1, _ := e.Encrypt(context.Background(), plain)
	c2, _ := e.Encrypt(context.Background(), plain)
	if c1 == c2 {
		t.Error("same plaintext should produce different ciphertexts (random nonce)")
	}
	d1, _ := e.Decrypt(context.Background(), c1)
	d2, _ := e.Decrypt(context.Background(), c2)
	if d1 != d2 || d1 != plain {
		t.Errorf("roundtrip failed: d1=%q d2=%q plain=%q", d1, d2, plain)
	}
}

func TestSensitiveEncryption_Tamper(t *testing.T) {
	e := setupEncryptionTest(t)
	encrypted, _ := e.Encrypt(context.Background(), "important_data")
	// 篡改密文最后一字节
	if len(encrypted) > 1 {
		encrypted = encrypted[:len(encrypted)-1] + "X"
	}
	_, err := e.Decrypt(context.Background(), encrypted)
	if err == nil {
		t.Error("tampered ciphertext should fail to decrypt")
	}
}
