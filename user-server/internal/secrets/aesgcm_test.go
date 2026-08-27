package secrets

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func syncOnceReset() sync.Once { return sync.Once{} }

func resetGlobals() {
	gcmOnce = syncOnceReset()
	gcmAEAD = nil
}

func TestRoundTrip(t *testing.T) {
	resetGlobals()
	os.Setenv(envMasterKey, strings.Repeat("a", keyLen))
	if err := InitFromEnv(); err != nil {
		t.Fatalf("init: %v", err)
	}
	if !Ready() {
		t.Fatalf("expected Ready after init")
	}
	ct, err := EncryptString("hello-world")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !IsCiphertextFormat(ct) {
		t.Fatalf("ciphertext format check failed: len=%d", len(ct))
	}
	pt, err := DecryptString(ct)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if pt != "hello-world" {
		t.Fatalf("round trip mismatch: %s", pt)
	}
}

func TestInitMissing(t *testing.T) {
	// 单独跑：重置 once。重新设置短 key 验证 fail-fast。
	resetGlobals()
	os.Setenv(envMasterKey, "short")
	if err := InitFromEnv(); err == nil {
		t.Fatalf("expected ErrMasterKeyMissing")
	}
	if Ready() {
		t.Fatalf("expected NOT Ready after fail")
	}
	if _, err := EncryptString("x"); err == nil {
		t.Fatalf("Encrypt should fail when not Ready")
	}
	_ = atomic.LoadInt32
}

func TestInitWrongKeyThenRightKey(t *testing.T) {
	resetGlobals()
	os.Setenv(envMasterKey, "short")
	_ = InitFromEnv()
	resetGlobals()
	os.Setenv(envMasterKey, strings.Repeat("z", keyLen))
	if err := InitFromEnv(); err != nil {
		t.Fatalf("init with proper key failed: %v", err)
	}
	ct, err := EncryptString("ok")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if pt, _ := DecryptString(ct); pt != "ok" {
		t.Fatalf("decrypt mismatch")
	}
}

func TestTamperRejected(t *testing.T) {
	resetGlobals()
	os.Setenv(envMasterKey, strings.Repeat("k", keyLen))
	if err := InitFromEnv(); err != nil {
		t.Fatalf("init: %v", err)
	}
	ct, err := EncryptString("xyz")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	tampered := append([]byte{}, ct...)
	tampered[len(tampered)-1] ^= 0x01
	if _, err := Decrypt(tampered); err == nil {
		t.Fatalf("expected tamper rejection")
	}
}

func TestNonceUniqueness(t *testing.T) {
	resetGlobals()
	os.Setenv(envMasterKey, strings.Repeat("k", keyLen))
	if err := InitFromEnv(); err != nil {
		t.Fatalf("init: %v", err)
	}
	a, _ := EncryptString("repeat")
	b, _ := EncryptString("repeat")
	if bytes.Equal([]byte(a), []byte(b)) {
		t.Fatalf("expected unique ciphertext for same plaintext")
	}
}
