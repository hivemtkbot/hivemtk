package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"marketing/internal/model"
	"marketing/internal/pkg/testutil"
	dbutil "marketing/internal/pkg/utils/db"
)

func setupTikTokAutoReplyRouter(ctrl *TikTokAutoReplyController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// 注入模拟用户上下文(避开鉴权中间件)
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	})
	group := router.Group("/api/v1")
	ctrl.RegisterRoutes(group)
	return router
}

// initAutoReplyTestDB 初始化自动回复测试数据库（注入全局 DB 供 service.NewXxxService 调用）
func initAutoReplyTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
	)
	dbutil.SetTestDB(database)
	return database
}

func TestTikTokAutoReplyController_GetAccounts_Success(t *testing.T) {
	initAutoReplyTestDB(t)
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	req, _ := http.NewRequest("GET", "/api/v1/tiktok/auto-reply/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTikTokAutoReplyController_GetRule_Success(t *testing.T) {
	initAutoReplyTestDB(t)
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	req, _ := http.NewRequest("GET", "/api/v1/tiktok/auto-reply/rule", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTikTokAutoReplyController_SaveRule_InvalidJSON(t *testing.T) {
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	req, _ := http.NewRequest("POST", "/api/v1/tiktok/auto-reply/rule", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestTikTokAutoReplyController_SaveRule_Success(t *testing.T) {
	initAutoReplyTestDB(t)
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	body, _ := json.Marshal(map[string]any{
		"name":       "Test Rule",
		"keywords":   "hello",
		"reply":      "world",
		"is_active":  true,
		"auto_reply": true,
	})
	req, _ := http.NewRequest("POST", "/api/v1/tiktok/auto-reply/rule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTikTokAutoReplyController_SaveCookies_InvalidJSON(t *testing.T) {
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	req, _ := http.NewRequest("POST", "/api/v1/tiktok/auto-reply/accounts/test-id/cookies", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestTikTokAutoReplyController_UpsertAccount_Success(t *testing.T) {
	initAutoReplyTestDB(t)
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	body, _ := json.Marshal(map[string]any{
		"username": "test_user",
		"cookie":   "test_cookie",
	})
	req, _ := http.NewRequest("POST", "/api/v1/tiktok/auto-reply/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestTikTokAutoReplyController_ListLogs_Success(t *testing.T) {
	initAutoReplyTestDB(t)
	ctrl := NewTikTokAutoReplyController()
	router := setupTikTokAutoReplyRouter(ctrl)

	req, _ := http.NewRequest("GET", "/api/v1/tiktok/auto-reply/logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}
