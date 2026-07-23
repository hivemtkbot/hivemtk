package service

import (
	"context"
	"testing"

	"marketing/internal/dto"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

func setupUserServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.User{},
	)
	db.SetTestDB(database)
	return database
}

func TestNewUserService(t *testing.T) {
	service := NewUserService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

func TestUserService_RegisterUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	req := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
	}

	response, err := service.RegisterUser(req)
	if err != nil {
		t.Fatalf("RegisterUser failed: %v", err)
	}

	if response == nil {
		t.Fatal("Expected non-nil response")
	}

	if response.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %s", response.Username)
	}

	if response.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got %s", response.Email)
	}
}

func TestUserService_RegisterUser_DuplicateUsername(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册一个用户
	req := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	service.RegisterUser(req)

	// 尝试注册相同用户名的用户
	req2 := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password456",
		Email:    "test2@example.com",
		Role:     "user",
	}

	_, err := service.RegisterUser(req2)
	if err == nil {
		t.Error("Expected error for duplicate username")
	}

	if err.Error() != "用户名已存在" {
		t.Errorf("Expected '用户名已存在', got %s", err.Error())
	}
}

func TestUserService_RegisterUser_DuplicateEmail(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册一个用户
	req := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	service.RegisterUser(req)

	// 尝试注册相同邮箱的用户
	req2 := &dto.CreateUserRequest{
		Username: "testuser2",
		Password: "password456",
		Email:    "test@example.com",
		Role:     "user",
	}

	_, err := service.RegisterUser(req2)
	if err == nil {
		t.Error("Expected error for duplicate email")
	}

	if err.Error() != "邮箱已存在" {
		t.Errorf("Expected '邮箱已存在', got %s", err.Error())
	}
}

func TestUserService_GetUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 获取用户
	response, err := service.GetUser(regResp.ID)
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if response.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %s", response.Username)
	}
}

func TestUserService_GetUser_NotFound(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	_, err := service.GetUser("non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

func TestUserService_GetUserByUsername(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 根据用户名获取
	response, err := service.GetUserByUsername("testuser")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}

	if response.ID != regResp.ID {
		t.Errorf("Expected ID %s, got %s", regResp.ID, response.ID)
	}
}

func TestUserService_GetUserByUsername_NotFound(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	_, err := service.GetUserByUsername("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent username")
	}
}

func TestUserService_GetUserList(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 注册多个用户
	for i := 0; i < 5; i++ {
		req := &dto.CreateUserRequest{
			Username: "user" + string(rune('0'+i)),
			Password: "password123",
			Email:    "user" + string(rune('0'+i)) + "@example.com",
			Role:     "user",
		}
		service.RegisterUser(req)
	}

	// 获取用户列表
	response, err := service.GetUserList(1, 10)
	if err != nil {
		t.Fatalf("GetUserList failed: %v", err)
	}

	if response.Total != 5 {
		t.Errorf("Expected total 5, got %d", response.Total)
	}

	if len(response.Users) != 5 {
		t.Errorf("Expected 5 users, got %d", len(response.Users))
	}
}

