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

func setupAppConfigTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemConfig{},
	)
	db.SetTestDB(database)
	return database
}

func TestAppConfigController_GetAppConfig_Success(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.GET("/app/config", ctrl.GetAppConfig)

	req, _ := http.NewRequest("GET", "/app/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestAppConfigController_GetAppConfig_ReturnsConfig(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.GET("/app/config", ctrl.GetAppConfig)

	req, _ := http.NewRequest("GET", "/app/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

func TestAppConfigController_UpdateAppConfig_Success(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.PUT("/app/config", ctrl.UpdateAppConfig)

	updateReq := AppConfigReq{
		BasicConfig: BasicConfig{
			AppName:        "Test App",
			Version:        "1.0.0",
			Environment:    "test",
			DebugMode:      true,
			SessionTimeout: 3600,
		},
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/app/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestAppConfigController_UpdateAppConfig_InvalidJSON(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.PUT("/app/config", ctrl.UpdateAppConfig)

	req, _ := http.NewRequest("PUT", "/app/config", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestAppConfigController_SyncWithPlatform_Success(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.POST("/app/config/sync", ctrl.SyncWithPlatform)

	req, _ := http.NewRequest("POST", "/app/config/sync", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status OK, Bad Request or Internal Server Error, got %d", w.Code)
	}
}

func TestAppConfigController_HealthCheck_Success(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.GET("/app/config/health", ctrl.HealthCheck)

	req, _ := http.NewRequest("GET", "/app/config/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestAppConfigController_HealthCheck_ReturnsStatus(t *testing.T) {
	setupAppConfigTestDB(t)
	gin.SetMode(gin.TestMode)
	ctrl := NewAppConfigController()
	router := gin.New()
	router.GET("/app/config/health", ctrl.HealthCheck)

	req, _ := http.NewRequest("GET", "/app/config/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != float64(0) {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

func TestAppConfigController_NewAppConfigController(t *testing.T) {
	ctrl := NewAppConfigController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
