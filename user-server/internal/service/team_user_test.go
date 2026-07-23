package service

import (
	"context"
	"encoding/json"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/bcrypt"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupTeamUserServiceTestDB 设置测试数据库
func setupTeamUserServiceTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.TeamUser{},
		&model.TeamRole{},
		&model.OperationLog{},
	)
	db.SetTestDB(database)
	return database
}

// setupTeamUserService 设置测试服务
func setupTeamUserService(t *testing.T) *TeamUserService {
	setupTeamUserServiceTestDB(t)
	return NewTeamUserService()
}

// setupTeamRoleService 设置角色服务
func setupTeamRoleService(t *testing.T) *TeamRoleService {
	setupTeamUserServiceTestDB(t)
	return NewTeamRoleService()
}

// setupPermissionService 设置权限服务
func setupPermissionService(t *testing.T) *PermissionService {
	setupTeamUserServiceTestDB(t)
	return NewPermissionService()
}

// TestNewTeamUserService 测试创建服务实例
func TestNewTeamUserService(t *testing.T) {
	service := NewTeamUserService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.repo == nil {
		t.Error("Expected repo to be initialized")
	}
	if service.roleRepo == nil {
		t.Error("Expected roleRepo to be initialized")
	}
	if service.logRepo == nil {
		t.Error("Expected logRepo to be initialized")
	}
}

// TestTeamUserService_Create_Success 测试创建用户成功
func TestTeamUserService_Create_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 先创建角色
	role := &model.TeamRole{
		Code:        "operator",
		Name:        "Operator",
		Permissions: "[]",
		IsSystem:    false,
	}
	service.roleRepo.Create(role)

	req := &CreateTeamUserRequest{
		Username: "testuser",
		Password: "password123",
		Name:     "Test User",
		Email:    "test@example.com",
		Phone:    "123456789",
		Role:     "operator",
		Avatar:   "avatar.jpg",
	}

	user, err := service.Create(context.Background(), req, 1, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if user.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", user.Username)
	}
	if user.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", user.Name)
	}
	if user.Status != model.TeamUserStatusActive {
		t.Errorf("Expected status active, got %d", user.Status)
	}
}

// TestTeamUserService_Create_UsernameExists 测试用户名已存在
func TestTeamUserService_Create_UsernameExists(t *testing.T) {
	service := setupTeamUserService(t)

	// 先创建用户
	existingUser := &model.TeamUser{
		Username: "existing",
		Password: "password",
	}
	service.repo.Create(existingUser)

	req := &CreateTeamUserRequest{
		Username: "existing",
		Password: "password123",
		Role:     "operator",
	}

	_, err := service.Create(context.Background(), req, 1, "admin", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for existing username")
	}
}

// TestTeamUserService_Create_InvalidRole 测试无效角色
func TestTeamUserService_Create_InvalidRole(t *testing.T) {
	service := setupTeamUserService(t)

	req := &CreateTeamUserRequest{
		Username: "testuser",
		Password: "password123",
		Role:     "invalid_role",
	}

	_, err := service.Create(context.Background(), req, 1, "admin", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for invalid role")
	}
}

// TestTeamUserService_Update_Success 测试更新用户成功
func TestTeamUserService_Update_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 先创建用户
	user := &model.TeamUser{
		Username: "testuser",
		Password: "password",
		Name:     "Old Name",
		Role:     "operator",
		Status:   model.TeamUserStatusActive,
	}
	service.repo.Create(user)

	req := &UpdateTeamUserRequest{
		Name:  "New Name",
		Email: "new@example.com",
	}

	updated, err := service.Update(context.Background(), user.ID, req, 1, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Name != "New Name" {
		t.Errorf("Expected name 'New Name', got '%s'", updated.Name)
	}
	if updated.Email != "new@example.com" {
		t.Errorf("Expected email 'new@example.com', got '%s'", updated.Email)
	}
}

// TestTeamUserService_Update_NoPermission 测试无权限更新
func TestTeamUserService_Update_NoPermission(t *testing.T) {
	service := setupTeamUserService(t)

	// 先创建用户
	user := &model.TeamUser{
		Username: "testuser",
		Password: "password",
	}
	service.repo.Create(user)

	req := &UpdateTeamUserRequest{
		Name: "New Name",
	}

	// viewer 角色无法 update 其他用户（甚至不能 update 自己；只能改密）
	_, err := service.Update(context.Background(), user.ID, req, 1, "viewer", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for no permission")
	}
}

