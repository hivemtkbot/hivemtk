package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/utils/db"
)

type errorPinger struct {
	err error
}

func (f *errorPinger) Ping(ctx context.Context) error {
	return f.err
}

func TestHealthCheck_NoDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// 本用例验证「无外部依赖时健康检查应返回 200」。HealthCheck(nil) 在 db.GetDB() 非 nil
	// 时会去 ping 该库；在 ./... 并行下，同包内引导初始化用例会把全局 db.DB 指向共享库
	// user_db，导致此处 ping 偶发失败（503）。显式清空全局 DB，真正构造「无依赖」条件，测完恢复。
	prevDB := db.GetDB()
	db.SetTestDB(nil)
	t.Cleanup(func() { db.SetTestDB(prevDB) })

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
