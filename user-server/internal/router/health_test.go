package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type errorPinger struct {
	err error
}

func (f *errorPinger) Ping(ctx context.Context) error {
	return f.err
}

func TestHealthCheck_NoDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", HealthCheck(nil))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHealthCheck_RedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", HealthCheck(&errorPinger{err: errors.New("connection refused")}))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Redis 不可用但数据库不可用时应该降级
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusOK {
		t.Errorf("expected 200 or 503, got %d", w.Code)
	}
}

func TestLivenessCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", LivenessCheck())

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestReadinessCheck_NoDB(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", ReadinessCheck(nil))

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 没有数据库时应该是 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when no DB, got %d", w.Code)
	}
}
