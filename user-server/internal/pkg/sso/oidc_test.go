package sso

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func mockOIDCServer(t *testing.T) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen rsa: %v", err)
	}

	var issuer atomic.Value
	issuer.Store("https://test-idp.example.com")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		iss := issuer.Load().(string)
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 iss,
				"authorization_endpoint": iss + "/oauth2/authorize",
				"token_endpoint":         iss + "/oauth2/token",
				"userinfo_endpoint":      iss + "/oauth2/userinfo",
				"jwks_uri":               iss + "/.well-known/jwks.json",
			})
		case "/.well-known/jwks.json":
			n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
			e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes())
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"keys": []map[string]string{
					{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": "test-kid-1", "n": n, "e": e},
				},
			})
		case "/oauth2/authorize":
			state := r.URL.Query().Get("state")
			http.Redirect(w, r, "https://hivemtk.example.com/api/sso/callback?code=test-code&state="+state, http.StatusFound)
		case "/oauth2/token":
			claims := jwt.MapClaims{
				"iss":                iss,
				"sub":                "user-123",
				"aud":                "test-client",
				"iat":                time.Now().Unix(),
				"exp":                time.Now().Add(time.Hour).Unix(),
				"email":              "alice@example.com",
				"email_verified":     true,
				"preferred_username": "alice",
				"name":               "Alice Smith",
				"roles":              []string{"admin", "user"},
			}
			tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
			tok.Header["kid"] = "test-kid-1"
			signed, err := tok.SignedString(priv)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-access",
				"id_token":     signed,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	issuer.Store(srv.URL)
	return srv, priv
}

func TestNewOIDCProvider_DefaultsFilled(t *testing.T) {
	p := NewOIDCProvider(OIDCConfig{
		Issuer:      "https://x",
		ClientID:    "c",
		RedirectURL: "https://app/cb",
	})
	if len(p.cfg.Scopes) == 0 {
		t.Fatal("scopes not filled")
	}
	if p.cfg.UsernameClaim != "preferred_username" {
		t.Errorf("UsernameClaim default not set: %s", p.cfg.UsernameClaim)
	}
	if p.cfg.DefaultRole != "user" {
		t.Errorf("DefaultRole default not set: %s", p.cfg.DefaultRole)
	}
	if p.cfg.HTTPTimeout == 0 {
		t.Errorf("HTTPTimeout not set")
	}
}

func TestPKCES256(t *testing.T) {
	v := "abcdefghijklmnopqrstuvwxyz123456"
	c := PKCES256(v)
	if len(c) == 0 {
		t.Fatal("pkce empty")
	}
	if strings.ContainsAny(c, "+/=") {
		t.Errorf("pkce not base64url-safe: %s", c)
	}
}

func TestAudienceClaim_Unmarshal(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"string", `"client"`, 1},
		{"array", `["a","b"]`, 2},
		{"empty", `null`, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var a AudienceClaim
			if err := json.Unmarshal([]byte(tt.in), &a); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(a) != tt.want {
				t.Errorf("len: got %d want %d", len(a), tt.want)
			}
		})
	}
}

func TestOIDCProvider_DiscoveryAndJWKS(t *testing.T) {
	srv, _ := mockOIDCServer(t)
	defer srv.Close()

	p := NewOIDCProvider(OIDCConfig{
		Issuer:      srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://hivemtk.example.com/api/sso/callback",
	})
	if err := p.ensureFresh(testContext()); err != nil {
		t.Fatalf("ensureFresh: %v", err)
	}
	if p.discovery == nil {
		t.Fatal("discovery nil")
	}
	if p.jwks == nil {
		t.Fatal("jwks nil")
	}
	if p.discovery.AuthorizationEndpoint == "" {
		t.Error("authorization endpoint empty")
	}
	if p.discovery.JWKSURI == "" {
		t.Error("jwks uri empty")
	}
	if len(p.jwks.Keys) != 1 {
		t.Errorf("expected 1 jwk, got %d", len(p.jwks.Keys))
	}
}

func TestOIDCProvider_VerifyIDToken(t *testing.T) {
	srv, priv := mockOIDCServer(t)
	defer srv.Close()

	p := NewOIDCProvider(OIDCConfig{
		Issuer:      srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://hivemtk.example.com/api/sso/callback",
	})
	if err := p.ensureFresh(testContext()); err != nil {
		t.Fatalf("ensureFresh: %v", err)
	}

	claims := jwt.MapClaims{
		"iss":                srv.URL,
		"sub":                "user-456",
		"aud":                "test-client",
		"iat":                time.Now().Unix(),
		"exp":                time.Now().Add(time.Hour).Unix(),
		"email":              "bob@example.com",
		"preferred_username": "bob",
		"roles":              []string{"editor"},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-kid-1"
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	got, err := p.verifyIDToken(testContext(), signed)
	if err != nil {
		t.Fatalf("verifyIDToken: %v", err)
	}
	if got.Subject != "user-456" {
		t.Errorf("sub: got %s", got.Subject)
	}
	if got.Email != "bob@example.com" {
		t.Errorf("email: got %s", got.Email)
	}
	if got.PreferredUsername != "bob" {
		t.Errorf("preferred_username: got %s", got.PreferredUsername)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "editor" {
		t.Errorf("roles: got %v", got.Roles)
	}
}

func TestOIDCProvider_VerifyIDToken_Expired(t *testing.T) {
	srv, priv := mockOIDCServer(t)
	defer srv.Close()

	p := NewOIDCProvider(OIDCConfig{
		Issuer:      srv.URL,
		ClientID:    "test-client",
		RedirectURL: "https://hivemtk.example.com/api/sso/callback",
	})
	if err := p.ensureFresh(testContext()); err != nil {
		t.Fatalf("ensureFresh: %v", err)
	}

	claims := jwt.MapClaims{
		"iss": srv.URL,
		"sub": "user-789",
		"aud": "test-client",
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = "test-kid-1"
	signed, _ := tok.SignedString(priv)

	_, err := p.verifyIDToken(testContext(), signed)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestUnsupportedCurve(t *testing.T) {
	_, err := curveFromName("secp256k1")
	if err == nil {
		t.Fatal("expected error for unsupported curve")
	}
	if _, ok := err.(*UnsupportedCurveError); !ok {
		t.Errorf("expected UnsupportedCurveError, got %T", err)
	}
}

func testContext() context.Context {
	return context.Background()
}
