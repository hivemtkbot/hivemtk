package model

import (
	"testing"
	"time"
)

func TestTeamUserStatus_Constants(t *testing.T) {
	expectedValues := map[TeamUserStatus]int{
		TeamUserStatusInactive: 0,
		TeamUserStatusActive:   1,
	}

	for status, expected := range expectedValues {
		if int(status) != expected {
			t.Errorf("Expected TeamUserStatus value %d, got %d", expected, status)
		}
	}
}

func TestTeamUser_TableName(t *testing.T) {
	user := &TeamUser{}
	tableName := user.TableName()
	if tableName != "team_users" {
		t.Errorf("Expected table name 'team_users', got %s", tableName)
	}
}

func TestTeamUser_BasicFields(t *testing.T) {
	now := time.Now()

	user := &TeamUser{
		ID:          1,
		Username:    "testuser",
		Password:    "hashed_password",
		Name:        "Test User",
		Email:       "test@example.com",
		Phone:       "13800138000",
		Avatar:      "https://example.com/avatar.jpg",
		Role:        "admin",
		Status:      TeamUserStatusActive,
		LastLoginAt: &now,
		LastLoginIP: "192.168.1.1",
	}

	if user.ID != 1 {
		t.Errorf("Expected ID 1, got %d", user.ID)
	}
	if user.Username != "testuser" {
		t.Errorf("Expected Username 'testuser', got %s", user.Username)
	}
	if user.Name != "Test User" {
		t.Errorf("Expected Name 'Test User', got %s", user.Name)
	}
	if user.Email != "test@example.com" {
		t.Errorf("Expected Email 'test@example.com', got %s", user.Email)
	}
	if user.Role != "admin" {
		t.Errorf("Expected Role 'admin', got %s", user.Role)
	}
	if user.Status != TeamUserStatusActive {
		t.Errorf("Expected Status TeamUserStatusActive, got %d", user.Status)
	}
}

func TestTeamUser_DefaultValues(t *testing.T) {
	user := &TeamUser{}

	if user.Role != "" {
		t.Logf("Role is %s (expected empty before save)", user.Role)
	}
	if user.Status != 0 {
		t.Logf("Status is %d (expected 0 before save)", user.Status)
	}
}

func TestTeamUser_RoleValues(t *testing.T) {
	roles := []string{"admin", "manager", "viewer"}

	for _, role := range roles {
		user := &TeamUser{
			Role: role,
		}
		if user.Role != role {
			t.Errorf("Expected Role %s, got %s", role, user.Role)
		}
	}
}

func TestTeamUser_WithNilLastLogin(t *testing.T) {
	user := &TeamUser{
		Username:    "newuser",
		LastLoginAt: nil,
	}

	if user.LastLoginAt != nil {
		t.Errorf("Expected LastLoginAt to be nil, got %v", user.LastLoginAt)
	}
}

func TestTeamRole_TableName(t *testing.T) {
	role := &TeamRole{}
	tableName := role.TableName()
	if tableName != "team_roles" {
		t.Errorf("Expected table name 'team_roles', got %s", tableName)
	}
}

func TestTeamRole_BasicFields(t *testing.T) {
	role := &TeamRole{
		ID:          1,
		Code:        "custom_role",
		Name:        "Custom Role",
		Permissions: `["users.view", "cards.edit"]`,
		IsSystem:    false,
		Status:      1,
	}

	if role.ID != 1 {
		t.Errorf("Expected ID 1, got %d", role.ID)
	}
	if role.Code != "custom_role" {
		t.Errorf("Expected Code 'custom_role', got %s", role.Code)
	}
	if role.Name != "Custom Role" {
		t.Errorf("Expected Name 'Custom Role', got %s", role.Name)
	}
	if role.IsSystem {
		t.Error("Expected IsSystem to be false")
	}
}

func TestTeamRole_SystemRoles(t *testing.T) {
	if len(SystemRoles) != 3 {
		t.Errorf("Expected 3 SystemRoles, got %d", len(SystemRoles))
	}

	roleCodes := []string{"admin", "manager", "viewer"}
	for i, code := range roleCodes {
		if SystemRoles[i].Code != code {
			t.Errorf("Expected SystemRole[%d].Code to be %s, got %s", i, code, SystemRoles[i].Code)
		}
	}
}

func TestSystemPermissions(t *testing.T) {
	expectedPermissions := []string{
		"users.view", "users.create", "users.update", "users.delete",
		"roles.view", "roles.create", "roles.update", "roles.delete",
		"cards.view", "cards.create", "cards.update", "cards.delete", "cards.*",
		"shortlinks.view", "shortlinks.create", "shortlinks.update", "shortlinks.delete", "shortlinks.*",
		"livecodes.view", "livecodes.create", "livecodes.update", "livecodes.delete", "livecodes.*",
		"clues.view", "clues.create", "clues.update", "clues.delete", "clues.*",
		"autoreply.view", "autoreply.create", "autoreply.update", "autoreply.delete", "autoreply.*",
		"system.config", "system.logs", "system.backup",
		"*",
	}

	for _, perm := range expectedPermissions {
		if _, exists := SystemPermissions[perm]; !exists {
			t.Errorf("Expected SystemPermissions to contain %s", perm)
		}
	}
}

func TestOperationLog_TableName(t *testing.T) {
	log := &OperationLog{}
	tableName := log.TableName()
	if tableName != "operation_logs" {
		t.Errorf("Expected table name 'operation_logs', got %s", tableName)
	}
}

func TestOperationLog_BasicFields(t *testing.T) {
	log := &OperationLog{
		ID:         1,
		UserID:     100,
		Username:   "testuser",
		Action:     "create",
		Module:     "user",
		Resource:   "TeamUser",
		ResourceID: "1",
		Detail:     `{"name": "new user"}`,
		OldValue:   "",
		NewValue:   `{"name": "new user"}`,
		IP:         "192.168.1.1",
		UserAgent:  "Mozilla/5.0",
	}

	if log.ID != 1 {
		t.Errorf("Expected ID 1, got %d", log.ID)
	}
	if log.Action != "create" {
		t.Errorf("Expected Action 'create', got %s", log.Action)
	}
	if log.Module != "user" {
		t.Errorf("Expected Module 'user', got %s", log.Module)
	}
	if log.Resource != "TeamUser" {
		t.Errorf("Expected Resource 'TeamUser', got %s", log.Resource)
	}
}

func TestOperationLog_ActionValues(t *testing.T) {
	actions := []string{"create", "update", "delete", "login", "logout"}

	for _, action := range actions {
		log := &OperationLog{
			Action: action,
		}
		if log.Action != action {
			t.Errorf("Expected Action %s, got %s", action, log.Action)
		}
	}
}

func TestGenerateUUID(t *testing.T) {
	uuid1 := GenerateUUID()
	uuid2 := GenerateUUID()

	if uuid1 == "" {
		t.Error("Expected GenerateUUID() to return non-empty string")
	}
	if uuid1 == uuid2 {
		t.Error("Expected GenerateUUID() to return unique values")
	}
	if len(uuid1) != 36 {
		t.Errorf("Expected UUID length 36, got %d", len(uuid1))
	}
}

func TestTeamUser_BeforeCreate(t *testing.T) {
	user := &TeamUser{
		Username: "testuser",
	}

	// BeforeCreate returns nil (no-op implementation)
	err := user.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestTeamRole_BeforeCreate(t *testing.T) {
	role := &TeamRole{
		Code: "test_role",
		Name: "Test Role",
	}

	// BeforeCreate returns nil (no-op implementation)
	err := role.BeforeCreate(nil)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
