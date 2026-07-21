package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAdminConfigController_GetAdminConfig_Success 测试获取管理员配置成功
func TestAdminConfigController_GetAdminConfig_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAdminConfigController()
	router := gin.New()
	router.GET("/admin/config", ctrl.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	if response["code"] != "SUCCESS" {
		t.Errorf("Expected code SUCCESS, got %v", response["code"])
	}
}

// TestAdminConfigController_GetAdminConfig_ReturnsConfigInfo 测试返回配置信息
func TestAdminConfigController_GetAdminConfig_ReturnsConfigInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAdminConfigController()
	router := gin.New()
	router.GET("/admin/config", ctrl.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)

	data := response["data"].(map[string]any)
	if data["login"] == nil {
		t.Error("Expected login config to be present")
	}
	if data["auto_login"] == nil {
		t.Error("Expected auto_login config to be present")
	}
}

// TestAdminConfigController_GetDefaultCredentials_Success 测试获取默认凭据成功
func TestAdminConfigController_GetDefaultCredentials_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAdminConfigController()
	router := gin.New()
	router.GET("/admin/credentials", ctrl.GetDefaultCredentials)

	req, _ := http.NewRequest("GET", "/admin/credentials", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 根据配置可能成功或返回 403
	if w.Code != http.StatusOK && w.Code != http.StatusForbidden {
		t.Errorf("Expected status OK or Forbidden, got %d", w.Code)
	}
}

// TestAdminConfigController_GetDefaultCredentials_Structure 测试返回数据结构
func TestAdminConfigController_GetDefaultCredentials_Structure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAdminConfigController()
	router := gin.New()
	router.GET("/admin/credentials", ctrl.GetDefaultCredentials)

	req, _ := http.NewRequest("GET", "/admin/credentials", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		var response map[string]any
		json.Unmarshal(w.Body.Bytes(), &response)

		data := response["data"].(map[string]any)
		if data["username"] == "" {
			t.Error("Expected username to be present")
		}
		if data["password"] == "" {
			t.Error("Expected password to be present")
		}
	}
}

// TestAdminConfigController_NewAdminConfigController 测试构造函数
func TestAdminConfigController_NewAdminConfigController(t *testing.T) {
	ctrl := NewAdminConfigController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
