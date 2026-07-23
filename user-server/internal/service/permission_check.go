package service

// permission_check.go SystemUser 权限检查工具（2026-07 阶段 1 重构：单表化 system_users）
//
// 五层架构归属: L3 业务服务层
// 设计依据：
//   - 原 service/permission_check.go 依赖 model.SystemRoles（已废弃）
//   - 原 service/team_user.go 内的 PermissionService 已迁移至本文件
//   - 角色定义：system_users.role 仅有 3 档 admin/customer_service/staff
//
// 使用场景：
//   - middleware/permission.go 通过 NewPermissionService().CheckPermission 实现细粒度权限
//   - Service 业务方法通过 RequireRole/RequireAdmin 等做断言
//   - 兼容历史 AssertCanOperateTeamUser（语义改为基于 system_user）

import (
	"context"
	"errors"
	"strings"
)

// SystemUser 角色常量（与 system_users.role CHECK 约束对齐）
const (
	SystemUserRoleAdmin           = "admin"
	SystemUserRoleCustomerService = "customer_service"
	SystemUserRoleStaff           = "staff"
)

// 兼容旧 TeamUser 角色名（阶段 1 过渡期仍可使用，避免破坏性变更）
const (
	LegacyTeamUserRoleAdmin   = "admin"
	LegacyTeamUserRoleManager = "manager"
	LegacyTeamUserRoleViewer  = "viewer"
)

// ErrServicePermissionDenied Service 层权限不足
var ErrServicePermissionDenied = errors.New("无权限执行此操作")

// ErrServiceRoleMissing Service 层缺少角色信息
var ErrServiceRoleMissing = errors.New("缺少操作者角色信息")

// IsValidSystemUserRoleCode 校验 system_user 角色是否合法（3 档）
func IsValidSystemUserRoleCode(role string) bool {
	return role == SystemUserRoleAdmin ||
		role == SystemUserRoleCustomerService ||
		role == SystemUserRoleStaff
}

// RequireRole 要求操作者具备指定角色之一（任一即可）
//   - 严格模式：operatorRole 为空直接拒绝（防止绕过）
//   - 多角色匹配：传入的 allowed 任意一个满足即可
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
	return RequireRole(operatorRole, SystemUserRoleAdmin)
}

// RequireManagerOrAdmin 要求操作者是 manager/admin/customer_service 之一
//
// 阶段 1：因 system_users.role 仅有 3 档，customer_service 视为可执行"经理级"操作。
// 兼容：旧 manager 角色 token 仍能通过（按历史契约）。
func RequireManagerOrAdmin(operatorRole string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	switch operatorRole {
	case SystemUserRoleAdmin, SystemUserRoleCustomerService, LegacyTeamUserRoleManager:
		return nil
	default:
		return ErrServicePermissionDenied
	}
}

