package utils

import (
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := "test-encryption-key-32-bytes!"
	data := "Hello, World!"

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if encrypted == "" {
		t.Fatal("Encrypt returned empty string")
	}

	if encrypted == data {
		t.Fatal("Encrypted data should be different from original")
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != data {
		t.Errorf("Decrypted data %q != original %q", decrypted, data)
	}
}

func TestEncrypt_EmptyData(t *testing.T) {
	key := "test-encryption-key-32-bytes!"
	data := ""

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != data {
		t.Errorf("Decrypted data %q != original %q", decrypted, data)
	}
}

func TestEncrypt_LongData(t *testing.T) {
	key := "test-encryption-key-32-bytes!"
	data := "This is a longer string that should still be encrypted and decrypted correctly. It contains multiple sentences to ensure the encryption handles longer inputs properly."

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != data {
		t.Errorf("Decrypted data != original")
	}
}

func TestEncrypt_WithShortKey(t *testing.T) {
	key := "short"
	data := "Hello, World!"

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != data {
		t.Errorf("Decrypted data %q != original %q", decrypted, data)
	}
}

func TestEncrypt_WithLongKey(t *testing.T) {
	key := "this-is-a-very-long-key-that-is-longer-than-32-characters-for-testing"
	data := "Hello, World!"

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if decrypted != data {
		t.Errorf("Decrypted data %q != original %q", decrypted, data)
	}
}

func TestDecrypt_WrongKey(t *testing.T) {
	key := "test-encryption-key-32-bytes!"
	wrongKey := "wrong-encryption-key-32-bytes!"
	data := "Hello, World!"

	encrypted, err := Encrypt(data, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	_, err = Decrypt(encrypted, wrongKey)
	if err == nil {
		t.Error("Expected Decrypt to fail with wrong key")
	}
}

func TestDecrypt_InvalidBase64(t *testing.T) {
	key := "test-encryption-key-32-bytes!"
	invalidBase64 := "not-valid-base64!!!"

	_, err := Decrypt(invalidBase64, key)
	if err == nil {
		t.Error("Expected Decrypt to fail with invalid base64")
	}
}

func TestDecrypt_EmptyString(t *testing.T) {
	key := "test-encryption-key-32-bytes!"

	_, err := Decrypt("", key)
	if err == nil {
		t.Error("Expected Decrypt to fail with empty string")
	}
}

func TestDecrypt_TooShort(t *testing.T) {
	key := "test-encryption-key-32-bytes!"

	_, err := Decrypt("dG9vc2hvcnQ=", key) // "tooshort" in base64
	if err == nil {
		t.Error("Expected Decrypt to fail with ciphertext too short")
	}
}

func TestCreateKey(t *testing.T) {
	// Test with key longer than 32 bytes
	longKey := "this-is-a-very-long-key-that-is-longer-than-32-characters"
	result := createKey(longKey)
	if len(result) != 32 {
		t.Errorf("createKey should return 32 bytes, got %d", len(result))
	}
	if result != longKey[:32] {
		t.Errorf("createKey should truncate to first 32 bytes")
	}
}

func TestCreateKey_ShortKey(t *testing.T) {
	shortKey := "short"
	result := createKey(shortKey)
	if len(result) != 32 {
		t.Errorf("createKey should return 32 bytes, got %d", len(result))
	}
	if result[:5] != shortKey {
		t.Errorf("createKey should start with original key")
	}
}

func TestCreateKey_Exact32Bytes(t *testing.T) {
	exactKey := "12345678901234567890123456789012"
	result := createKey(exactKey)
	if len(result) != 32 {
		t.Errorf("createKey should return 32 bytes, got %d", len(result))
	}
	if result != exactKey {
		t.Errorf("createKey should return exact key when already 32 bytes")
	}
}
