package repository

import (
	"context"
	"testing"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupTestRepositoryDB 设置测试数据库
func setupTestRepositoryDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.User{},
	)
	db.SetTestDB(database)
	return database
}

// TestNewUserRepository 测试创建 UserRepository
func TestNewUserRepository(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()
	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
}

// TestUserRepository_Create 测试创建用户
func TestUserRepository_Create(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		RealName: "Test User",
	}

	err := repo.Create(user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if user.ID == "" {
		t.Error("Expected non-empty ID after create")
	}

	// 验证密码已被加密
	if user.Password == "password123" {
		t.Error("Expected password to be hashed")
	}
}

// TestUserRepository_GetByID 测试根据 ID 获取用户
func TestUserRepository_GetByID(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}
	repo.Create(user)

	// 根据 ID 获取
	fetchedUser, err := repo.GetByID(user.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if fetchedUser.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got %s", fetchedUser.Username)
	}
}

// TestUserRepository_GetByID_NotFound 测试获取不存在的用户
func TestUserRepository_GetByID_NotFound(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	_, err := repo.GetByID"non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

// TestUserRepository_GetByUsername 测试根据用户名获取用户
func TestUserRepository_GetByUsername(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}
	repo.Create(user)

	// 根据用户名获取
	fetchedUser, err := repo.GetByUsername(context.Background(), "testuser")
	if err != nil {
		t.Fatalf("GetByUsername failed: %v", err)
	}

	if fetchedUser.ID != user.ID {
		t.Errorf("Expected ID %s, got %s", user.ID, fetchedUser.ID)
	}
}

// TestUserRepository_GetByUsername_NotFound 测试获取不存在的用户
func TestUserRepository_GetByUsername_NotFound(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	_, err := repo.GetByUsername(context.Background(), "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent user")
	}
}

// TestUserRepository_GetUserList 测试获取用户列表
func TestUserRepository_GetUserList(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 创建多个用户
	for i := 0; i < 5; i++ {
		user := &model.User{
			Username: "user" + string(rune('0'+i)),
			Password: "password123",
			Email:    "user" + string(rune('0'+i)) + "@example.com",
		}
		repo.Create(user)
	}

	// 获取第一页
	users, total, err := repo.GetUserList(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("GetUserList failed: %v", err)
	}

	if total != 5 {
		t.Errorf("Expected total 5, got %d", total)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users, got %d", len(users))
	}
}

// TestUserRepository_GetUserList_Pagination 测试分页
func TestUserRepository_GetUserList_Pagination(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 创建多个用户
	for i := 0; i < 10; i++ {
		user := &model.User{
			Username: "user" + string(rune('0'+i)),
			Password: "password123",
			Email:    "user" + string(rune('0'+i)) + "@example.com",
		}
		repo.Create(user)
	}

	// 获取第一页（每页 5 条）
	users, total, err := repo.GetUserList(context.Background(), 1, 5)
	if err != nil {
		t.Fatalf("GetUserList failed: %v", err)
	}

	if total != 10 {
		t.Errorf("Expected total 10, got %d", total)
	}

	if len(users) != 5 {
		t.Errorf("Expected 5 users on first page, got %d", len(users))
	}

	// 获取第二页
	users2, total2, err := repo.GetUserList(context.Background(), 2, 5)
	if err != nil {
		t.Fatalf("GetUserList page 2 failed: %v", err)
	}

	if total2 != 10 {
		t.Errorf("Expected total 10, got %d", total2)
	}

	if len(users2) != 5 {
		t.Errorf("Expected 5 users on second page, got %d", len(users2))
	}
}

// TestUserRepository_Delete 测试删除用户
func TestUserRepository_Delete(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}
	repo.Create(user)

	// 删除用户
	err := repo.Deleteuser.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证用户已被删除
	_, err = repo.GetByID(user.ID)
	if err == nil {
		t.Error("Expected error after delete")
	}
}

