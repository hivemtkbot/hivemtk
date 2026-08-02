package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/config"
)

// TestClient_Do_401SelfHeal 验证遇 401 时清空 token 并重试一次，最终成功（V6 自愈）。
func TestClient_Do_401SelfHeal(t *testing.T) {
	t.Setenv("MERCHANT_API_SECRET", "test-secret")
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":401,"msg":"unauthorized"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(BaseResp{Code: 0, Msg: "ok"})
	}))
	defer srv.Close()

	config.PlatformCfg = &config.PlatformConfig{APIURL: srv.URL}
	c := NewPlatformClient("test-key")

	var resp BaseResp
	if err := c.Do("GET", "/test", nil, &resp); err != nil {
		t.Fatalf("expected success after 401 retry, got: %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected 2 hits (1x401 + 1x200), got %d", hits)
	}
}

// TestClient_Do_StructuredError 验证非 2xx 返回结构化 *PlatformError，透传状态码与业务 msg。
func TestClient_Do_StructuredError(t *testing.T) {
	t.Setenv("MERCHANT_API_SECRET", "test-secret")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"msg":"not found"}`))
	}))
	defer srv.Close()

	config.PlatformCfg = &config.PlatformConfig{APIURL: srv.URL}
	c := NewPlatformClient("test-key")

	var resp BaseResp
	err := c.Do("GET", "/missing", nil, &resp)
	perr, ok := err.(*PlatformError)
	if !ok {
		t.Fatalf("expected *PlatformError, got %T: %v", err, err)
	}
	if perr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", perr.StatusCode)
	}
	if perr.Msg() != "not found" {
		t.Fatalf("expected msg 'not found', got %q", perr.Msg())
	}
}
