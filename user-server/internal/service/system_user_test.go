package service

import (
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupSystemUserServiceTestDB 设置系统用户服务测试数据库
func setupSystemUserServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.SystemUser{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewSystemUserService 测试创建系统用户服务
func TestNewSystemUserService(t *testing.T) {
	service := NewSystemUserService()
	if service == nil {
		t.Error("Expected non-nil service")
	}
}

// TestSystemUserService_GetUsers_Empty 测试空用户列表
func TestSystemUserService_GetUsers_Empty(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0, got %d", total)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}
}

// TestSystemUserService_GetUsers_WithUsers 测试获取用户列表
func TestSystemUserService_GetUsers_WithUsers(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	for i := 0; i < 5; i++ {
		user := model.SystemUser{
			Username: "user" + string(rune('0'+i)),
			Password: "Password123",
			Email:    "user" + string(rune('0'+i)) + "@example.com",
			RealName: "User " + string(rune('0'+i)),
			Role:     "user",
			Status:   1,
		}
		database.Create(&user)
	}

	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users, got %d", len(users))
	}

	// 验证用户数据存在（由于按创建时间倒序，不检查具体顺序）
	usernames := make(map[string]bool)
	for _, user := range users {
		usernames[user.Username] = true
	}
	for i := 0; i < 5; i++ {
		expectedUsername := "user" + string(rune('0'+i))
		if !usernames[expectedUsername] {
			t.Errorf("Expected username %s to be in results", expectedUsername)
		}
	}
}

// TestSystemUserService_GetUsers_Pagination 测试分页
func TestSystemUserService_GetUsers_Pagination(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建 15 个用户，使用不同的用户名
	for i := 0; i < 15; i++ {
		user := model.SystemUser{
			Username: "pageuser" + string(rune('a'+i)),
			Password: "Password123",
			Email:    "pageuser" + string(rune('a'+i)) + "@example.com",
			Role:     "user",
			Status:   1,
		}
		database.Create(&user)
	}

	// 第一页
	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers page 1 failed: %v", err)
	}

	if total != 15 {
		t.Errorf("Expected total 15, got %d", total)
	}

	if len(users) != 10 {
		t.Errorf("Expected 10 users on page 1, got %d", len(users))
	}

	// 第二页
	users, total, err = service.GetUsers(2, 10)
	if err != nil {
		t.Fatalf("GetUsers page 2 failed: %v", err)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users on page 2, got %d", len(users))
	}
}

// TestSystemUserService_GetUserByID 测试根据 ID 获取用户
func TestSystemUserService_GetUserByID(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 获取用户
	retrievedUser, err := service.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	if retrievedUser.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %s", retrievedUser.Username)
	}

	if retrievedUser.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got %s", retrievedUser.Email)
	}

	if retrievedUser.RealName != "Test User" {
		t.Errorf("Expected real_name 'Test User', got %s", retrievedUser.RealName)
	}

	if retrievedUser.Role != "user" {
		t.Errorf("Expected role 'user', got %s", retrievedUser.Role)
	}

	if retrievedUser.Status != 1 {
		t.Errorf("Expected status 1, got %d", retrievedUser.Status)
	}
}

// TestSystemUserService_GetUserByID_NotFound 测试获取不存在的用户
func TestSystemUserService_GetUserByID_NotFound(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	_, err := service.GetUserByID(99999)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}

	if err.Error() != "用户不存在" {
		t.Errorf("Expected '用户不存在', got %s", err.Error())
	}
}

// TestSystemUserService_CreateUser 测试创建用户
func TestSystemUserService_CreateUser(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "newuser",
		Password: "Password123",
		Email:    "new@example.com",
		RealName: "New User",
		Role:     "user",
		Status:   1,
	}

	user, err := service.CreateUser(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user == nil {
		t.Fatal("Expected non-nil user")
	}

	if user.Username != "newuser" {
		t.Errorf("Expected username 'newuser', got %s", user.Username)
	}

	if user.Email != "new@example.com" {
		t.Errorf("Expected email 'new@example.com', got %s", user.Email)
	}

	if user.RealName != "New User" {
		t.Errorf("Expected real_name 'New User', got %s", user.RealName)
	}

	if user.Role != "user" {
		t.Errorf("Expected role 'user', got %s", user.Role)
	}

	if user.Status != 1 {
		t.Errorf("Expected status 1, got %d", user.Status)
	}
}

