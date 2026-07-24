package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 2026-07-24 安全清理：移除 GetDefaultCredentials 端点后，
// 相关测试用例一并删除。该端点原本会返回 config 中的默认密码，
// 任何持有 JWT 的账号都能拿到，构成严重入侵面。
// 重构后：管理员配置只承载 UI 行为（登录页提示/自动登录开关），
// 真正的超管密码只能从 DB 查（InitAdmin 写入）。

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

// TestAdminConfigController_NoPasswordField 强约束：响应中绝不出现 password 字段
func TestAdminConfigController_NoPasswordField(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctrl := NewAdminConfigController()
	router := gin.New()
	router.GET("/admin/config", ctrl.GetAdminConfig)

	req, _ := http.NewRequest("GET", "/admin/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// 大小写不敏感匹配（防止未来有人在字段名/JSON key 上做文章）
	lowerBody := strings.ToLower(body)
	for _, kw := range []string{"password", "passwd"} {
		if strings.Contains(lowerBody, kw) {
			t.Errorf("Admin config response should not contain keyword %q, body: %s", kw, body)
		}
	}
}

func TestAdminConfigController_NewAdminConfigController(t *testing.T) {
	ctrl := NewAdminConfigController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}
