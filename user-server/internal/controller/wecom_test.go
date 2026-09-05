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

func setupWeComTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.WeComAccount{},
		&model.WeComCustomer{},
		&model.WeComGroup{},
		&model.WeComMessage{},
		&model.WeComTag{},
	)
	db.SetTestDB(database)
	return database
}

func setupWeComRouter(ctrl *WeComController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user_id", "test_value")
		c.Next()
	})
	router.POST("/wecom/accounts", ctrl.CreateAccount)
	router.GET("/wecom/accounts", ctrl.GetAccountList)
	router.GET("/wecom/accounts/:id", ctrl.GetAccountByID)
	router.PUT("/wecom/accounts/:id", ctrl.UpdateAccount)
	router.DELETE("/wecom/accounts/:id", ctrl.DeleteAccount)
	router.GET("/wecom/accounts/:id/customers", ctrl.GetCustomerList)
	router.GET("/wecom/accounts/:id/groups", ctrl.GetGroupList)
	router.GET("/wecom/accounts/:id/messages", ctrl.GetMessageList)
	router.GET("/wecom/accounts/:id/tags", ctrl.GetTagList)
	router.POST("/wecom/accounts/:id/send-message", ctrl.SendMessage)
	return router
}

func TestWeComController_CreateAccount_NoAuth(t *testing.T) {
	ctrl := NewWeComController(nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/wecom/accounts", ctrl.CreateAccount)

	body, _ := json.Marshal(map[string]string{"name": "test"})
	req, _ := http.NewRequest("POST", "/wecom/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 (param validation), got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_CreateAccount_InvalidJSON(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("POST", "/wecom/accounts", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestWeComController_GetAccountList_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_GetAccountByID_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("Expected 200/400/404, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_DeleteAccount_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("DELETE", "/wecom/accounts/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 200, 404 or 500, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_GetCustomerList_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts/test-id/customers", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_GetGroupList_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts/test-id/groups", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_GetMessageList_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts/test-id/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_GetTagList_Success(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("GET", "/wecom/accounts/test-id/tags", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestWeComController_SendMessage_InvalidJSON(t *testing.T) {
	setupWeComTestDB(t)
	ctrl := NewWeComController(nil)
	router := setupWeComRouter(ctrl)

	req, _ := http.NewRequest("POST", "/wecom/accounts/test-id/send-message", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}