// TestTeamUserService_Delete_Success 测试删除用户成功
func TestTeamUserService_Delete_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建两个管理员，确保删除后还有管理员
	admin1 := &model.TeamUser{
		Username: "admin1",
		Password: "password",
		Role:     "admin",
	}
	admin2 := &model.TeamUser{
		Username: "admin2",
		Password: "password",
		Role:     "admin",
	}
	service.repo.Create(admin1)
	service.repo.Create(admin2)

	// 删除 admin2
	err := service.Delete(context.Background(), admin2.ID, admin1.ID, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, err = service.repo.GetByID(admin2.ID)
	if err == nil {
		t.Error("Expected user to be deleted")
	}
}

// TestTeamUserService_Delete_CannotDeleteSelf 测试不能删除自己
func TestTeamUserService_Delete_CannotDeleteSelf(t *testing.T) {
	service := setupTeamUserService(t)

	user := &model.TeamUser{
		Username: "testuser",
		Password: "password",
		Role:     "operator",
	}
	service.repo.Create(user)

	err := service.Delete(context.Background(), user.ID, user.ID, "admin", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for deleting self")
	}
}

// TestTeamUserService_Delete_LastAdmin 测试不能删除最后一个管理员
func TestTeamUserService_Delete_LastAdmin(t *testing.T) {
	service := setupTeamUserService(t)

	// 只创建一个管理员
	admin := &model.TeamUser{
		Username: "admin",
		Password: "password",
		Role:     "admin",
	}
	service.repo.Create(admin)

	err := service.Delete(context.Background(), admin.ID, 999, "admin", "127.0.0.1")
	if err == nil {
		t.Error("Expected error for deleting last admin")
	}
}

// TestTeamUserService_GetByID_Success 测试根据 ID 获取用户成功
func TestTeamUserService_GetByID_Success(t *testing.T) {
	service := setupTeamUserService(t)

	user := &model.TeamUser{
		Username: "testuser",
		Password: "password",
	}
	service.repo.Create(user)

	retrieved, err := service.GetByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", retrieved.Username)
	}
}

// TestTeamUserService_GetByID_NotFound 测试获取不存在的用户
func TestTeamUserService_GetByID_NotFound(t *testing.T) {
	service := setupTeamUserService(t)

	_, err := service.GetByID(context.Background(), 99999)
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

// TestTeamUserService_GetList_Success 测试获取用户列表成功
func TestTeamUserService_GetList_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建多个用户
	for i := 0; i < 5; i++ {
		user := &model.TeamUser{
			Username: "user" + string(rune('0'+i)),
			Password: "password",
		}
		service.repo.Create(user)
	}

	result, err := service.GetList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if result.Total != 5 {
		t.Errorf("Expected total 5, got %d", result.Total)
	}
	if len(result.List) != 5 {
		t.Errorf("Expected 5 users, got %d", len(result.List))
	}
}

// TestTeamUserService_GetList_Pagination 测试分页获取用户列表
func TestTeamUserService_GetList_Pagination(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建 15 个用户
	for i := 0; i < 15; i++ {
		user := &model.TeamUser{
			Username: "user" + string(rune('A'+i)),
			Password: "password",
		}
		service.repo.Create(user)
	}

	// 获取第一页
	result1, err := service.GetList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetList page 1 failed: %v", err)
	}
	if len(result1.List) != 10 {
		t.Errorf("Expected 10 users on page 1, got %d", len(result1.List))
	}

	// 获取第二页
	result2, err := service.GetList(context.Background(), 2, 10)
	if err != nil {
		t.Fatalf("GetList page 2 failed: %v", err)
	}
	if len(result2.List) != 5 {
		t.Errorf("Expected 5 users on page 2, got %d", len(result2.List))
	}
}

// TestTeamUserService_GetList_DefaultPageSize 测试默认分页大小
func TestTeamUserService_GetList_DefaultPageSize(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建 25 个用户
	for i := 0; i < 25; i++ {
		user := &model.TeamUser{
			Username: "user" + string(rune('A'+i)),
			Password: "password",
		}
		service.repo.Create(user)
	}

	// 使用过大的 pageSize，应该被限制为 10
	result, err := service.GetList(context.Background(), 1, 150)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}
	if len(result.List) != 10 {
		t.Errorf("Expected 10 users (max page size), got %d", len(result.List))
	}
}

// TestTeamUserService_GetList_EmptyList 测试空用户列表
func TestTeamUserService_GetList_EmptyList(t *testing.T) {
	service := setupTeamUserService(t)

	result, err := service.GetList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if result.Total != 0 {
		t.Errorf("Expected total 0, got %d", result.Total)
	}
	if len(result.List) != 0 {
		t.Errorf("Expected 0 users, got %d", len(result.List))
	}
}