// TestUserRepository_Update 测试更新用户
func TestUserRepository_Update(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
		RealName: "Test User",
	}
	repo.Create(user)

	// 更新用户
	user.RealName = "Updated Name"
	err := repo.Updateuser)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// 验证更新
	fetchedUser, _ := repo.GetByID(user.ID)
	if fetchedUser.RealName != "Updated Name" {
		t.Errorf("Expected RealName 'Updated Name', got %s", fetchedUser.RealName)
	}
}

// TestUserRepository_UpdatePassword 测试更新密码
func TestUserRepository_UpdatePassword(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "oldpassword",
		Email:    "test@example.com",
	}
	repo.Create(user)
	oldHashedPassword := user.Password

	// 更新密码
	hashedPassword := "hashed_newpassword123" // 实际应该先哈希，这里直接传入哈希值
	err := repo.UpdatePassword(context.Background(), user.ID, hashedPassword)
	if err != nil {
		t.Fatalf("UpdatePassword failed: %v", err)
	}

	// 验证密码已更新
	fetchedUser, _ := repo.GetByID(user.ID)
	if fetchedUser.Password == oldHashedPassword {
		t.Error("Expected password to be updated")
	}
	if fetchedUser.Password != hashedPassword {
		t.Error("Expected password to match new password")
	}
}

// TestUserRepository_UserIsExist 测试用户是否存在
func TestUserRepository_UserIsExist(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username:  "testuser",
		Password:  "password123",
		Email:     "test@example.com",
		AccountID: "account123",
		TgID:      12345,
		FirstName: "John",
		LastName:  "Doe",
		UserName:  "johndoe",
	}
	repo.Create(user)

	// 检查用户是否存在
	id, exists := repo.UserIsExist(context.Background(), "account123", 12345, "John", "Doe", "johndoe")
	if !exists {
		t.Error("Expected user to exist")
	}
	if id != user.ID {
		t.Errorf("Expected ID %s, got %s", user.ID, id)
	}

	// 检查不存在的用户
	_, exists = repo.UserIsExist(context.Background(), "nonexistent", 0, "Jane", "Doe", "janedoe")
	if exists {
		t.Error("Expected user to not exist")
	}
}

// TestUserRepository_UsernameExists 测试用户名是否存在
func TestUserRepository_UsernameExists(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}
	repo.Create(user)

	// 检查用户名是否存在
	exists, err := repo.UsernameExists(context.Background(), "testuser", "")
	if err != nil {
		t.Fatalf("UsernameExists failed: %v", err)
	}
	if !exists {
		t.Error("Expected username to exist")
	}

	// 检查不存在的用户名
	exists, err = repo.UsernameExists(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("UsernameExists failed: %v", err)
	}
	if exists {
		t.Error("Expected username to not exist")
	}

	// 排除自己
	exists, err = repo.UsernameExists(context.Background(), "testuser", user.ID)
	if err != nil {
		t.Fatalf("UsernameExists failed: %v", err)
	}
	if exists {
		t.Error("Expected username to not exist when excluding self")
	}
}

// TestUserRepository_EmailExists 测试邮箱是否存在
func TestUserRepository_EmailExists(t *testing.T) {
	setupTestRepositoryDB(t)

	repo := NewUserRepository()

	// 先创建用户
	user := &model.User{
		Username: "testuser",
		Password: "password123",
		Email:    "test@example.com",
	}
	repo.Create(user)

	// 检查邮箱是否存在
	exists, err := repo.EmailExists(context.Background(), "test@example.com", "")
	if err != nil {
		t.Fatalf("EmailExists failed: %v", err)
	}
	if !exists {
		t.Error("Expected email to exist")
	}

	// 检查不存在的邮箱
	exists, err = repo.EmailExists(context.Background(), "nonexistent@example.com", "")
	if err != nil {
		t.Fatalf("EmailExists failed: %v", err)
	}
	if exists {
		t.Error("Expected email to not exist")
	}

	// 排除自己
	exists, err = repo.EmailExists(context.Background(), "test@example.com", user.ID)
	if err != nil {
		t.Fatalf("EmailExists failed: %v", err)
	}
	if exists {
		t.Error("Expected email to not exist when excluding self")
	}
}