func TestUserService_DeleteUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 删除用户
	err := service.DeleteUser(regResp.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// 验证用户已被删除
	_, err = service.GetUser(regResp.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

func TestUserService_UpdateUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 更新用户
	status := 1
	updateReq := &dto.UpdateUserRequest{
		RealName: "Updated Name",
		Phone:    "13812345678",
		Status:   &status,
	}

	response, err := service.UpdateUser(regResp.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	if response.RealName != "Updated Name" {
		t.Errorf("Expected RealName 'Updated Name', got %s", response.RealName)
	}

	if response.Phone != "13812345678" {
		t.Errorf("Expected Phone '13812345678', got %s", response.Phone)
	}
}

func TestUserService_UpdateUser_DuplicateUsername(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 注册两个用户
	regReq1 := &dto.CreateUserRequest{
		Username: "testuser1",
		Password: "password123",
		Email:    "test1@example.com",
		Role:     "user",
	}
	_, _ = service.RegisterUser(regReq1)

	regReq2 := &dto.CreateUserRequest{
		Username: "testuser2",
		Password: "password123",
		Email:    "test2@example.com",
		Role:     "user",
	}
	regResp2, _ := service.RegisterUser(regReq2)

	// 尝试将 user2 的用户名改为 user1 的用户名（已存在）
	updateReq := &dto.UpdateUserRequest{
		Username: "testuser1",
	}

	_, err := service.UpdateUser(regResp2.ID, updateReq)
	if err == nil {
		t.Error("Expected error for duplicate username")
	}

	if err.Error() != "用户名已存在" {
		t.Errorf("Expected '用户名已存在', got %s", err.Error())
	}
}

func TestUserService_UpdatePassword(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "oldpassword123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 更新密码
	updateReq := &dto.UpdatePasswordRequest{
		OldPassword: "oldpassword123",
		NewPassword: "newpassword123",
	}

	err := service.UpdatePassword(regResp.ID, updateReq)
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	// 验证新密码是否生效
	loginReq := &dto.LoginRequest{
		Username: "testuser",
		Password: "newpassword123",
	}
	loginResp, err := service.Login(loginReq)
	if err != nil {
		t.Fatalf("Login with new password failed: %v", err)
	}

	if loginResp == nil {
		t.Error("Expected successful login with new password")
	}
}

func TestUserService_UpdatePassword_WrongOldPassword(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "correctpassword",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 尝试使用错误的旧密码更新
	updateReq := &dto.UpdatePasswordRequest{
		OldPassword: "wrongpassword",
		NewPassword: "newpassword",
	}

	err := service.UpdatePassword(regResp.ID, updateReq)
	if err == nil {
		t.Error("Expected error for wrong old password")
	}

	if err.Error() != "原密码不正确" {
		t.Errorf("Expected '原密码不正确', got %s", err.Error())
	}
}

func TestUserService_Login(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 登录
	loginReq := &dto.LoginRequest{
		Username: "testuser",
		Password: "password123",
	}

	loginResp, err := service.Login(loginReq)
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if loginResp == nil {
		t.Fatal("Expected non-nil response")
	}

	if loginResp.User.ID != regResp.ID {
		t.Errorf("Expected user ID %s, got %s", regResp.ID, loginResp.User.ID)
	}

	if loginResp.Token == "" {
		t.Error("Expected non-empty token")
	}
}

func TestUserService_Login_WrongPassword(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	service.RegisterUser(regReq)

	// 使用错误密码登录
	loginReq := &dto.LoginRequest{
		Username: "testuser",
		Password: "wrongpassword",
	}

	_, err := service.Login(loginReq)
	if err == nil {
		t.Error("Expected error for wrong password")
	}

	if err.Error() != "用户名或密码错误" {
		t.Errorf("Expected '用户名或密码错误', got %s", err.Error())
	}
}

func TestUserService_InitUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 初始化用户
	userID, err := service.InitUser("account123", 12345, "John", "Doe", "johndoe")
	if err != nil {
		t.Fatalf("InitUser failed: %v", err)
	}

	if userID == "" {
		t.Error("Expected non-empty user ID")
	}

	// 再次初始化相同用户，应该返回相同的 ID
	userID2, err := service.InitUser("account123", 12345, "John", "Doe", "johndoe")
	if err != nil {
		t.Fatalf("InitUser second call failed: %v", err)
	}

	if userID != userID2 {
		t.Errorf("Expected same user ID %s, got %s", userID, userID2)
	}
}

func TestUserService_Login_DisabledUser(t *testing.T) {
	setupUserServiceTestDB(t)

	service := NewUserService()

	// 先注册用户
	regReq := &dto.CreateUserRequest{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		Role:     "user",
	}
	regResp, _ := service.RegisterUser(regReq)

	// 手动禁用用户
	db.GetDB().Model(&model.User{}).Where("id = ?", regResp.ID).Update("status", 0)

	// 尝试登录
	loginReq := &dto.LoginRequest{
		Username: "testuser",
		Password: "password123",
	}

	_, err := service.Login(loginReq)
	if err == nil {
		t.Error("Expected error for disabled user")
	}

	if err.Error() != "账户已被禁用" {
		t.Errorf("Expected '账户已被禁用', got %s", err.Error())
	}
}