// TestTeamUserService_Login_Success 测试登录成功
func TestTeamUserService_Login_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建用户 - 使用动态生成的密码哈希
	hashedPassword, err := bcrypt.HashPassword("password")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	user := &model.TeamUser{
		Username: "testuser",
		Password: hashedPassword,
	}
	service.repo.Create(user)

	req := &TeamUserLoginRequest{
		Username: "testuser",
		Password: "password",
	}

	result, err := service.Login(req, "127.0.0.1")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if result.Token == "" {
		t.Error("Expected token to be set")
	}
	if result.User.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", result.User.Username)
	}
}

// TestTeamUserService_Login_UserNotFound 测试用户不存在
func TestTeamUserService_Login_UserNotFound(t *testing.T) {
	service := setupTeamUserService(t)

	req := &TeamUserLoginRequest{
		Username: "nonexistent",
		Password: "password",
	}

	_, err := service.Login(req, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for nonexistent user")
	}
}

// TestTeamUserService_Login_WrongPassword 测试密码错误
func TestTeamUserService_Login_WrongPassword(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建用户 - 使用动态生成的密码哈希
	hashedPassword, err := bcrypt.HashPassword("correctpassword")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	user := &model.TeamUser{
		Username: "testuser",
		Password: hashedPassword,
	}
	service.repo.Create(user)

	req := &TeamUserLoginRequest{
		Username: "testuser",
		Password: "wrongpassword",
	}

	_, err = service.Login(req, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for wrong password")
	}
}

// TestTeamUserService_Login_InactiveUser 测试非活跃用户
func TestTeamUserService_Login_InactiveUser(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建非活跃用户 - 使用动态生成的密码哈希
	// 注意：由于模型中 Status 字段有 default:1 标签，GORM 会忽略零值 (0)
	// 因此需要使用 Update 方法来设置状态为 0
	hashedPassword, err := bcrypt.HashPassword("password")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// 先创建用户 (不设置状态，使用默认值)
	user := &model.TeamUser{
		Username: "testuser",
		Password: hashedPassword,
		Role:     "viewer",
	}
	err = service.repo.Create(user)
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// 使用 Update 单独更新状态字段为 0，这样可以绕过 GORM 的 default 行为
	db.GetDB().Model(&model.TeamUser{}).Where("id = ?", user.ID).Update("status", model.TeamUserStatusInactive)

	// 验证用户已正确创建
	createdUser, err := service.repo.GetByUsername("testuser")
	if err != nil {
		t.Fatalf("Failed to get created user: %v", err)
	}
	t.Logf("Created user status: %d (expected %d)", createdUser.Status, model.TeamUserStatusInactive)

	req := &TeamUserLoginRequest{
		Username: "testuser",
		Password: "password",
	}

	_, err = service.Login(req, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for inactive user")
	}
}

// TestTeamUserService_ChangePassword_Success 测试修改密码成功
func TestTeamUserService_ChangePassword_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建用户 - 使用动态生成的密码哈希
	hashedPassword, err := bcrypt.HashPassword("oldpassword")
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	user := &model.TeamUser{
		Username: "testuser",
		Password: hashedPassword,
	}
	service.repo.Create(user)

	req := &TeamChangePasswordRequest{
		OldPassword: "oldpassword",
		NewPassword: "newpassword123",
	}

	err = service.ChangePassword(user.ID, req, "127.0.0.1")
	if err != nil {
		t.Fatalf("ChangePassword failed: %v", err)
	}

	// 验证密码已更改
	updated, _ := service.repo.GetByID(user.ID)
	if updated.Password == user.Password {
		t.Error("Expected password to be changed")
	}
}

// TestTeamUserService_ChangePassword_WrongOldPassword 测试旧密码错误
func TestTeamUserService_ChangePassword_WrongOldPassword(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建用户
	user := &model.TeamUser{
		Username: "testuser",
		Password: "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
	}
	service.repo.Create(user)

	req := &TeamChangePasswordRequest{
		OldPassword: "wrongpassword",
		NewPassword: "newpassword123",
	}

	err := service.ChangePassword(user.ID, req, "127.0.0.1")
	if err == nil {
		t.Error("Expected error for wrong old password")
	}
}

