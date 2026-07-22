package service

import (
	"context"
	"errors"

	"marketing/internal/model"
)

// Service 层权限检查工具（P1-6）
//
// 设计目的：
//   - 不依赖 controller/middleware 上下文也能进行权限断言
//   - 避免业务 Service 误以为"前端已校验"就跳过权限判断
//   - 当 controller 直接调用 Service 而不经过中间件时，仍能保证安全
//
// 使用方法：
//   - 在 Service 方法开始时调用 RequireRole / RequirePermission
//   - operatorRole 由 controller 从 JWT claims 透传
//   - 若 operatorRole 为空，表示调用方未提供角色信息，直接拒绝

// ErrServicePermissionDenied Service 层权限不足
var ErrServicePermissionDenied = errors.New("无权限执行此操作")

// ErrServiceRoleMissing Service 层缺少角色信息
var ErrServiceRoleMissing = errors.New("缺少操作者角色信息")

// TeamUserRole TeamUser 角色常量（与 model.SystemRoles 对齐，避免硬编码）
const (
	TeamUserRoleAdmin   = "admin"
	TeamUserRoleManager = "manager"
	TeamUserRoleViewer  = "viewer"
)

// IsValidTeamUserRole 校验 TeamUser 角色是否合法
func IsValidTeamUserRole(role string) bool {
	for _, r := range model.SystemRoles {
		if r.Code == role {
			return true
		}
	}
	return false
}

// RequireRole 要求操作者具备指定角色之一（任一即可）
//   - 严格模式：operatorRole 为空直接拒绝（防止绕过）
//   - 多角色匹配：admin/manager/viewer 任意一个满足即可
func RequireRole(operatorRole string, allowed ...string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	for _, r := range allowed {
		if operatorRole == r {
			return nil
		}
	}
	return ErrServicePermissionDenied
}

// RequireAdmin 要求操作者是 admin
func RequireAdmin(operatorRole string) error {
	return RequireRole(operatorRole, TeamUserRoleAdmin)
}

// RequireManagerOrAdmin 要求操作者是 manager 或 admin
func RequireManagerOrAdmin(operatorRole string) error {
	return RequireRole(operatorRole, TeamUserRoleAdmin, TeamUserRoleManager)
}

// RequireNotViewer 要求操作者不是 viewer（仅 admin/manager 可执行）
func RequireNotViewer(operatorRole string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	if operatorRole == TeamUserRoleViewer {
		return ErrServicePermissionDenied
	}
	return nil
}

// RequirePermission 要求操作者具备指定权限
//   - 使用 PermissionService.CheckPermission
//   - 管理员（admin）默认放行
func RequirePermission(operatorRole, permission string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	ps := NewPermissionService()
	if !ps.CheckPermission(context.Background(), operatorRole, permission) {
		return ErrServicePermissionDenied
	}
	return nil
}

// IsWriteRole 判断角色是否具备写权限
//   - admin/manager 可写
//   - viewer 仅读
func IsWriteRole(role string) bool {
	return role == TeamUserRoleAdmin || role == TeamUserRoleManager
}

// IsReadOnlyRole 判断角色是否仅可读
func IsReadOnlyRole(role string) bool {
	return role == TeamUserRoleViewer
}

// AssertCanOperateTeamUser 断言操作者有权操作 TeamUser
// 规则：
//   - 仅 admin 可以创建/删除/重置其他 TeamUser
//   - manager 可以更新其他人的基本资料（除角色外）
//   - viewer 不能操作其他用户，也不能 update 自己的 profile
//     （viewer 仅能通过 /api/team/user/change-password 改密）
//
// operatorID: 操作者 ID
// operatorRole: 操作者角色
// targetID: 目标用户 ID（0 表示创建）
// action: 操作类型（create/update/delete/reset_password/change_password）
func AssertCanOperateTeamUser(operatorID uint, operatorRole, action string, targetID uint) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	// viewer 在任何场景下都不能进行 CRUD/重置（仅 change_password 走 /api/team/user/change-password）
	if operatorRole == TeamUserRoleViewer {
		return ErrServicePermissionDenied
	}
	// admin 拥有所有操作权限
	if operatorRole == TeamUserRoleAdmin {
		return nil
	}
	// manager 仅能更新（不能 create/delete/reset）
	if operatorRole == TeamUserRoleManager {
		switch action {
		case "update":
			return nil
		default:
			return ErrServicePermissionDenied
		}
	}
	return ErrServicePermissionDenied
}