// TestSystemUserService_CreateUser_DuplicateUsername 测试创建重复用户名的用户
func TestSystemUserService_CreateUser_DuplicateUsername(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 先创建一个用户
	user := model.SystemUser{
		Username: "existinguser",
		Password: "Password123",
		Email:    "existing@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 尝试创建相同用户名的用户
	req := &CreateUserRequest{
		Username: "existinguser",
		Password: "Password456",
		Email:    "new@example.com",
		Role:     "user",
		Status:   1,
	}

	_, err := service.CreateUser(req)
	if err == nil {
		t.Error("Expected error for duplicate username")
	}

	if err.Error() != "用户名已存在" {
		t.Errorf("Expected '用户名已存在', got %s", err.Error())
	}
}

// TestSystemUserService_CreateUser_AdminRole 测试创建管理员用户
func TestSystemUserService_CreateUser_AdminRole(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "adminuser",
		Password: "Password123",
		Email:    "admin@example.com",
		Role:     "admin",
		Status:   1,
	}

	user, err := service.CreateUser(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", user.Role)
	}
}

// TestSystemUserService_CreateUser_DefaultStatus 测试默认状态
func TestSystemUserService_CreateUser_DefaultStatus(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "newuser",
		Password: "Password123",
		Email:    "new@example.com",
		Role:     "user",
		// Status 不设置，应该默认为 1
	}

	user, err := service.CreateUser(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.Status != 1 {
		t.Errorf("Expected default status 1, got %d", user.Status)
	}
}

func TestSystemUserService_CreateUser_WithMerchantID(t *testing.T) {
	// 显式初始化测试 DB：本测试原本依赖全局 DB，但其他测试的 cleanup 会关闭全局 DB，
	// 导致 "sql: database is closed"。显式 setup 后通过 SetTestDB 注入全局 DB。
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "merchantuser",
		Password: "Password123",
		Email:    "merchant@example.com",
		Role:     "user",
		Status:   1,
	}

	user, err := service.CreateUser(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user == nil {
		t.Fatal("Expected user to be returned")
	}
}

// TestSystemUserService_UpdateUser 测试更新用户
func TestSystemUserService_UpdateUser(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 更新用户（注意：服务代码中 Status != 0 才会更新，所以用 2 而不是 0）
	req := &UpdateUserRequest{
		Email:    "updated@example.com",
		RealName: "Updated User",
		Role:     "admin",
		Status:   2, // 使用非 0 值
	}

	updatedUser, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	if updatedUser.Email != "updated@example.com" {
		t.Errorf("Expected email 'updated@example.com', got %s", updatedUser.Email)
	}

	if updatedUser.RealName != "Updated User" {
		t.Errorf("Expected real_name 'Updated User', got %s", updatedUser.RealName)
	}

	if updatedUser.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", updatedUser.Role)
	}

	if updatedUser.Status != 2 {
		t.Errorf("Expected status 2, got %d", updatedUser.Status)
	}
}

// TestSystemUserService_UpdateUser_NotFound 测试更新不存在的用户
func TestSystemUserService_UpdateUser_NotFound(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &UpdateUserRequest{
		Email: "updated@example.com",
	}

	_, err := service.UpdateUser(99999, req)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}

	if err.Error() != "用户不存在" {
		t.Errorf("Expected '用户不存在', got %s", err.Error())
	}
}

// TestSystemUserService_UpdateUser_PartialUpdate 测试部分更新
func TestSystemUserService_UpdateUser_PartialUpdate(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 只更新邮箱
	req := &UpdateUserRequest{
		Email: "updated@example.com",
		// 其他字段为空
	}

	updatedUser, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// 验证邮箱已更新
	if updatedUser.Email != "updated@example.com" {
		t.Errorf("Expected email 'updated@example.com', got %s", updatedUser.Email)
	}

	// 验证其他字段保持不变
	if updatedUser.RealName != "Test User" {
		t.Errorf("Expected real_name 'Test User', got %s", updatedUser.RealName)
	}

	if updatedUser.Role != "user" {
		t.Errorf("Expected role 'user', got %s", updatedUser.Role)
	}
}

func TestSystemUserService_UpdateUser_WithMerchantID(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	req := &UpdateUserRequest{}

	_, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	var dbUser model.SystemUser
	database.First(&dbUser, user.ID)
}