// TestTeamUserService_ResetPassword_Success 测试重置密码成功
func TestTeamUserService_ResetPassword_Success(t *testing.T) {
	service := setupTeamUserService(t)

	// 创建用户
	user := &model.TeamUser{
		Username: "testuser",
		Password: "oldpassword",
	}
	service.repo.Create(user)

	err := service.ResetPassword(user.ID, "newpassword123", 1, "admin", "127.0.0.1")
	if err != nil {
		t.Fatalf("ResetPassword failed: %v", err)
	}

	// 验证密码已重置
	updated, _ := service.repo.GetByID(user.ID)
	if updated.Password == user.Password {
		t.Error("Expected password to be reset")
	}
}

// TestNewTeamRoleService 测试创建角色服务实例
func TestNewTeamRoleService(t *testing.T) {
	service := NewTeamRoleService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.repo == nil {
		t.Error("Expected repo to be initialized")
	}
}

// TestTeamRoleService_GetList_Success 测试获取角色列表成功
func TestTeamRoleService_GetList_Success(t *testing.T) {
	service := setupTeamRoleService(t)

	// 创建角色
	role1 := &model.TeamRole{
		Code:        "operator",
		Name:        "Operator",
		Permissions: "[]",
	}
	role2 := &model.TeamRole{
		Code:        "supervisor",
		Name:        "Supervisor",
		Permissions: "[]",
	}
	service.repo.Create(role1)
	service.repo.Create(role2)

	roles, err := service.GetList()
	if err != nil {
		t.Fatalf("GetList failed: %v", err)
	}

	if len(roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(roles))
	}
}

// TestTeamRoleService_Create_Success 测试创建角色成功
func TestTeamRoleService_Create_Success(t *testing.T) {
	service := setupTeamRoleService(t)

	req := &CreateRoleRequest{
		Code:        "custom_role",
		Name:        "Custom Role",
		Permissions: []string{"cards.view", "cards.edit"},
	}

	role, err := service.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if role.Code != "custom_role" {
		t.Errorf("Expected code 'custom_role', got '%s'", role.Code)
	}
	if role.Name != "Custom Role" {
		t.Errorf("Expected name 'Custom Role', got '%s'", role.Name)
	}
}

// TestTeamRoleService_Create_DuplicateCode 测试角色编码已存在
func TestTeamRoleService_Create_DuplicateCode(t *testing.T) {
	service := setupTeamRoleService(t)

	// 先创建角色
	existingRole := &model.TeamRole{
		Code:        "existing",
		Name:        "Existing",
		Permissions: "[]",
	}
	service.repo.Create(existingRole)

	req := &CreateRoleRequest{
		Code: "existing",
		Name: "Duplicate",
	}

	_, err := service.Create(context.Background(), req)
	if err == nil {
		t.Error("Expected error for duplicate code")
	}
}

// TestTeamRoleService_Update_Success 测试更新角色成功
func TestTeamRoleService_Update_Success(t *testing.T) {
	service := setupTeamRoleService(t)

	// 创建角色
	role := &model.TeamRole{
		Code:        "custom",
		Name:        "Old Name",
		Permissions: "[]",
		IsSystem:    false,
	}
	service.repo.Create(role)

	req := &UpdateRoleRequest{
		Name:        "New Name",
		Permissions: []string{"cards.view"},
	}

	updated, err := service.Update(context.Background(), role.ID, req)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if updated.Name != "New Name" {
		t.Errorf("Expected name 'New Name', got '%s'", updated.Name)
	}
}

// TestTeamRoleService_Update_SystemRole 测试不能更新系统角色
func TestTeamRoleService_Update_SystemRole(t *testing.T) {
	service := setupTeamRoleService(t)

	// 创建系统角色
	role := &model.TeamRole{
		Code:        "admin",
		Name:        "Admin",
		Permissions: "[]",
		IsSystem:    true,
	}
	service.repo.Create(role)

	req := &UpdateRoleRequest{
		Name: "New Name",
	}

	_, err := service.Update(context.Background(), role.ID, req)
	if err == nil {
		t.Error("Expected error for updating system role")
	}
}

