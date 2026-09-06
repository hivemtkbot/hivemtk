package crypto

import (
	"os"
	"strings"
	"sync"
	"testing"
)

// TestMain 在测试前设置测试 KEY（CI 环境也安全）
func TestMain(m *testing.M) {
	os.Setenv("FIELD_ENCRYPTION_KEY", "test-encryption-key-must-be-32-chars-min")

	once = sync.Once{}
	gcm = nil
	initErr = nil
	m.Run()
}

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	Init()
	plaintext := "user_secret_13812345678@example.com"
	ciphertext, err := Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	if ciphertext == plaintext {
		t.Error("ciphertext should differ from plaintext")
	}
	if !strings.Contains(ciphertext, "==") && len(ciphertext) < 20 {
		t.Error("ciphertext should be base64 encoded and sufficiently long")
	}
	decrypted, err := Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("round trip failed: got %q want %q", decrypted, plaintext)
	}
}

func TestEncryptEmptyString(t *testing.T) {
	Init()
	ct, err := Encrypt("")
	if err != nil {
		t.Fatalf("Encrypt empty failed: %v", err)
	}
	if ct != "" {
		t.Errorf("Encrypt empty should return empty, got %q", ct)
	}
}

func TestDecryptEmptyString(t *testing.T) {
	Init()
	pt, err := Decrypt("")
	if err != nil {
		t.Fatalf("Decrypt empty failed: %v", err)
	}
	if pt != "" {
		t.Errorf("Decrypt empty should return empty, got %q", pt)
	}
}

func TestDecryptInvalidBase64(t *testing.T) {
	Init()
	_, err := Decrypt("not-base64!@#$")
	if err == nil {
		t.Error("Decrypt invalid base64 should return error")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {

	Init()
	plaintext := "test_plaintext_aaa"
	ct1, _ := Encrypt(plaintext)
	ct2, _ := Encrypt(plaintext)
	if ct1 == ct2 {
		t.Error("two encryptions of same plaintext should differ (nonce random)")
	}
}

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"13812345678", "138****5678"},
		{"1234567", "123****4567"},
		{"12345", "****"},
		{"", "****"},
		{"12", "****"},
	}
	for _, c := range cases {
		got := MaskPhone(c.input)
		if got != c.expected {
			t.Errorf("MaskPhone(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestMaskEmail(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"john.doe@example.com", "j*********@example.com"},
		{"ab@example.com", "**@example.com"},
		{"a@example.com", "****"},
		{"no-at-sign", "****"},
	}
	for _, c := range cases {
		got := MaskEmail(c.input)
		if got == c.input {
			t.Errorf("MaskEmail should mask input, got original: %q", got)
		}
	}

	if got := MaskEmail(""); got != "" {
		t.Errorf("MaskEmail(\"\") should return empty, got %q", got)
	}
}