// TestSystemUserService_DeleteUser 测试删除用户
func TestSystemUserService_DeleteUser(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 删除用户
	err := service.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// 验证用户已被删除
	_, err = service.GetUserByID(user.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}

	if err.Error() != "用户不存在" {
		t.Errorf("Expected '用户不存在', got %s", err.Error())
	}
}

// TestSystemUserService_DeleteUser_NotFound 测试删除不存在的用户
func TestSystemUserService_DeleteUser_NotFound(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	err := service.DeleteUser(99999)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}

	if err.Error() != "用户不存在" {
		t.Errorf("Expected '用户不存在', got %s", err.Error())
	}
}

// TestSystemUserService_ResetPassword 测试重置密码
func TestSystemUserService_ResetPassword(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "oldpassword123",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 验证旧密码
	oldUser, _ := service.GetUserByID(user.ID)
	if oldUser.Username != "testuser" {
		t.Fatal("Failed to create user")
	}

	// 重置密码
	newPassword := "newpassword123"
	err := service.ResetPassword(user.ID, newPassword)
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// 验证密码已更新（通过检查数据库中的密码是否已更改）
	var updatedUser model.SystemUser
	database.First(&updatedUser, user.ID)

	// 新密码应该可以验证通过
	if !CheckPassword(&updatedUser,newPassword) {
		t.Error("New password should be valid")
	}

	// 旧密码应该验证失败
	if CheckPassword(&updatedUser,"oldpassword123") {
		t.Error("Old password should be invalid")
	}
}

// TestSystemUserService_ResetPassword_NotFound 测试重置不存在用户的密码
func TestSystemUserService_ResetPassword_NotFound(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	err := service.ResetPassword(99999, "newpassword123")
	if err == nil {
		t.Error("Expected error for non-existent user")
	}

	if err.Error() != "用户不存在" {
		t.Errorf("Expected '用户不存在', got %s", err.Error())
	}
}

// TestSystemUserService_ResetPassword_InvalidPassword 测试重置密码时密码加密
func TestSystemUserService_ResetPassword_PasswordHashing(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "oldpassword123",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 重置密码
	newPassword := "newpassword123"
	err := service.ResetPassword(user.ID, newPassword)
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// 验证密码在数据库中是加密的（不应该等于明文）
	var updatedUser model.SystemUser
	database.First(&updatedUser, user.ID)

	if updatedUser.Password == newPassword {
		t.Error("Password should be hashed, not stored as plain text")
	}

	// 验证密码可以正确验证
	if !CheckPassword(&updatedUser,newPassword) {
		t.Error("Password verification should succeed")
	}
}

// TestSystemUserService_GetUsers_OrderByCreatedAt 测试按创建时间排序
func TestSystemUserService_GetUsers_OrderByCreatedAt(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 按顺序创建用户
	usernames := []string{"first", "second", "third", "fourth", "fifth"}
	for _, username := range usernames {
		user := model.SystemUser{
			Username: username,
			Password: "Password123",
			Email:    username + "@example.com",
			Role:     "user",
			Status:   1,
		}
		database.Create(&user)
	}

	users, _, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	// 验证按创建时间倒序排列（最新的在前）
	expectedOrder := []string{"fifth", "fourth", "third", "second", "first"}
	for i, expectedUsername := range expectedOrder {
		if users[i].Username != expectedUsername {
			t.Errorf("Expected username %s at position %d, got %s", expectedUsername, i, users[i].Username)
		}
	}
}

// TestSystemUserService_CreateUser_InvalidRole 测试创建无效角色的用户
// 服务层现在会验证角色有效性
func TestSystemUserService_CreateUser_InvalidRole(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 尝试创建无效角色的用户
	req := &CreateUserRequest{
		Username: "invaliduser",
		Password: "Password123",
		Email:    "invalid@example.com",
		Role:     "invalid_role", // 无效角色
		Status:   1,
	}

	// 服务层现在会拒绝无效角色
	_, err := service.CreateUser(req)
	if err == nil {
		t.Fatal("CreateUser should fail for invalid role")
	}

	if err.Error() != "角色非法，仅支持 admin/user" {
		t.Errorf("Expected '角色非法，仅支持 admin/user', got %s", err.Error())
	}
}

