package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("IS_TEST_MODE", "true") // Enable test mode to bypass remote auth
	defer os.Setenv("IS_TEST_MODE", "")

	tests := []struct {
		name           string
		apiKey         string
		expectedStatus int
	}{
		{
			name:           "With valid API key",
			apiKey:         "test-api-key-12345", // At least 16 chars
			expectedStatus: http.StatusOK,
		},
		{
			name:           "With empty API key",
			apiKey:         "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "With too short API key",
			apiKey:         "short",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(w)

			req, _ := http.NewRequest("GET", "http://test.com/test", nil)
			if tt.apiKey != "" {
				req.Header.Set("X-API-KEY", tt.apiKey)
			}
			ctx.Request = req

			middleware := AuthMiddleware()
			middleware(ctx)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestAuthMiddleware_WithNextHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	os.Setenv("IS_TEST_MODE", "true")
	defer os.Setenv("IS_TEST_MODE", "")

	w := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(w)

	req, _ := http.NewRequest("GET", "http://test.com/test", nil)
	req.Header.Set("X-API-KEY", "valid-api-key-12345")
	ctx.Request = req

	middleware := AuthMiddleware()
	middleware(ctx)

	// The middleware should have called c.Next() internally
	// We verify the response was successful
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
