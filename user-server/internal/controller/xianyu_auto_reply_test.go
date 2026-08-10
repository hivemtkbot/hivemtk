package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupXianyuTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.AutoReplyAccount{},
		&model.AutoReplyRule{},
		&model.AutoReplyLog{},
	)
	db.SetTestDB(database)
	return database
}

func setupXianyuRouter(ctrl *XianyuAutoReplyController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", uint(1))
		c.Next()
	})
	router.GET("/xianyu-auto-reply/accounts", ctrl.ListAccounts)
	router.GET("/xianyu-auto-reply/rule", ctrl.GetRule)
	router.POST("/xianyu-auto-reply/rule", ctrl.SaveRule)
	router.GET("/xianyu-auto-reply/logs", ctrl.ListLogs)
	return router
}

func TestXianyuAutoReplyController_ListAccounts_Success(t *testing.T) {
	setupXianyuTestDB(t)
	ctrl := NewXianyuAutoReplyController(nil)
	router := setupXianyuRouter(ctrl)

	req, _ := http.NewRequest("GET", "/xianyu-auto-reply/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestXianyuAutoReplyController_GetRule_Success(t *testing.T) {
	setupXianyuTestDB(t)
	ctrl := NewXianyuAutoReplyController(nil)
	router := setupXianyuRouter(ctrl)

	req, _ := http.NewRequest("GET", "/xianyu-auto-reply/rule", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestXianyuAutoReplyController_SaveRule_InvalidJSON(t *testing.T) {
	setupXianyuTestDB(t)
	ctrl := NewXianyuAutoReplyController(nil)
	router := setupXianyuRouter(ctrl)

	req, _ := http.NewRequest("POST", "/xianyu-auto-reply/rule", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestXianyuAutoReplyController_SaveRule_Success(t *testing.T) {
	setupXianyuTestDB(t)
	ctrl := NewXianyuAutoReplyController(nil)
	router := setupXianyuRouter(ctrl)

	body, _ := json.Marshal(map[string]any{
		"keywords":      "价格,优惠",
		"reply_content": "亲，价格是...",
		"frequency":     5,
		"daily_limit":   50,
		"is_active":     true,
	})
	req, _ := http.NewRequest("POST", "/xianyu-auto-reply/rule", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestXianyuAutoReplyController_ListLogs_Success(t *testing.T) {
	setupXianyuTestDB(t)
	ctrl := NewXianyuAutoReplyController(nil)
	router := setupXianyuRouter(ctrl)

	req, _ := http.NewRequest("GET", "/xianyu-auto-reply/logs?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}
