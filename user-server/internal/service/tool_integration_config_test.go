package service

import (
	"strings"
	"testing"

	"hivemtk-user/internal/secrets"
)

const testMasterKey = "0123456789abcdef0123456789abcdef"

func resetSecretsForTest(t *testing.T, key string) {
	t.Helper()
	secrets.ResetForTest()
	t.Setenv("MASTER_KEY", key)
	if key != "" {
		if err := secrets.InitFromEnv(); err != nil {
			t.Fatalf("secrets.InitFromEnv: %v", err)
		}
	}
}

func TestToolIntegration_EncryptDecryptRoundTrip(t *testing.T) {
	resetSecretsForTest(t, testMasterKey)

	plain := "sk-live-abcdef-123456"
	enc, err := secrets.EncryptString(plain)
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}
	if !strings.HasPrefix(enc, "enc:v1:") {
		t.Errorf("ciphertext should have enc:v1: prefix, got %s", enc[:min(16, len(enc))])
	}
	if enc == plain {
		t.Error("ciphertext must differ from plaintext")
	}
	got, err := secrets.DecryptString(enc)
	if err != nil {
		t.Fatalf("DecryptString: %v", err)
	}
	if got != plain {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plain)
	}
	if !secrets.IsCiphertextFormat(enc) {
		t.Error("IsCiphertextFormat should detect prefix")
	}
	if secrets.IsCiphertextFormat(plain) {
		t.Error("IsCiphertextFormat should NOT detect plaintext")
	}
}

func TestToolIntegration_EncryptionIsRandomized(t *testing.T) {
	resetSecretsForTest(t, testMasterKey)

	e1, _ := secrets.EncryptString("same-secret")
	e2, _ := secrets.EncryptString("same-secret")
	if e1 == e2 {
		t.Error("nonce randomization should yield different ciphertexts")
	}
	d1, _ := secrets.DecryptString(e1)
	d2, _ := secrets.DecryptString(e2)
	if d1 != "same-secret" || d2 != "same-secret" {
		t.Error("both ciphertexts should decrypt to original")
	}
}

func TestToolIntegration_PlaintextPassthroughAndUpgrade(t *testing.T) {
	resetSecretsForTest(t, testMasterKey)

	plain := "legacy-plaintext-key"
	got, err := secrets.DecryptString(plain)
	if err != nil {
		t.Fatalf("DecryptString on plaintext: %v", err)
	}
	if got != plain {
		t.Errorf("plaintext passthrough broken: got %q", got)
	}

	cfg := &ToolIntegrationConfig{
		Logistics: LogisticsIntegration{Enabled: true, Key: "k1", Secret: "s1"},
		AfterSale: AfterSaleIntegration{Enabled: false},
	}
	if !cfg.hasPlaintextCredentials() {
		t.Fatal("should detect plaintext credential fields")
	}

	if err := encryptCredentialFields(cfg); err != nil {
		t.Fatalf("encryptCredentialFields: %v", err)
	}
	if !strings.HasPrefix(cfg.Logistics.Key, "enc:v1:") || !strings.HasPrefix(cfg.Logistics.Secret, "enc:v1:") {
		t.Error("fields should be encrypted after upgrade")
	}

	for _, p := range cfg.credentialFields() {
		*p, err = secrets.DecryptString(*p)
		if err != nil {
			t.Fatalf("decrypt credential: %v", err)
		}
	}
	if cfg.Logistics.Key != "k1" || cfg.Logistics.Secret != "s1" {
		t.Errorf("decrypt round-trip mismatch: %+v", cfg.Logistics)
	}
}

func TestToolIntegration_NoMasterKeyDegrade(t *testing.T) {
	resetSecretsForTest(t, "")

	if secrets.Ready() {
		t.Fatal("secrets must not be ready without MASTER_KEY")
	}

	cfg := &ToolIntegrationConfig{Logistics: LogisticsIntegration{Key: "k", Secret: "s"}}
	if err := encryptCredentialFields(cfg); err == nil {
		t.Error("encryptCredentialFields without MASTER_KEY must error")
	}
}

func TestToolIntegration_TamperedCiphertextError(t *testing.T) {
	resetSecretsForTest(t, testMasterKey)

	enc, err := secrets.EncryptString("sensitive-data")
	if err != nil {
		t.Fatalf("EncryptString: %v", err)
	}

	raw := strings.TrimPrefix(enc, "enc:v1:")
	mutated := []byte(raw)
	mutated[len(mutated)-1] ^= 0x01
	if _, err := secrets.DecryptString("enc:v1:" + string(mutated)); err == nil {
		t.Error("tampered ciphertext must fail to decrypt")
	}

	resetSecretsForTest(t, "0123456789abcdef0123456789abcdee")
	if _, err := secrets.DecryptString(enc); err == nil {
		t.Error("decryption under wrong key must fail")
	}
	resetSecretsForTest(t, testMasterKey)

	if _, err := secrets.DecryptString("enc:v1:not-base64!!!"); err == nil {
		t.Error("invalid base64 must error")
	}
}

func TestToolIntegration_DecryptWrongKeyExplicitError(t *testing.T) {
	resetSecretsForTest(t, "0123456789abcdef0123456789abcdef")
	enc, _ := secrets.EncryptString("data")

	resetSecretsForTest(t, "abcdef0123456789abcdef0123456789")
	if _, err := secrets.DecryptString(enc); err == nil {
		t.Error("old ciphertext under new key must error")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
