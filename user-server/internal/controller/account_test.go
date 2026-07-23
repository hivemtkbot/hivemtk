package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupTestControllerDB 设置测试数据库
func setupTestControllerDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemUser{},
		&model.User{},
		&model.Account{},
		&model.DouyinCard{},
		&model.ShortLink{},
		&model.DomainPool{},
	)
	db.SetTestDB(database)
	return database
}

// TestAccountController_CreateAccount_Success 测试创建账户成功
func TestAccountController_CreateAccount_Success(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	trueVal := true
	createReq := dto.CreateAccountRequest{
		TgBotToken:          "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:               "100.0",
		GroupID:             123,
		ProxyEnableProxy:    false,
		ProxyProtoclo:       "http",
		ProxyHost:           "localhost",
		ProxyPort:           8080,
		DouyinHeadless:      &trueVal,
		KuaishouHeadless:    &trueVal,
		XiaohongshuHeadless: &trueVal,
		XianyuHeadless:      &trueVal,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_CreateAccount_InvalidJSON 测试无效 JSON 请求
func TestAccountController_CreateAccount_InvalidJSON(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAccountController_CreateAccount_MissingRequiredFields 测试缺少必填字段
func TestAccountController_CreateAccount_MissingRequiredFields(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	createReq := dto.CreateAccountRequest{
		TgBotToken: "", // 必填字段为空
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAccountController_GetAccounts_Success 测试获取账户列表成功
func TestAccountController_GetAccounts_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	// 创建测试账户
	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
		Status:     1,
	}
	database.Create(&account)

	req, _ := http.NewRequest("GET", "/accounts", nil)
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

// TestAccountController_GetAccounts_EmptyList 测试空账户列表
func TestAccountController_GetAccounts_EmptyList(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	req, _ := http.NewRequest("GET", "/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestAccountController_UpdateAccount_Success 测试更新账户成功
func TestAccountController_UpdateAccount_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()

	// 创建测试账户
	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
		Status:     1,
	}
	database.Create(&account)

	router.PUT("/accounts/:id", ctrl.UpdateAccount)

	updateReq := dto.UpdateAccountRequest{
		ID:        "test-account-id",
		TgName:    "Updated TG Name",
		Price:     "200.0",
		ProxyHost: "newproxy.example.com",
		ProxyPort: 9090,
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/accounts/"+account.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_UpdateAccount_InvalidJSON 测试无效 JSON
func TestAccountController_UpdateAccount_InvalidJSON(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()

	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
	}
	database.Create(&account)

	router.PUT("/accounts/:id", ctrl.UpdateAccount)

	req, _ := http.NewRequest("PUT", "/accounts/"+account.ID, bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAccountController_UpdateAccount_MissingID 测试缺少账户 ID
func TestAccountController_UpdateAccount_MissingID(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.PUT("/accounts", ctrl.UpdateAccount)

	updateReq := dto.UpdateAccountRequest{
		ID:    "",
		Price: "200.0",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestAccountController_DeleteAccount_Success 测试删除账户成功
func TestAccountController_DeleteAccount_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()

	// 创建测试账户
	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
	}
	database.Create(&account)

	router.DELETE("/accounts/:id", ctrl.DeleteAccount)

	req, _ := http.NewRequest("DELETE", "/accounts/"+account.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_DeleteAccount_NotFound 测试删除不存在的账户
func TestAccountController_DeleteAccount_NotFound(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.DELETE("/accounts/:id", ctrl.DeleteAccount)

	req, _ := http.NewRequest("DELETE", "/accounts/non-existent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// 删除不存在的账户会返回成功（幂等性）
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_CreateAccount_MinimalRequest 测试最小化创建请求
func TestAccountController_CreateAccount_MinimalRequest(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	createReq := dto.CreateAccountRequest{
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_CreateAccount_WithProxyConfig 测试创建带代理配置的账户
func TestAccountController_CreateAccount_WithProxyConfig(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	trueVal := true
	createReq := dto.CreateAccountRequest{
		TgBotToken:       "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:            "100.0",
		GroupID:          123,
		ProxyEnableProxy: true,
		ProxyProtoclo:    "socks5",
		ProxyHost:        "proxy.example.com",
		ProxyPort:        1080,
		DouyinHeadless:   &trueVal,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_UpdateAccount_PartialUpdate 测试部分更新账户
func TestAccountController_UpdateAccount_PartialUpdate(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()

	account := model.Account{
		ID:         "test-account-id",
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
	}
	database.Create(&account)

	router.PUT("/accounts/:id", ctrl.UpdateAccount)

	updateReq := dto.UpdateAccountRequest{
		ID:     "test-account-id",
		TgName: "New TG Name",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/accounts/"+account.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestAccountController_GetAccounts_MultipleAccounts 测试获取多个账户
func TestAccountController_GetAccounts_MultipleAccounts(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.GET("/accounts", ctrl.GetAccounts)

	// 创建多个测试账户
	accounts := []model.Account{}
	for _, account := range accounts {
		database.Create(&account)
	}

	req, _ := http.NewRequest("GET", "/accounts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}

	var response map[string]any
	json.Unmarshal(w.Body.Bytes(), &response)
	data := response["data"].(map[string]any)
	list := data["list"].([]any)
	if len(list) != 3 {
		t.Errorf("Expected 3 accounts, got %d", len(list))
	}
}

// TestAccountController_CreateAccount_WithoutHeadless 测试创建不带无头模式配置的账户
func TestAccountController_CreateAccount_WithoutHeadless(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewAccountController()
	router := setupGinEngine()
	router.POST("/accounts", ctrl.CreateAccount)

	createReq := dto.CreateAccountRequest{
		TgBotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
		Price:      "100.0",
		GroupID:    123,
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/accounts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}