// TestTeamRoleService_Delete_Success 测试删除角色成功
func TestTeamRoleService_Delete_Success(t *testing.T) {
	service := setupTeamRoleService(t)

	// 创建非系统角色
	role := &model.TeamRole{
		Code:        "custom",
		Name:        "Custom",
		Permissions: "[]",
		IsSystem:    false,
	}
	service.repo.Create(role)

	err := service.Delete(context.Background(), role.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, err = service.repo.GetByID(role.ID)
	if err == nil {
		t.Error("Expected role to be deleted")
	}
}

// TestTeamRoleService_Delete_SystemRole 测试不能删除系统角色
func TestTeamRoleService_Delete_SystemRole(t *testing.T) {
	service := setupTeamRoleService(t)

	// 创建系统角色
	role := &model.TeamRole{
		Code:        "admin",
		Name:        "Admin",
		Permissions: "[]",
		IsSystem:    true,
	}
	service.repo.Create(role)

	err := service.Delete(context.Background(), role.ID)
	if err == nil {
		t.Error("Expected error for deleting system role")
	}
}

// TestTeamRoleService_GetPermissions 测试获取所有权限
func TestTeamRoleService_GetPermissions(t *testing.T) {
	service := setupTeamRoleService(t)

	permissions := service.GetPermissions()
	if permissions == nil {
		t.Error("Expected permissions map to be returned")
	}
}

// TestNewPermissionService 测试创建权限服务实例
func TestNewPermissionService(t *testing.T) {
	service := NewPermissionService()
	if service == nil {
		t.Fatal("Expected service instance, got nil")
	}
	if service.roleRepo == nil {
		t.Error("Expected roleRepo to be initialized")
	}
}

// TestPermissionService_CheckPermission_Admin 测试管理员权限
func TestPermissionService_CheckPermission_Admin(t *testing.T) {
	service := setupPermissionService(t)

	hasPermission := service.CheckPermission("admin", "cards.view")
	if !hasPermission {
		t.Error("Expected admin to have all permissions")
	}
}

// TestPermissionService_CheckPermission_WithRole 测试角色权限
func TestPermissionService_CheckPermission_WithRole(t *testing.T) {
	service := setupPermissionService(t)

	// 创建带权限的角色
	permissions := []string{"cards.view", "cards.edit"}
	permissionsJSON, _ := json.Marshal(permissions)
	role := &model.TeamRole{
		Code:        "operator",
		Name:        "Operator",
		Permissions: string(permissionsJSON),
	}
	service.roleRepo.Create(role)

	hasPermission := service.CheckPermission("operator", "cards.view")
	if !hasPermission {
		t.Error("Expected operator to have cards.view permission")
	}
}

// TestPermissionService_CheckPermission_Wildcard 测试通配符权限
func TestPermissionService_CheckPermission_Wildcard(t *testing.T) {
	service := setupPermissionService(t)

	// 创建带通配符权限的角色
	permissions := []string{"cards.*"}
	permissionsJSON, _ := json.Marshal(permissions)
	role := &model.TeamRole{
		Code:        "supervisor",
		Name:        "Supervisor",
		Permissions: string(permissionsJSON),
	}
	service.roleRepo.Create(role)

	hasPermission := service.CheckPermission("supervisor", "cards.create")
	if !hasPermission {
		t.Error("Expected supervisor to have cards.create permission via wildcard")
	}
}

// TestPermissionService_CheckPermission_NoPermission 测试无权限
func TestPermissionService_CheckPermission_NoPermission(t *testing.T) {
	service := setupPermissionService(t)

	// 创建带有限权限的角色
	permissions := []string{"cards.view"}
	permissionsJSON, _ := json.Marshal(permissions)
	role := &model.TeamRole{
		Code:        "viewer",
		Name:        "Viewer",
		Permissions: string(permissionsJSON),
	}
	service.roleRepo.Create(role)

	hasPermission := service.CheckPermission("viewer", "cards.delete")
	if hasPermission {
		t.Error("Expected viewer to not have cards.delete permission")
	}
}

// TestPermissionService_GetUserPermissions_Admin 测试获取管理员权限
func TestPermissionService_GetUserPermissions_Admin(t *testing.T) {
	service := setupPermissionService(t)

	permissions, err := service.GetUserPermissions("admin")
	if err != nil {
		t.Fatalf("GetUserPermissions failed: %v", err)
	}
	if len(permissions) != 1 || permissions[0] != "*" {
		t.Errorf("Expected admin to have ['*'] permissions, got %v", permissions)
	}
}

// TestPermissionService_GetUserPermissions_CustomRole 测试获取自定义角色权限
func TestPermissionService_GetUserPermissions_CustomRole(t *testing.T) {
	service := setupPermissionService(t)

	permissions := []string{"cards.view", "cards.edit", "users.view"}
	permissionsJSON, _ := json.Marshal(permissions)
	role := &model.TeamRole{
		Code:        "operator",
		Name:        "Operator",
		Permissions: string(permissionsJSON),
	}
	service.roleRepo.Create(role)

	result, err := service.GetUserPermissions("operator")
	if err != nil {
		t.Fatalf("GetUserPermissions failed: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(result))
	}
}
