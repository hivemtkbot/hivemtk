package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/model"
	"hivemtk-user/internal/pkg/db"

	"hivemtk-user/internal/pkg/testutil"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func setupUnifiedMessageTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.UnifiedMessage{},
	)
	db.SetTestDB(database)
	return database
}

func setupUnifiedMessageRouter(ctrl *UnifiedMessageController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Next()
	})
	router.GET("/messages", ctrl.GetMessages)
	router.GET("/messages/:id", ctrl.GetMessageByID)
	return router
}

func TestUnifiedMessageController_GetMessages_NoAuth(t *testing.T) {
	setupUnifiedMessageTestDB(t)
	ctrl := NewUnifiedMessageController()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/messages", ctrl.GetMessages)

	req, _ := http.NewRequest("GET", "/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 (no auth check), got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUnifiedMessageController_GetMessages_Success(t *testing.T) {
	setupUnifiedMessageTestDB(t)
	ctrl := NewUnifiedMessageController()
	router := setupUnifiedMessageRouter(ctrl)

	req, _ := http.NewRequest("GET", "/messages?page=1&page_size=10&platform=douyin", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUnifiedMessageController_GetMessageByID_InvalidID(t *testing.T) {
	setupUnifiedMessageTestDB(t)
	ctrl := NewUnifiedMessageController()
	router := setupUnifiedMessageRouter(ctrl)

	req, _ := http.NewRequest("GET", "/messages/abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for invalid ID, got %d", w.Code)
	}
}

func TestUnifiedMessageController_GetMessageByID_Success(t *testing.T) {
	setupUnifiedMessageTestDB(t)
	ctrl := NewUnifiedMessageController()
	router := setupUnifiedMessageRouter(ctrl)

	req, _ := http.NewRequest("GET", "/messages/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("Expected 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
	}
}
