package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"hivemtk-user/internal/dto"
	"hivemtk-user/internal/model"
)

// TestUserController_GetUserList_Success 测试获取用户列表成功
func TestUserController_GetUserList_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.GET("/users", ctrl.GetUserList)

	// 创建测试用户
	user := model.User{
		Username: "testuser",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	req, _ := http.NewRequest("GET", "/users?page=1&page_size=10", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_GetUserList_DefaultPagination 测试默认分页参数
func TestUserController_GetUserList_DefaultPagination(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.GET("/users", ctrl.GetUserList)

	req, _ := http.NewRequest("GET", "/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d", w.Code)
	}
}

// TestUserController_GetUser_Success 测试获取用户详情成功
func TestUserController_GetUser_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	router.GET("/users/:id", ctrl.GetUser)

	req, _ := http.NewRequest("GET", "/users/"+user.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_GetUser_NotFound 测试用户不存在
func TestUserController_GetUser_NotFound(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.GET("/users/:id", ctrl.GetUser)

	req, _ := http.NewRequest("GET", "/users/non-existent-id", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status Not Found, got %d", w.Code)
	}
}

// TestUserController_CreateUser_Success 测试创建用户成功
func TestUserController_CreateUser_Success(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users", ctrl.CreateUser)

	createReq := dto.CreateUserRequest{
		Username: "newuser",
		Password: "password123",
		Email:    "newuser@example.com",
		RealName: "New User",
		Role:     "user",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_CreateUser_InvalidJSON 测试无效 JSON 请求
func TestUserController_CreateUser_InvalidJSON(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users", ctrl.CreateUser)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestUserController_CreateUser_EmptyUsername 测试空用户名
func TestUserController_CreateUser_EmptyUsername(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users", ctrl.CreateUser)

	createReq := dto.CreateUserRequest{
		Username: "",
		Password: "password123",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestUserController_CreateUser_EmptyPassword 测试空密码
func TestUserController_CreateUser_EmptyPassword(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users", ctrl.CreateUser)

	createReq := dto.CreateUserRequest{
		Username: "newuser",
		Password: "",
	}
	body, _ := json.Marshal(createReq)

	req, _ := http.NewRequest("POST", "/users", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestUserController_UpdateUser_Success 测试更新用户成功
func TestUserController_UpdateUser_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	router.PUT("/users/:id", ctrl.UpdateUser)

	updateReq := dto.UpdateUserRequest{
		ID:       user.ID,
		RealName: "Updated Name",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/users/"+user.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_UpdateUser_InvalidJSON 测试无效 JSON
func TestUserController_UpdateUser_InvalidJSON(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	router.PUT("/users/:id", ctrl.UpdateUser)

	req, _ := http.NewRequest("PUT", "/users/"+user.ID, bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestUserController_DeleteUser_Success 测试删除用户成功
func TestUserController_DeleteUser_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	router.DELETE("/users/:id", ctrl.DeleteUser)

	req, _ := http.NewRequest("DELETE", "/users/"+user.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_UpdatePassword_Success 测试更新密码成功
func TestUserController_UpdatePassword_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Password: "oldpassword123",
		Status:   1,
	}
	database.Create(&user)

	router.PUT("/users/:id/password", ctrl.UpdatePassword)

	updateReq := dto.UpdatePasswordRequest{
		ID:          user.ID,
		OldPassword: "oldpassword123",
		NewPassword: "newpassword123",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/users/"+user.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_UpdatePassword_WrongOldPassword 测试错误原密码
func TestUserController_UpdatePassword_WrongOldPassword(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()

	user := model.User{
		Username: "testuser",
		Password: "correctpassword",
		Status:   1,
	}
	database.Create(&user)

	router.PUT("/users/:id/password", ctrl.UpdatePassword)

	updateReq := dto.UpdatePasswordRequest{
		ID:          user.ID,
		OldPassword: "wrongpassword",
		NewPassword: "newpassword",
	}
	body, _ := json.Marshal(updateReq)

	req, _ := http.NewRequest("PUT", "/users/"+user.ID+"/password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status Internal Server Error, got %d", w.Code)
	}
}

// TestUserController_Login_Success 测试登录成功
func TestUserController_Login_Success(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users/login", ctrl.Login)

	// 创建测试用户
	user := model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	loginReq := dto.LoginRequest{
		Username: "testuser",
		Password: "password123",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/users/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK, got %d, body: %s", w.Code, w.Body.String())
	}
}

// TestUserController_Login_WrongPassword 测试错误密码
func TestUserController_Login_WrongPassword(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users/login", ctrl.Login)

	// 创建测试用户
	user := model.User{
		Username: "testuser",
		Password: "correctpassword",
		Email:    "test@example.com",
		Status:   1,
	}
	database.Create(&user)

	loginReq := dto.LoginRequest{
		Username: "testuser",
		Password: "wrongpassword",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/users/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestUserController_Login_InvalidJSON 测试无效 JSON
func TestUserController_Login_InvalidJSON(t *testing.T) {
	setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users/login", ctrl.Login)

	req, _ := http.NewRequest("POST", "/users/login", bytes.NewReader([]byte("invalid-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request, got %d", w.Code)
	}
}

// TestUserController_Login_NonExistentUser 测试不存在的用户
func TestUserController_Login_NonExistentUser(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users/login", ctrl.Login)

	// 创建第一个用户
	firstUser := model.User{
		Username: "existinguser",
		Password: "password123",
		Status:   1,
	}
	database.Create(&firstUser)

	loginReq := dto.LoginRequest{
		Username: "nonexistent",
		Password: "password123",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/users/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}

// TestUserController_Login_DisabledUser 测试禁用的用户
func TestUserController_Login_DisabledUser(t *testing.T) {
	database := setupTestControllerDB(t)
	ctrl := NewUserController()
	router := setupGinEngine()
	router.POST("/users/login", ctrl.Login)

	// 创建第一个用户
	firstUser := model.User{
		Username: "admin",
		Password: "admin123",
		Status:   1,
	}
	database.Create(&firstUser)

	// 创建禁用状态的用户
	disabledUser := model.User{
		Username: "disableduser",
		Password: "password123",
		Status:   0,
	}
	database.Create(&disabledUser)

	loginReq := dto.LoginRequest{
		Username: "disableduser",
		Password: "password123",
	}
	body, _ := json.Marshal(loginReq)

	req, _ := http.NewRequest("POST", "/users/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %d", w.Code)
	}
}
