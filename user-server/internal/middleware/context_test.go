package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestContextMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req.Header.Set("User-Agent", "Test-Agent/1.0")
	req.RemoteAddr = "192.168.1.1:12345"
	ctx.Request = req

	middleware := ContextMiddleware()
	middleware(ctx)

	ip, exists := ctx.Get("ip")
	if !exists {
		t.Error("Expected 'ip' to be set in context")
	}
	if ip == "" {
		t.Error("Expected non-empty IP")
	}

	userAgent, exists := ctx.Get("user_agent")
	if !exists {
		t.Error("Expected 'user_agent' to be set in context")
	}
	if userAgent != "Test-Agent/1.0" {
		t.Errorf("Expected User-Agent 'Test-Agent/1.0', got %s", userAgent)
	}
}

func TestContextMiddleware_NoUserAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req.RemoteAddr = "10.0.0.1:8080"
	ctx.Request = req

	middleware := ContextMiddleware()
	middleware(ctx)

	if _, exists := ctx.Get("ip"); !exists {
		t.Error("Expected 'ip' to be set in context")
	}

	userAgent, exists := ctx.Get("user_agent")
	if !exists {
		t.Error("Expected 'user_agent' to be set in context")
	}
	if userAgent != "" {
		t.Errorf("Expected empty User-Agent, got %s", userAgent)
	}
}