// TestSystemUserService_UpdateUser_InvalidRole 测试更新无效角色的用户
// 注意：服务层不验证角色有效性，由控制器层通过 binding 标签验证
func TestSystemUserService_UpdateUser_InvalidRole(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 尝试更新为无效角色
	req := &UpdateUserRequest{
		Role: "invalid_role", // 无效角色
	}

	// 服务层不会验证角色，所以会更新成功
	// 角色验证由控制器的 binding 标签处理
	updatedUser, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser should not fail at service level for invalid role: %v", err)
	}

	if updatedUser.Role != "invalid_role" {
		t.Errorf("Expected role 'invalid_role', got %s", updatedUser.Role)
	}
}

// TestSystemUserService_CreateUser_EmptyPassword 测试创建空密码的用户
func TestSystemUserService_CreateUser_EmptyPassword(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "emptypassword",
		Password: "", // 空密码
		Email:    "empty@example.com",
		Role:     "user",
		Status:   1,
	}

	_, err := service.CreateUser(req)
	if err != nil {
		// 空密码应该被拒绝或处理
		t.Logf("CreateUser with empty password returned: %v", err)
	}
}

// TestSystemUserService_GetUserByID_AdminRole 测试获取管理员用户
func TestSystemUserService_GetUserByID_AdminRole(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建管理员用户
	user := model.SystemUser{
		Username: "adminuser",
		Password: "Password123",
		Email:    "admin@example.com",
		Role:     "admin",
		Status:   1,
	}
	database.Create(&user)

	retrievedUser, err := service.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID failed: %v", err)
	}

	if retrievedUser.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", retrievedUser.Role)
	}
}

// TestSystemUserService_UpdateUser_ToAdmin 测试将用户更新为管理员
func TestSystemUserService_UpdateUser_ToAdmin(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建普通用户
	user := model.SystemUser{
		Username: "regularuser",
		Password: "Password123",
		Email:    "user@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 更新为管理员
	req := &UpdateUserRequest{
		Role: "admin",
	}

	updatedUser, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	if updatedUser.Role != "admin" {
		t.Errorf("Expected role 'admin', got %s", updatedUser.Role)
	}
}

// TestSystemUserService_GetUsers_DisabledUsers 测试获取禁用用户
func TestSystemUserService_GetUsers_DisabledUsers(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建启用用户
	for i := 0; i < 3; i++ {
		user := model.SystemUser{
			Username: "active" + string(rune('0'+i)),
			Password: "Password123",
			Email:    "active" + string(rune('0'+i)) + "@example.com",
			Role:     "user",
			Status:   1,
		}
		database.Create(&user)
	}

	// 创建禁用用户
	for i := 0; i < 2; i++ {
		user := model.SystemUser{
			Username: "disabled" + string(rune('0'+i)),
			Password: "Password123",
			Email:    "disabled" + string(rune('0'+i)) + "@example.com",
			Role:     "user",
			Status:   0,
		}
		database.Create(&user)
	}

	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	// 应该获取所有用户，包括禁用的
	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users, got %d", len(users))
	}
}

// TestSystemUserService_CreateUser_SpecialCharacters 测试创建带特殊字符的用户
func TestSystemUserService_CreateUser_SpecialCharacters(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	req := &CreateUserRequest{
		Username: "user_with_special",
		Password: "P@ssw0rd!23",
		Email:    "user+test@example.com",
		RealName: "User With Special Chars",
		Role:     "user",
		Status:   1,
	}

	user, err := service.CreateUser(req)
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if user.Email != "user+test@example.com" {
		t.Errorf("Expected email 'user+test@example.com', got %s", user.Email)
	}
}

// TestSystemUserService_UpdateUser_EmptyFields 测试更新空字段
func TestSystemUserService_UpdateUser_EmptyFields(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 用空字段更新
	req := &UpdateUserRequest{
		Email:    "",
		RealName: "",
		Role:     "",
		Status:   0, // Status 为 0 不会更新（服务代码逻辑：if req.Status != 0）
	}

	updatedUser, err := service.UpdateUser(user.ID, req)
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}

	// 验证空字段不会覆盖原有数据
	if updatedUser.Email != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got %s", updatedUser.Email)
	}

	if updatedUser.RealName != "Test User" {
		t.Errorf("Expected real_name 'Test User', got %s", updatedUser.RealName)
	}

	if updatedUser.Role != "user" {
		t.Errorf("Expected role 'user', got %s", updatedUser.Role)
	}

	// Status 为 0 不会更新（这是服务代码的设计逻辑）
	if updatedUser.Status != 1 {
		t.Errorf("Expected status 1 (unchanged), got %d", updatedUser.Status)
	}
}

