package repository

import (
	"context"
	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"testing"

	"gorm.io/gorm"
	"marketing/internal/pkg/testutil"
)

// setupTeamUserTestDB 设置团队用户测试数据库
func setupTeamUserTestDB(t *testing.T) *gorm.DB {
	database := testutil.NewTestDB(t,
		&model.TeamUser{},
		&model.TeamRole{},
		&model.OperationLog{},
	)
	db.SetTestDB(database)
	return database
}

// setupTeamUserRepositories 创建测试用的仓库实例
func setupTeamUserRepositories(t *testing.T) (TeamUserRepository, TeamRoleRepository, OperationLogRepository) {
	setupTeamUserTestDB(t)

	userRepo := NewTeamUserRepository()
	roleRepo := NewTeamRoleRepository()
	logRepo := NewOperationLogRepository()

	return userRepo, roleRepo, logRepo
}

// TestTeamUserRepository_Create 测试创建团队用户
func TestTeamUserRepository_Create(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	tests := []struct {
		name    string
		user    *model.TeamUser
		wantErr bool
	}{
		{
			name: "create user success",
			user: &model.TeamUser{
				Username: "testuser",
				Password: "password123",
				Name:     "Test User",
				Email:    "test@example.com",
				Role:     "admin",
				Status:   1,
			},
			wantErr: false,
		},
		{
			name: "create user with minimal fields",
			user: &model.TeamUser{
				Username: "minimal",
				Password: "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := userRepo.Create(context.Background(), tt.user)

			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && tt.user.ID == 0 {
				t.Error("Expected user ID to be set after creation")
			}
		})
	}
}

// TestTeamUserRepository_GetByID 测试根据 ID 获取用户
func TestTeamUserRepository_GetByID(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	// 创建测试数据
	user := &model.TeamUser{
		Username: "getbyid",
		Password: "pass",
		Name:     "GetByID User",
	}
	userRepo.Create(context.Background(), user)

	tests := []struct {
		name    string
		id      uint
		wantErr bool
	}{
		{
			name:    "get existing user",
			id:      user.ID,
			wantErr: false,
		},
		{
			name:    "get non-existing user",
			id:      99999,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := userRepo.GetByID(context.Background(), tt.id)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && result.Name != "GetByID User" {
				t.Errorf("Expected name 'GetByID User', got '%s'", result.Name)
			}
		})
	}
}

func TestTeamUserRepository_Update(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	// 创建测试数据
	user := &model.TeamUser{
		Username: "updateuser",
		Password: "pass",
		Name:     "Original Name",
	}
	userRepo.Create(context.Background(), user)

	user.Name = "Updated Name"
	user.Email = "updated@example.com"

	err := userRepo.Update(context.Background(), user)
	if err != nil {
		t.Errorf("Update() error = %v", err)
	}

	updated, _ := userRepo.GetByID(context.Background(), user.ID)
	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
}

// TestTeamUserRepository_Delete 测试删除用户
func TestTeamUserRepository_Delete(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	// 创建测试数据
	user := &model.TeamUser{
		Username: "deleteuser",
		Password: "pass",
	}
	userRepo.Create(context.Background(), user)

	err := userRepo.Delete(context.Background(), user.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
	}

	_, err = userRepo.GetByID(context.Background(), user.ID)
	if err == nil {
		t.Error("Expected user to be deleted")
	}
}

// TestTeamUserRepository_UpdateLastLogin 测试更新最后登录信息
func TestTeamUserRepository_UpdateLastLogin(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	// 创建测试数据
	user := &model.TeamUser{
		Username: "loginuser",
		Password: "pass",
	}
	userRepo.Create(context.Background(), user)

	err := userRepo.UpdateLastLogin(context.Background(), user.ID, "192.168.1.1")
	if err != nil {
		t.Errorf("UpdateLastLogin() error = %v", err)
	}

	updated, _ := userRepo.GetByID(context.Background(), user.ID)
	if updated.LastLoginIP != "192.168.1.1" {
		t.Errorf("Expected LastLoginIP '192.168.1.1', got '%s'", updated.LastLoginIP)
	}
	if updated.LastLoginAt.IsZero() {
		t.Error("Expected LastLoginAt to be updated")
	}
}

// TestTeamUserRepository_UsernameExists 测试用户名是否存在
func TestTeamUserRepository_UsernameExists(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	userRepo.Create(context.Background(), &model.TeamUser{
		Username: "existinguser",
		Password: "pass",
	})

	exists, _ := userRepo.UsernameExists(context.Background(), "existinguser", 0)
	if !exists {
		t.Error("Expected username to exist")
	}

	exists, _ = userRepo.UsernameExists(context.Background(), "nonexistent", 0)
	if exists {
		t.Error("Expected username to not exist")
	}
}

// TestTeamUserRepository_EmailExists 测试邮箱是否存在
func TestTeamUserRepository_EmailExists(t *testing.T) {
	userRepo, _, _ := setupTeamUserRepositories(t)

	// 创建测试数据
	userRepo.Create(context.Background(), &model.TeamUser{
		Username: "testuser",
		Password: "pass",
		Email:    "test@example.com",
	})

	exists, _ := userRepo.EmailExists(context.Background(), "test@example.com", 0)
	if !exists {
		t.Error("Expected email to exist")
	}

	exists, _ = userRepo.EmailExists(context.Background(), "other@example.com", 0)
	if exists {
		t.Error("Expected email to not exist")
	}

	// 测试空邮箱
	exists, _ = userRepo.EmailExists(context.Background(), "", 0)
	if exists {
		t.Error("Expected empty email to return false")
	}
}
