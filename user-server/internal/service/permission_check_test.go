//go:build permission_check
// +build permission_check

package service

import (
	"testing"
)

// TestRequireRole_P1-6 服务层角色断言测试
func TestRequireRole(t *testing.T) {
	tests := []struct {
		name         string
		operatorRole string
		allowed      []string
		wantErr      bool
	}{
		{name: "admin 满足 admin", operatorRole: "admin", allowed: []string{"admin"}, wantErr: false},
		{name: "admin 满足 manager", operatorRole: "admin", allowed: []string{"manager", "admin"}, wantErr: false},
		{name: "manager 满足 manager", operatorRole: "manager", allowed: []string{"manager"}, wantErr: false},
		{name: "manager 不满足 admin-only", operatorRole: "manager", allowed: []string{"admin"}, wantErr: true},
		{name: "viewer 不满足 admin/manager", operatorRole: "viewer", allowed: []string{"admin", "manager"}, wantErr: true},
		{name: "空角色拒绝", operatorRole: "", allowed: []string{"admin", "manager", "viewer"}, wantErr: true},
		{name: "未知角色拒绝", operatorRole: "unknown", allowed: []string{"admin", "manager", "viewer"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RequireRole(tt.operatorRole, tt.allowed...)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("RequireRole(%q, %v) error = %v, wantErr = %v", tt.operatorRole, tt.allowed, err, tt.wantErr)
			}
		})
	}
}

// TestRequireAdmin 测试仅 admin
func TestRequireAdmin(t *testing.T) {
	if err := RequireAdmin("admin"); err != nil {
		t.Errorf("admin should pass, got %v", err)
	}
	if err := RequireAdmin("manager"); err == nil {
		t.Errorf("manager should not pass RequireAdmin")
	}
	if err := RequireAdmin("viewer"); err == nil {
		t.Errorf("viewer should not pass RequireAdmin")
	}
	if err := RequireAdmin(""); err == nil {
		t.Errorf("empty role should not pass RequireAdmin")
	}
}

// TestRequireManagerOrAdmin 测试 manager 或 admin
func TestRequireManagerOrAdmin(t *testing.T) {
	if err := RequireManagerOrAdmin("admin"); err != nil {
		t.Errorf("admin should pass, got %v", err)
	}
	if err := RequireManagerOrAdmin("manager"); err != nil {
		t.Errorf("manager should pass, got %v", err)
	}
	if err := RequireManagerOrAdmin("viewer"); err == nil {
		t.Errorf("viewer should not pass")
	}
}

// TestRequireNotViewer 测试非 viewer
func TestRequireNotViewer(t *testing.T) {
	if err := RequireNotViewer("admin"); err != nil {
		t.Errorf("admin should pass, got %v", err)
	}
	if err := RequireNotViewer("manager"); err != nil {
		t.Errorf("manager should pass, got %v", err)
	}
	if err := RequireNotViewer("viewer"); err == nil {
		t.Errorf("viewer should not pass")
	}
}

// TestIsWriteRole 测试角色读写权限判定
func TestIsWriteRole(t *testing.T) {
	if !IsWriteRole("admin") {
		t.Error("admin should be write role")
	}
	if !IsWriteRole("manager") {
		t.Error("manager should be write role")
	}
	if IsWriteRole("viewer") {
		t.Error("viewer should NOT be write role")
	}
}

// TestIsReadOnlyRole 测试只读角色
func TestIsReadOnlyRole(t *testing.T) {
	if !IsReadOnlyRole("viewer") {
		t.Error("viewer should be read-only")
	}
	if IsReadOnlyRole("admin") {
		t.Error("admin should NOT be read-only")
	}
	if IsReadOnlyRole("manager") {
		t.Error("manager should NOT be read-only")
	}
}

// TestAssertCanOperateTeamUser 测试 TeamUser 操作权限
func TestAssertCanOperateTeamUser(t *testing.T) {
	tests := []struct {
		name         string
		operatorID   uint
		operatorRole string
		action       string
		targetID     uint
		wantErr      bool
	}{
		// admin 可以做任何操作
		{name: "admin create", operatorID: 1, operatorRole: "admin", action: "create", targetID: 0, wantErr: false},
		{name: "admin delete other", operatorID: 1, operatorRole: "admin", action: "delete", targetID: 5, wantErr: false},
		{name: "admin reset_password other", operatorID: 1, operatorRole: "admin", action: "reset_password", targetID: 5, wantErr: false},

		// manager 仅能更新（不能 create/delete/reset）
		{name: "manager update other", operatorID: 1, operatorRole: "manager", action: "update", targetID: 5, wantErr: false},
		{name: "manager update self", operatorID: 5, operatorRole: "manager", action: "update", targetID: 5, wantErr: false},
		{name: "manager create 拒绝", operatorID: 1, operatorRole: "manager", action: "create", targetID: 0, wantErr: true},
		{name: "manager delete 拒绝", operatorID: 1, operatorRole: "manager", action: "delete", targetID: 5, wantErr: true},
		{name: "manager reset 拒绝", operatorID: 1, operatorRole: "manager", action: "reset_password", targetID: 5, wantErr: true},
		{name: "manager delete self 拒绝", operatorID: 5, operatorRole: "manager", action: "delete", targetID: 5, wantErr: true},

		// viewer 任何操作都拒绝
		{name: "viewer create 拒绝", operatorID: 1, operatorRole: "viewer", action: "create", targetID: 0, wantErr: true},
		{name: "viewer update other 拒绝", operatorID: 1, operatorRole: "viewer", action: "update", targetID: 5, wantErr: true},
		{name: "viewer update self 拒绝", operatorID: 5, operatorRole: "viewer", action: "update", targetID: 5, wantErr: true},

		// 空角色
		{name: "空角色拒绝", operatorID: 1, operatorRole: "", action: "create", targetID: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AssertCanOperateTeamUser(tt.operatorID, tt.operatorRole, tt.action, tt.targetID)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("AssertCanOperateTeamUser(%d, %q, %q, %d) error = %v, wantErr = %v",
					tt.operatorID, tt.operatorRole, tt.action, tt.targetID, err, tt.wantErr)
			}
		})
	}
}

// TestIsValidTeamUserRole 测试角色合法性校验
func TestIsValidTeamUserRole(t *testing.T) {
	valid := []string{"admin", "manager", "viewer"}
	for _, r := range valid {
		if !IsValidTeamUserRole(r) {
			t.Errorf("role %q should be valid", r)
		}
	}
	invalid := []string{"", "super_admin", "operator", "sales", "user"}
	for _, r := range invalid {
		if IsValidTeamUserRole(r) {
			t.Errorf("role %q should NOT be valid", r)
		}
	}
}

// TestIsValidSystemUserRole 已在 model/system_user_test.go 中覆盖
// 删除此重复测试（项目规则"不允许跳过"，重复测试应删除而非 t.Skip）
