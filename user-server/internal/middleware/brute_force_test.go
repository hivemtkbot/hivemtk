package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBruteForceGuard_NoLockOnRequestAlone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetBruteForceForTest()
	defer ResetBruteForceForTest()

	calls := 0
	r := gin.New()
	r.Use(func(c *gin.Context) { calls++; c.Next() })
	r.Use(BruteForceGuard("test.endpoint"))
	r.GET("/x", func(c *gin.Context) { c.Status(200) })

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d should pass, got %d", i, w.Code)
		}
	}
	if calls != 10 {
		t.Fatalf("expected 10 successful passes, got %d", calls)
	}
}

func TestBruteForceGuard_LocksAfterRecordFailures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetBruteForceForTest()
	defer ResetBruteForceForTest()

	r := gin.New()
	r.Use(BruteForceGuard("test.endpoint"))
	r.GET("/x", func(c *gin.Context) {
		RecordBruteForceFailure(c, "test.endpoint")
		c.Status(http.StatusUnauthorized)
	})

	ip := "192.0.2.2:1234"

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d should reach handler, got %d", i, w.Code)
		}
	}

	req := httptest.NewRequest("GET", "/x", nil)
	req.RemoteAddr = ip
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6 should be locked (429), got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header should be set")
	}
}

func TestBruteForceGuard_ClearResetsLock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetBruteForceForTest()
	defer ResetBruteForceForTest()

	r := gin.New()
	r.Use(BruteForceGuard("test.endpoint"))
	hitHandler := false
	r.GET("/x", func(c *gin.Context) {
		hitHandler = true
		ClearBruteForceFailure(c, "test.endpoint")
		c.Status(http.StatusOK)
	})

	ip := "192.0.2.3:1234"

	for i := 0; i < 6; i++ {
		req := httptest.NewRequest("GET", "/x", nil)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("clear-loop attempt %d should be 200, got %d", i, w.Code)
		}
	}
	if !hitHandler {
		t.Fatal("handler should be reached")
	}
}