// RequireNotViewer 要求操作者不是 viewer/staff（仅 admin/manager/customer_service 可执行）
func RequireNotViewer(operatorRole string) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	if operatorRole == LegacyTeamUserRoleViewer || operatorRole == SystemUserRoleStaff {
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
//   - admin/customer_service 可写
//   - staff 仅读
func IsWriteRole(role string) bool {
	return role == SystemUserRoleAdmin || role == SystemUserRoleCustomerService
}

// IsReadOnlyRole 判断角色是否仅可读
func IsReadOnlyRole(role string) bool {
	return role == SystemUserRoleStaff || role == LegacyTeamUserRoleViewer
}

// AssertCanOperateSystemUser 断言操作者有权操作 SystemUser
// 规则：
//   - 仅 admin 可以创建/删除/重置其他 SystemUser
//   - customer_service/manager 可以更新其他人的基本资料（除角色外）
//   - staff/viewer 不能操作其他用户
//
// operatorID: 操作者 ID
// operatorRole: 操作者角色
// targetID: 目标用户 ID（0 表示创建）
// action: 操作类型（create/update/delete/reset_password/change_password）
func AssertCanOperateSystemUser(operatorID uint, operatorRole, action string, targetID uint) error {
	if operatorRole == "" {
		return ErrServiceRoleMissing
	}
	// staff/viewer 在任何场景下都不能进行 CRUD/重置
	if operatorRole == SystemUserRoleStaff || operatorRole == LegacyTeamUserRoleViewer {
		return ErrServicePermissionDenied
	}
	// admin 拥有所有操作权限
	if operatorRole == SystemUserRoleAdmin {
		return nil
	}
	// customer_service/manager 仅能更新（不能 create/delete/reset）
	if operatorRole == SystemUserRoleCustomerService || operatorRole == LegacyTeamUserRoleManager {
		switch action {
		case "update":
			return nil
		default:
			return ErrServicePermissionDenied
		}
	}
	return ErrServicePermissionDenied
}

// AssertCanOperateTeamUser 旧名兼容（委托给 AssertCanOperateSystemUser）
// 保留原因：service/team_user.go 之外的旧调用方可能仍使用此函数
func AssertCanOperateTeamUser(operatorID uint, operatorRole, action string, targetID uint) error {
	return AssertCanOperateSystemUser(operatorID, operatorRole, action, targetID)
}

// IsValidTeamUserRole 旧名兼容：使用新 3 档定义
func IsValidTeamUserRole(role string) bool {
	return IsValidSystemUserRoleCode(role) ||
		role == LegacyTeamUserRoleManager ||
		role == LegacyTeamUserRoleViewer
}

// ============== PermissionService 细粒度权限（从 service/team_user.go 迁移） ==============

// PermissionService 权限服务
//
// 2026-07-22 方向E：所有方法第一参数改为 ctx context.Context。
type PermissionService struct{}

// NewPermissionService 创建权限服务实例
func NewPermissionService() *PermissionService {
	return &PermissionService{}
}

// CheckPermission 检查权限
// 独立部署版本：仅根据 role 检查权限，移除 merchantID 作用域参数。
func (s *PermissionService) CheckPermission(ctx context.Context, roleCode, permission string) bool {
	// 管理员拥有所有权限
	if roleCode == SystemUserRoleAdmin || roleCode == LegacyTeamUserRoleAdmin {
		return true
	}

	// 系统内置的细粒度权限映射
	rolePerms := defaultRolePermissions(roleCode)
	if rolePerms == nil {
		return false
	}
	return matchPermission(rolePerms, permission)
}

// defaultRolePermissions 返回内置角色权限列表（阶段 1 简化）
// 取自原 model.SystemRoles 中 3 个内置角色的 Permissions JSON。
func defaultRolePermissions(roleCode string) []string {
	switch roleCode {
	case SystemUserRoleAdmin:
		return []string{"*"}
	case SystemUserRoleCustomerService, LegacyTeamUserRoleManager:
		// 原 manager 权限（卡片/短链/线索/自动回复）
		return []string{
			"cards.*", "shortlinks.*", "clues.*", "autoreply.*",
		}
	case SystemUserRoleStaff, LegacyTeamUserRoleViewer:
		// 原 viewer 只读权限
		return []string{
			"cards.view", "shortlinks.view", "clues.view",
		}
	default:
		return nil
	}
}

// matchPermission 权限匹配（支持通配符 *.x）
func matchPermission(allowed []string, target string) bool {
	for _, p := range allowed {
		if p == "*" {
			return true
		}
		if p == target {
			return true
		}
		if strings.HasSuffix(p, ".*") {
			prefix := strings.TrimSuffix(p, ".*")
			if strings.HasPrefix(target, prefix+".") {
				return true
			}
		}
	}
	return false
}

// GetUserPermissions 获取用户的所有权限
// 独立部署版本：移除 merchantID 作用域参数。
func (s *PermissionService) GetUserPermissions(ctx context.Context, roleCode string) ([]string, error) {
	if roleCode == SystemUserRoleAdmin {
		return []string{"*"}, nil
	}
	if perms := defaultRolePermissions(roleCode); perms != nil {
		return perms, nil
	}
	return nil, errors.New("角色权限未定义")
}
