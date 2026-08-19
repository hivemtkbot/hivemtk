package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"
)

func TestGenerateVisitorToken(t *testing.T) {
	secret := "test-secret-key-32chars-long-enough"
	token, err := GenerateVisitorToken(secret, "ch1", "v1", "s1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
	// Verify format: should contain "."
	if len(token) < 20 {
		t.Fatalf("token too short: %s", token)
	}
}

func TestGenerateVisitorToken_EmptyParams(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		channelID string
		visitorID string
		sessionID string
	}{
		{"empty secret", "", "ch1", "v1", "s1"},
		{"empty channelID", "secret", "", "v1", "s1"},
		{"empty visitorID", "secret", "ch1", "", "s1"},
		{"empty sessionID", "secret", "ch1", "v1", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GenerateVisitorToken(tt.secret, tt.channelID, tt.visitorID, tt.sessionID, 0)
			if err == nil {
				t.Fatal("expected error for empty params")
			}
		})
	}
}

func TestGenerateAndValidate_RoundTrip(t *testing.T) {
	secret := "test-secret-for-round-trip-32bytes!!"
	token, err := GenerateVisitorToken(secret, "channel-1", "visitor-42", "session-99", 1*time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Valid token should pass
	if err := ValidateVisitorToken(secret, token, "channel-1", "visitor-42", "session-99"); err != nil {
		t.Fatalf("validate failed: %v", err)
	}
}

func TestValidateVisitorToken_WrongChannel(t *testing.T) {
	secret := "test-secret-for-round-trip-32bytes!!"
	token, err := GenerateVisitorToken(secret, "channel-1", "visitor-42", "session-99", 1*time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Wrong channel should fail
	if err := ValidateVisitorToken(secret, token, "channel-2", "visitor-42", "session-99"); err == nil {
		t.Fatal("expected error for wrong channel")
	}
}

func TestValidateVisitorToken_WrongVisitor(t *testing.T) {
	secret := "test-secret-for-round-trip-32bytes!!"
	token, err := GenerateVisitorToken(secret, "channel-1", "visitor-42", "session-99", 1*time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Wrong visitor should fail
	if err := ValidateVisitorToken(secret, token, "channel-1", "visitor-99", "session-99"); err == nil {
		t.Fatal("expected error for wrong visitor")
	}
}

func TestValidateVisitorToken_WrongSecret(t *testing.T) {
	secret := "test-secret-for-round-trip-32bytes!!"
	token, err := GenerateVisitorToken(secret, "channel-1", "visitor-42", "session-99", 1*time.Hour)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Wrong secret should fail
	if err := ValidateVisitorToken("wrong-secret-key-here-32bytes!!", token, "channel-1", "visitor-42", "session-99"); err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestValidateVisitorToken_Expired(t *testing.T) {
	secret := "test-secret-for-round-trip-32bytes!!"
	// 手动构造一个已过期的 token（使用过去的时间戳）
	pastExpireTS := time.Now().Add(-1 * time.Hour).Unix()
	payload := fmt.Sprintf("channel-1|visitor-42|session-99|%d", pastExpireTS)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	sig := hex.EncodeToString(mac.Sum(nil))
	token := fmt.Sprintf("%s.%d", sig, pastExpireTS)

	if err := ValidateVisitorToken(secret, token, "channel-1", "visitor-42", "session-99"); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateVisitorToken_EmptyToken(t *testing.T) {
	if err := ValidateVisitorToken("secret", "", "c", "v", "s"); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestValidateVisitorToken_InvalidFormat(t *testing.T) {
	if err := ValidateVisitorToken("secret", "invalid-token-no-dot", "c", "v", "s"); err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestValidateVisitorToken_DefaultTTL(t *testing.T) {
	secret := "test-secret-default-ttl-32characters!!"
	token, err := GenerateVisitorToken(secret, "ch", "v", "s", 0)
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Default TTL is 7 days, so it should pass validation
	if err := ValidateVisitorToken(secret, token, "ch", "v", "s"); err != nil {
		t.Fatalf("validate with default TTL failed: %v", err)
	}
}