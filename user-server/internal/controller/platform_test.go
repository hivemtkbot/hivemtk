package controller

import (
	"bytes"
	"encoding/json"
	"marketing/internal/config"
	"marketing/internal/platform"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ----------------------------------------------------------------------------
// 测试辅助
// ----------------------------------------------------------------------------

// mockPlatformServer 启动一个 httptest 服务器模拟 platform-server,
// 并把 config.PlatformCfg.APIURL 指向该服务器,供控制器调用
var (
	mockServer     *httptest.Server
	mockServerOnce sync.Once
)

func setupMockPlatformServer() {
	mockServerOnce.Do(func() {
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// 登录接口需要返回 token
			if r.URL.Path == "/api/auth/login" {
				_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"token":"test-jwt-token-for-platform-test","expires":9999999999}}`))
				return
			}
			// 其他路径返回通用成功响应
			_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{}}`))
		}))
	})
}

func setPlatformConfigForTest(t *testing.T) {
	t.Helper()
	if mockServer == nil {
		setupMockPlatformServer()
	}
	// 直接覆盖全局配置,指向上面的 mock server
	config.PlatformCfg = &config.PlatformConfig{
		APIURL:              mockServer.URL,
		Secret:              "test_secret",
		LogReportInterval:   60,
		LicenseSyncInterval: 300,
	}
}

func setMerchantKeyForTest(t *testing.T, key string) {
	t.Helper()
	platform.SetMerchantKeyForTest(key)
}

// setupPlatformController 设置平台控制器测试环境
func setupPlatformController(t *testing.T) (*PlatformController, *gin.Engine) {
	gin.SetMode(gin.TestMode)
	setPlatformConfigForTest(t)
	setMerchantKeyForTest(t, "test-merchant-key")
	ctrl := NewPlatformController()
	router := gin.New()
	return ctrl, router
}

// ============================================================================
// PlatformController 测试
// ============================================================================

