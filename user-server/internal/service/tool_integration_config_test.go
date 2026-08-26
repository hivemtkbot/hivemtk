package service

import (
	"strings"
	"testing"
)

func setMasterKey(t *testing.T, v string) {
	t.Helper()
	t.Setenv("MASTER_KEY", v)
}

// TestToolIntegration_EncryptDecryptRoundTrip 加解密 round-trip
func TestToolIntegration_EncryptDecryptRoundTrip(t *testing.T) {
	setMasterKey(t, "unit-test-master-key")

	plain := "sk-live-abcdef-123456"
	enc, err := encryptField(plain)
	if err != nil {
		t.Fatalf("encryptField: %v", err)
	}
	if !strings.HasPrefix(enc, "enc:v1:") {
		t.Errorf("ciphertext should have enc:v1: prefix, got %s", enc[:min(16, len(enc))])
	}
	if enc == plain {
		t.Error("ciphertext must differ from plaintext")
	}
	got, err := decryptField(enc)
	if err != nil {
		t.Fatalf("decryptField: %v", err)
	}
	if got != plain {
		t.Errorf("round-trip mismatch: got %q, want %q", got, plain)
	}
	if !isEncryptedField(enc) {
		t.Error("isEncryptedField should detect prefix")
	}
}

// TestToolIntegration_EncryptionIsRandomized 同一明文两次加密产生不同密文（随机 nonce）
func TestToolIntegration_EncryptionIsRandomized(t *testing.T) {
	setMasterKey(t, "another-master-key")
	e1, _ := encryptField("same-secret")
	e2, _ := encryptField("same-secret")
	if e1 == e2 {
		t.Error("nonce randomization should yield different ciphertexts")
	}
	d1, _ := decryptField(e1)
	d2, _ := decryptField(e2)
	if d1 != "same-secret" || d2 != "same-secret" {
		t.Error("both ciphertexts should decrypt to original")
	}
}

// TestToolIntegration_PlaintextPassthroughAndUpgrade 明文照常返回，可透明升级为密文
func TestToolIntegration_PlaintextPassthroughAndUpgrade(t *testing.T) {
	setMasterKey(t, "upgrade-key")

	plain := "legacy-plaintext-key"
	got, err := decryptField(plain) // 非enc前缀→原样返回
	if err != nil {
		t.Fatalf("decryptField on plaintext: %v", err)
	}
	if got != plain {
		t.Errorf("plaintext passthrough broken: got %q", got)
	}

	cfg := &ToolIntegrationConfig{
		Logistics: LogisticsIntegration{Enabled: true, Key: "k1", Secret: "s1"},
		AfterSale: AfterSaleIntegration{Enabled: false},
	}
	if !hasPlaintextSecretFields(cfg) {
		t.Fatal("should detect plaintext secret fields")
	}
	encCfg, err := encryptConfigFields(cfg)
	if err != nil {
		t.Fatalf("encryptConfigFields: %v", err)
	}
	if !strings.HasPrefix(encCfg.Logistics.Key, "enc:v1:") || !strings.HasPrefix(encCfg.Logistics.Secret, "enc:v1:") {
		t.Error("fields should be encrypted after upgrade")
	}
	// 入参不被修改（写库存副本）
	if cfg.Logistics.Key != "k1" || cfg.Logistics.Secret != "s1" {
		t.Errorf("input cfg must not be mutated: %+v", cfg.Logistics)
	}
	// 解密回读
	if err := decryptConfigFields(encCfg); err != nil {
		t.Fatalf("decryptConfigFields: %v", err)
	}
	if encCfg.Logistics.Key != "k1" || encCfg.Logistics.Secret != "s1" {
		t.Errorf("decrypt round-trip mismatch: %+v", encCfg.Logistics)
	}
}

// TestToolIntegration_NoMasterKeyDegrade 未设置 MASTER_KEY 时降级：明文读写、不报错
func TestToolIntegration_NoMasterKeyDegrade(t *testing.T) {
	t.Setenv("MASTER_KEY", "")

	v, err := encryptField("plain-value")
	if err != nil {
		t.Fatalf("degrade mode must not error: %v", err)
	}
	if v != "plain-value" {
		t.Errorf("degrade mode should store plaintext, got %q", v)
	}
	back, err := decryptField(v)
	if err != nil || back != "plain-value" {
		t.Errorf("degrade read failed: %q %v", back, err)
	}

	cfg := &ToolIntegrationConfig{Logistics: LogisticsIntegration{Key: "k", Secret: "s"}}
	encCfg, err := encryptConfigFields(cfg)
	if err != nil {
		t.Fatalf("save path in degrade mode must not error: %v", err)
	}
	if encCfg.Logistics.Key != "k" || encCfg.Logistics.Secret != "s" {
		t.Errorf("degrade save should keep plaintext: %+v", encCfg.Logistics)
	}
}

// TestToolIntegration_TamperedCiphertextError 篡改密文必须报错
func TestToolIntegration_TamperedCiphertextError(t *testing.T) {
	setMasterKey(t, "tamper-test-key")

	enc, err := encryptField("sensitive-data")
	if err != nil {
		t.Fatalf("encryptField: %v", err)
	}

	// 1. 篡改 base64 payload 字节
	raw := strings.TrimPrefix(enc, "enc:v1:")
	mutated := []byte(raw)
	mutated[len(mutated)-1] ^= 0x01
	if _, err := decryptField("enc:v1:" + string(mutated)); err == nil {
		t.Error("tampered ciphertext must fail to decrypt")
	}

	// 2. 换错密钥解密
	t.Setenv("MASTER_KEY", "a-different-key")
	if _, err := decryptField(enc); err == nil {
		t.Error("decryption under wrong key must fail")
	} else if !strings.Contains(err.Error(), "tampered or wrong key") {
		t.Errorf("error should be explicit: %v", err)
	}
	t.Setenv("MASTER_KEY", "tamper-test-key")

	// 3. 非法 base64
	if _, err := decryptField("enc:v1:not-base64!!!"); err == nil {
		t.Error("invalid base64 must error")
	}

	// 4. payload 过短
	if _, err := decryptField("enc:v1:" + "AAAA"); err == nil {
		t.Error("too-short payload must error")
	}
}

// TestToolIntegration_DecryptWrongKeyExplicitError 密钥轮换后旧密文报明确错误
func TestToolIntegration_DecryptWrongKeyExplicitError(t *testing.T) {
	setMasterKey(t, "key-A")
	enc, _ := encryptField("data")

	setMasterKey(t, "key-B")
	if _, err := decryptField(enc); err == nil {
		t.Error("old ciphertext under new key must error")
	}
}