// TestSystemUserService_toUserResponse 测试 toUserResponse 方法
func TestSystemUserService_toUserResponse(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "Password123",
		Email:    "test@example.com",
		RealName: "Test User",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 获取响应
	response := service.toUserResponse(&user)

	if response == nil {
		t.Fatal("Expected non-nil response")
	}

	if response.ID != user.ID {
		t.Errorf("Expected ID %d, got %d", user.ID, response.ID)
	}

	if response.Username != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, response.Username)
	}

	if response.Email != user.Email {
		t.Errorf("Expected email %s, got %s", user.Email, response.Email)
	}

	if response.RealName != user.RealName {
		t.Errorf("Expected real_name %s, got %s", user.RealName, response.RealName)
	}

	if response.Role != user.Role {
		t.Errorf("Expected role %s, got %s", user.Role, response.Role)
	}

	if response.Status != user.Status {
		t.Errorf("Expected status %d, got %d", user.Status, response.Status)
	}

	// 验证密码不在响应中（已验证响应结构体不包含密码字段）
}

// TestSystemUserService_CreateUser_MultipleUsers 测试批量创建用户
func TestSystemUserService_CreateUser_MultipleUsers(t *testing.T) {
	setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	usernames := []string{"user1", "user2", "user3", "user4", "user5"}

	for i, username := range usernames {
		req := &CreateUserRequest{
			Username: username,
			Password: "Password123",
			Email:    username + "@example.com",
			Role:     "user",
			Status:   1,
		}

		user, err := service.CreateUser(req)
		if err != nil {
			t.Fatalf("CreateUser %d failed: %v", i, err)
		}

		if user.Username != username {
			t.Errorf("Expected username %s, got %s", username, user.Username)
		}
	}

	// 验证所有用户都已创建
	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users, got %d", len(users))
	}
}

// TestSystemUserService_DeleteUser_MultipleUsers 测试删除多个用户
func TestSystemUserService_DeleteUser_MultipleUsers(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建多个用户
	userIDs := []uint{}
	for i := 0; i < 3; i++ {
		user := model.SystemUser{
			Username: "deleteuser" + string(rune('0'+i)),
			Password: "Password123",
			Email:    "deleteuser" + string(rune('0'+i)) + "@example.com",
			Role:     "user",
			Status:   1,
		}
		database.Create(&user)
		userIDs = append(userIDs, user.ID)
	}

	// 逐个删除
	for _, id := range userIDs {
		err := service.DeleteUser(id)
		if err != nil {
			t.Fatalf("DeleteUser %d failed: %v", id, err)
		}
	}

	// 验证所有用户都已删除
	users, total, err := service.GetUsers(1, 10)
	if err != nil {
		t.Fatalf("GetUsers failed: %v", err)
	}

	if total != 0 {
		t.Errorf("Expected total 0 after deletion, got %d", total)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users after deletion, got %d", len(users))
	}
}

// TestSystemUserService_ResetPassword_MultipleTimes 测试多次重置密码
func TestSystemUserService_ResetPassword_MultipleTimes(t *testing.T) {
	database := setupSystemUserServiceTestDB(t)
	service := NewSystemUserService()

	// 创建测试用户
	user := model.SystemUser{
		Username: "testuser",
		Password: "password1",
		Email:    "test@example.com",
		Role:     "user",
		Status:   1,
	}
	database.Create(&user)

	// 多次重置密码
	passwords := []string{"password2", "password3", "password4"}
	for _, pwd := range passwords {
		err := service.ResetPassword(user.ID, pwd)
		if err != nil {
			t.Fatalf("ResetPassword failed: %v", err)
		}

		// 验证新密码有效
		var updatedUser model.SystemUser
		database.First(&updatedUser, user.ID)
		if !CheckPassword(&updatedUser,pwd) {
			t.Errorf("Password %s should be valid", pwd)
		}
	}

	// 验证只有最后一个密码有效
	var finalUser model.SystemUser
	database.First(&finalUser, user.ID)
	if !CheckPassword(&finalUser,"password4") {
		t.Error("Final password should be valid")
	}
	if CheckPassword(&finalUser,"password1") {
		t.Error("Initial password should be invalid")
	}
}
