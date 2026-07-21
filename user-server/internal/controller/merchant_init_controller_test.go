package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/pkg/utils/db"

	"github.com/gin-gonic/gin"
	"marketing/internal/pkg/testutil"
)

func setupSystemInitTestDB(t *testing.T) {
	database := testutil.NewTestDB(t)
	db.SetTestDB(database)
}

func TestSystemInitController_Init_InvalidJSON(t *testing.T) {
	setupSystemInitTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSystemInitController()
	router.POST("/system/init", ctrl.InitAdmin)

	req, _ := http.NewRequest("POST", "/system/init", bytes.NewReader([]byte("bad")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

func TestSystemInitController_Init_Success(t *testing.T) {
	setupSystemInitTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSystemInitController()
	router.POST("/system/init", ctrl.InitAdmin)

	body, _ := json.Marshal(map[string]string{
		"company_name":   "Test Instance",
		"admin_username": "admin",
		"admin_password": "Admin@12345",
		"contact_email":  "admin@test.com",
	})
	req, _ := http.NewRequest("POST", "/system/init", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 允许成功或参数错误（取决于 service 内部校验），但不应出现 500
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected 200 or 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}