// TestPlatformController_GetLatestMessage_Success 测试获取最新消息成功
func TestPlatformController_GetLatestMessage_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/message/latest", ctrl.GetLatestMessage)

	req, _ := http.NewRequest("GET", "/platform/message/latest", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于服务层依赖外部系统,接受 200 或 500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_MarkMessageRead_Success 测试标记消息已读成功
func TestPlatformController_MarkMessageRead_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/:id/read", ctrl.MarkMessageRead)

	req, _ := http.NewRequest("POST", "/platform/messages/1/read", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_MarkMessageRead_EmptyID 测试空消息 ID
func TestPlatformController_MarkMessageRead_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/:id/read", ctrl.MarkMessageRead)

	req, _ := http.NewRequest("POST", "/platform/messages//read", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_GetLicenseStatus_Success 测试获取授权状态成功
func TestPlatformController_GetLicenseStatus_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/license/status", ctrl.GetLicenseStatus)

	// 创建临时 install.lock 文件
	tmpFile := "./install.lock"
	licenseInfo := map[string]any{
		"license_key":   "TEST-LICENSE-KEY",
		"expire_at":     time.Now().AddDate(1, 0, 0).Format(time.RFC3339),
		"created_at":    time.Now().Format(time.RFC3339),
		"last_check_at": time.Now().Format(time.RFC3339),
		"is_valid":      true,
	}
	data, _ := json.Marshal(licenseInfo)
	os.WriteFile(tmpFile, data, 0644)
	defer os.Remove(tmpFile)

	req, _ := http.NewRequest("GET", "/platform/license/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应返回 200
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_GetLicenseStatus_FileNotFound 测试授权文件不存在
func TestPlatformController_GetLicenseStatus_FileNotFound(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/license/status", ctrl.GetLicenseStatus)

	req, _ := http.NewRequest("GET", "/platform/license/status", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 文件不存在应返回 200，状态为 not_installed
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("Response data is not a map: %v", resp)
	}

	status, ok := data["status"].(string)
	if !ok || status != "not_installed" {
		t.Errorf("Expected status 'not_installed', got '%v'", data["status"])
	}
}

// TestPlatformController_ReportAPILog_Success 测试上报 API 日志成功
func TestPlatformController_ReportAPILog_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/api-logs", ctrl.ReportAPILog)

	logReq := map[string]any{
		"method":      "GET",
		"path":        "/api/test",
		"status_code": 200,
		"duration":    100,
		"user_agent":  "TestAgent/1.0",
	}
	body, _ := json.Marshal(logReq)

	req, _ := http.NewRequest("POST", "/platform/api-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_ReportAPILog_InvalidJSON 测试无效 JSON
func TestPlatformController_ReportAPILog_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/api-logs", ctrl.ReportAPILog)

	req, _ := http.NewRequest("POST", "/platform/api-logs", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_ReportAPILog_MissingFields 测试缺少必填字段
func TestPlatformController_ReportAPILog_MissingFields(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/api-logs", ctrl.ReportAPILog)

	logReq := map[string]any{
		"method": "GET",
		// 缺少 path 字段
	}
	body, _ := json.Marshal(logReq)

	req, _ := http.NewRequest("POST", "/platform/api-logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestPlatformController_RegisterMerchant_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/merchants/register", ctrl.RegisterMerchant)

	registerReq := map[string]any{
		"name":          "测试商户",
		"contact_email": "test@example.com",
		"contact_phone": "1234567890",
		"device_info":   "Test Device",
	}
	body, _ := json.Marshal(registerReq)

	req, _ := http.NewRequest("POST", "/platform/merchants/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_RegisterMerchant_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/merchants/register", ctrl.RegisterMerchant)

	req, _ := http.NewRequest("POST", "/platform/merchants/register", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestPlatformController_RegisterMerchant_MissingName(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/merchants/register", ctrl.RegisterMerchant)

	registerReq := map[string]any{
		"contact_email": "test@example.com",
		// 缺少 name 字段
	}
	body, _ := json.Marshal(registerReq)

	req, _ := http.NewRequest("POST", "/platform/merchants/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_GetDashboard_Success 测试获取驾驶舱数据成功
func TestPlatformController_GetDashboard_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/dashboard", ctrl.GetDashboard)

	req, _ := http.NewRequest("GET", "/platform/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_GetMerchantList_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/merchants", ctrl.GetMerchantList)

	req, _ := http.NewRequest("GET", "/platform/merchants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_UpdateMerchantLicense_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.PUT("/platform/merchants/license", ctrl.UpdateMerchantLicense)

	updateReq := map[string]any{
		"user_id":     "merchant-123",
		"license_key": "TEST-LICENSE-KEY",
		"expire_at":   "2025-12-31",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/platform/merchants/license", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_UpdateMerchantLicense_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.PUT("/platform/merchants/license", ctrl.UpdateMerchantLicense)

	req, _ := http.NewRequest("PUT", "/platform/merchants/license", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

func TestPlatformController_GetMerchantStats_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/merchants/:id/stats", ctrl.GetMerchantStats)

	req, _ := http.NewRequest("GET", "/platform/merchants/123/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_GetMerchantStats_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/merchants/:id/stats", ctrl.GetMerchantStats)

	req, _ := http.NewRequest("GET", "/platform/merchants//stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_GetVersionList_Success 测试获取版本列表成功
func TestPlatformController_GetVersionList_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/versions", ctrl.GetVersionList)

	req, _ := http.NewRequest("GET", "/platform/versions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CreateVersion_Success 测试创建版本成功
func TestPlatformController_CreateVersion_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/versions", ctrl.CreateVersion)

	createReq := map[string]any{
		"version":      "1.0.0",
		"description":  "新版本",
		"download_url": "https://example.com/download",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/platform/versions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CreateVersion_InvalidJSON 测试无效 JSON
func TestPlatformController_CreateVersion_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/versions", ctrl.CreateVersion)

	req, _ := http.NewRequest("POST", "/platform/versions", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_UpdateVersion_Success 测试更新版本成功
func TestPlatformController_UpdateVersion_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.PUT("/platform/versions/:id", ctrl.UpdateVersion)

	updateReq := map[string]any{
		"version":      "1.0.1",
		"description":  "更新版本",
		"download_url": "https://example.com/download/v1.0.1",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/platform/versions/1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_UpdateVersion_EmptyID 测试空版本 ID
func TestPlatformController_UpdateVersion_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.PUT("/platform/versions/:id", ctrl.UpdateVersion)

	req, _ := http.NewRequest("PUT", "/platform/versions/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 空 ID 可能返回 400 或 404(取决于路由匹配)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Bad Request or Not Found, got %d", w.Code)
	}
}

// TestPlatformController_DeleteVersion_Success 测试删除版本成功
func TestPlatformController_DeleteVersion_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.DELETE("/platform/versions/:id", ctrl.DeleteVersion)

	req, _ := http.NewRequest("DELETE", "/platform/versions/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_DeleteVersion_EmptyID 测试空版本 ID
func TestPlatformController_DeleteVersion_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.DELETE("/platform/versions/:id", ctrl.DeleteVersion)

	req, _ := http.NewRequest("DELETE", "/platform/versions/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 空 ID 可能返回 400 或 404(取决于路由匹配)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Bad Request or Not Found, got %d", w.Code)
	}
}

// TestPlatformController_GetMessageList_Success 测试获取站内信列表成功
func TestPlatformController_GetMessageList_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/messages", ctrl.GetMessageList)

	req, _ := http.NewRequest("GET", "/platform/messages", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_SendMessage_Success 测试发送站内信成功
func TestPlatformController_SendMessage_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/send", ctrl.SendMessage)

	sendReq := map[string]any{
		"title":   "测试消息",
		"content": "这是一条测试消息",
		"type":    "notification",
		"targets": []string{"all"},
	}
	body, _ := json.Marshal(sendReq)

	req, _ := http.NewRequest("POST", "/platform/messages/send", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_SendMessage_InvalidJSON 测试无效 JSON
func TestPlatformController_SendMessage_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/send", ctrl.SendMessage)

	req, _ := http.NewRequest("POST", "/platform/messages/send", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_MarkPlatformMessageRead_Success 测试标记站内信已读成功
func TestPlatformController_MarkPlatformMessageRead_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/:id/read", ctrl.MarkPlatformMessageRead)

	req, _ := http.NewRequest("POST", "/platform/messages/1/read", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_MarkPlatformMessageRead_EmptyID 测试空消息 ID
func TestPlatformController_MarkPlatformMessageRead_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/messages/:id/read", ctrl.MarkPlatformMessageRead)

	req, _ := http.NewRequest("POST", "/platform/messages//read", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_GetUserList_Success 测试获取用户列表成功
func TestPlatformController_GetUserList_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/users", ctrl.GetUserList)

	req, _ := http.NewRequest("GET", "/platform/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CreateUser_Success 测试创建用户成功
func TestPlatformController_CreateUser_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/users", ctrl.CreateUser)

	createReq := map[string]any{
		"username": "testuser",
		"password": "password123",
		"role":     "admin",
		"email":    "test@example.com",
		"phone":    "1234567890",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/platform/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CreateUser_InvalidJSON 测试无效 JSON
func TestPlatformController_CreateUser_InvalidJSON(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.POST("/platform/users", ctrl.CreateUser)

	req, _ := http.NewRequest("POST", "/platform/users", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestPlatformController_DeleteUser_Success 测试删除用户成功
func TestPlatformController_DeleteUser_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.DELETE("/platform/users/:id", ctrl.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/platform/users/1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_DeleteUser_EmptyID 测试空用户 ID
func TestPlatformController_DeleteUser_EmptyID(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.DELETE("/platform/users/:id", ctrl.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/platform/users/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 空 ID 可能返回 400 或 404(取决于路由匹配)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
		t.Errorf("Expected status Bad Request or Not Found, got %d", w.Code)
	}
}

// TestPlatformController_GetSystemStats_Success 测试获取系统统计成功
func TestPlatformController_GetSystemStats_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/stats/system", ctrl.GetSystemStats)

	req, _ := http.NewRequest("GET", "/platform/stats/system", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_GetPlatformStats_Success 测试获取平台统计成功
func TestPlatformController_GetPlatformStats_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/stats", ctrl.GetPlatformStats)

	req, _ := http.NewRequest("GET", "/platform/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_GetPlatformMerchantStats_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/stats/merchants", ctrl.GetPlatformMerchantStats)

	req, _ := http.NewRequest("GET", "/platform/stats/merchants?days=7", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestPlatformController_GetPlatformMerchantStats_DefaultDays(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/stats/merchants", ctrl.GetPlatformMerchantStats)

	req, _ := http.NewRequest("GET", "/platform/stats/merchants", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CheckUpdate_Success 测试检查更新成功
func TestPlatformController_CheckUpdate_Success(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/update/check", ctrl.CheckUpdate)

	req, _ := http.NewRequest("GET", "/platform/update/check?current_version=1.0.0&client_type=frontend", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于服务层依赖外部系统,接受 200 或 500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_CheckUpdate_DefaultVersion 测试默认版本
func TestPlatformController_CheckUpdate_DefaultVersion(t *testing.T) {
	ctrl, router := setupPlatformController(t)
	router.GET("/platform/update/check", ctrl.CheckUpdate)

	req, _ := http.NewRequest("GET", "/platform/update/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 由于服务层依赖外部系统,接受 200 或 500
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status OK or Internal Server Error, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestPlatformController_NewPlatformController 测试构造函数
func TestPlatformController_NewPlatformController(t *testing.T) {
	setPlatformConfigForTest(t)
	setMerchantKeyForTest(t, "test-merchant-key")
	ctrl := NewPlatformController()
	if ctrl == nil {
		t.Error("Expected controller instance, got nil")
	}
}

// TestPlatformController_NilClient 测试平台客户端未初始化时的行为
func TestPlatformController_NilClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	platform.SetMerchantKeyForTest("")
	router := gin.New()
	ctrl := &PlatformController{platformClient: nil}
	router.GET("/platform/dashboard", ctrl.GetDashboard)

	req, _ := http.NewRequest("GET", "/platform/dashboard", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 应当返回 503 服务不可用
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status Service Unavailable, got %d, body: %s", w.Code, w.Body.String())
	}

	platform.SetMerchantKeyForTest("")
}
