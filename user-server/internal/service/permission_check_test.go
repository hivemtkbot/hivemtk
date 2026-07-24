//go:build permission_check
// +build permission_check

package service

import (
	"testing"
)

// TestRequireRole_P1-6 服务层角色断言测试（P1-6，2026-07 阶段 1 适配三档角色）
//
// 阶段 1 重构后，system_users.role 仅保留 3 档：admin/customer_service/staff。
// manager/viewer 角色通过兼容常量保留，但本测试只覆盖生产 3 档。
func TestRequireRole(t *testing.T) {
	tests := []struct {
		name         string
		operatorRole string
		allowed      []string
		wantErr      bool
	}{
		{name: "admin 满足 admin", operatorRole: "admin", allowed: []string{"admin"}, wantErr: false},
		{name: "admin 满足 customer_service", operatorRole: "admin", allowed: []string{"customer_service", "admin"}, wantErr: false},
		{name: "customer_service 满足 customer_service", operatorRole: "customer_service", allowed: []string{"customer_service"}, wantErr: false},
		{name: "customer_service 不满足 admin-only", operatorRole: "customer_service", allowed: []string{"admin"}, wantErr: true},
		{name: "staff 不满足 admin/cs", operatorRole: "staff", allowed: []string{"admin", "customer_service"}, wantErr: true},
		{name: "空角色拒绝", operatorRole: "", allowed: []string{"admin", "customer_service", "staff"}, wantErr: true},
		{name: "未知角色拒绝", operatorRole: "unknown", allowed: []string{"admin", "customer_service", "staff"}, wantErr: true},
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
	if err := RequireAdmin("customer_service"); err == nil {
		t.Errorf("customer_service should not pass RequireAdmin")
	}
	if err := RequireAdmin("staff"); err == nil {
		t.Errorf("staff should not pass RequireAdmin")
	}
	if err := RequireAdmin(""); err == nil {
		t.Errorf("empty role should not pass RequireAdmin")
	}
}

// TestRequireManagerOrAdmin 测试 manager/customer_service 或 admin
// 阶段 1：customer_service 视为可执行"经理级"操作（与 LegacyTeamUserRoleManager 同等）。
func TestRequireManagerOrAdmin(t *testing.T) {
	if err := RequireManagerOrAdmin("admin"); err != nil {
		t.Errorf("admin should pass, got %v", err)
	}
	if err := RequireManagerOrAdmin("customer_service"); err != nil {
		t.Errorf("customer_service should pass, got %v", err)
	}
	// LegacyTeamUserRoleManager 仍能通过（兼容旧 token）
	if err := RequireManagerOrAdmin("manager"); err != nil {
		t.Errorf("manager should pass for backward compat, got %v", err)
	}
	if err := RequireManagerOrAdmin("staff"); err == nil {
		t.Errorf("staff should not pass")
	}
}

// TestRequireNotViewer 测试非 viewer/staff
// 阶段 1：staff 替代了原 viewer 的语义。
func TestRequireNotViewer(t *testing.T) {
	if err := RequireNotViewer("admin"); err != nil {
		t.Errorf("admin should pass, got %v", err)
	}
	if err := RequireNotViewer("customer_service"); err != nil {
		t.Errorf("customer_service should pass, got %v", err)
	}
	if err := RequireNotViewer("staff"); err == nil {
		t.Errorf("staff should NOT pass RequireNotViewer")
	}
	if err := RequireNotViewer("viewer"); err == nil {
		t.Errorf("viewer should NOT pass RequireNotViewer (legacy)")
	}
}

// TestIsWriteRole 测试角色读写权限判定（阶段 1：customer_service 视为可写，staff 仅读）
func TestIsWriteRole(t *testing.T) {
	if !IsWriteRole("admin") {
		t.Error("admin should be write role")
	}
	if !IsWriteRole("customer_service") {
		t.Error("customer_service should be write role")
	}
	if IsWriteRole("staff") {
		t.Error("staff should NOT be write role")
	}
}

// TestIsReadOnlyRole 测试只读角色（阶段 1：staff 视为只读）
func TestIsReadOnlyRole(t *testing.T) {
	if !IsReadOnlyRole("staff") {
		t.Error("staff should be read-only")
	}
	if !IsReadOnlyRole("viewer") {
		t.Error("viewer should be read-only (legacy)")
	}
	if IsReadOnlyRole("admin") {
		t.Error("admin should NOT be read-only")
	}
	if IsReadOnlyRole("customer_service") {
		t.Error("customer_service should NOT be read-only")
	}
}

// TestAssertCanOperateSystemUser 测试 SystemUser 操作权限（替代原 AssertCanOperateTeamUser）
// 阶段 1：仅 admin 可创建/删除/重置，customer_service 只能更新（不能 create/delete/reset）。
func TestAssertCanOperateSystemUser(t *testing.T) {
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

		// customer_service 仅能更新（不能 create/delete/reset）
		{name: "cs update other", operatorID: 1, operatorRole: "customer_service", action: "update", targetID: 5, wantErr: false},
		{name: "cs update self", operatorID: 5, operatorRole: "customer_service", action: "update", targetID: 5, wantErr: false},
		{name: "cs create 拒绝", operatorID: 1, operatorRole: "customer_service", action: "create", targetID: 0, wantErr: true},
		{name: "cs delete 拒绝", operatorID: 1, operatorRole: "customer_service", action: "delete", targetID: 5, wantErr: true},
		{name: "cs reset 拒绝", operatorID: 1, operatorRole: "customer_service", action: "reset_password", targetID: 5, wantErr: true},

		// staff 任何操作都拒绝
		{name: "staff create 拒绝", operatorID: 1, operatorRole: "staff", action: "create", targetID: 0, wantErr: true},
		{name: "staff update other 拒绝", operatorID: 1, operatorRole: "staff", action: "update", targetID: 5, wantErr: true},
		{name: "staff update self 拒绝", operatorID: 5, operatorRole: "staff", action: "update", targetID: 5, wantErr: true},

		// 空角色
		{name: "空角色拒绝", operatorID: 1, operatorRole: "", action: "create", targetID: 0, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := AssertCanOperateSystemUser(tt.operatorID, tt.operatorRole, tt.action, tt.targetID)
			gotErr := err != nil
			if gotErr != tt.wantErr {
				t.Errorf("AssertCanOperateSystemUser(%d, %q, %q, %d) error = %v, wantErr = %v",
					tt.operatorID, tt.operatorRole, tt.action, tt.targetID, err, tt.wantErr)
			}
		})
	}
}

// TestIsValidSystemUserRoleCode 测试三档角色合法性校验
func TestIsValidSystemUserRoleCode(t *testing.T) {
	valid := []string{"admin", "customer_service", "staff"}
	for _, r := range valid {
		if !IsValidSystemUserRoleCode(r) {
			t.Errorf("role %q should be valid", r)
		}
	}
	invalid := []string{"", "super_admin", "operator", "sales", "user", "manager", "viewer"}
	for _, r := range invalid {
		if IsValidSystemUserRoleCode(r) {
			t.Errorf("role %q should NOT be valid (stage-1 only keeps 3 roles)", r)
		}
	}
}
