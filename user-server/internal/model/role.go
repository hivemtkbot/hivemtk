package model

// SystemRoleCode 系统角色 code（与 model.SystemUser.Role 的合法值保持一致）
//
// 注意：变更时需同步检查 model.IsValidSystemUserRole 与 service/system_user.go 的 oneof 约束。
const (
	SystemRoleCodeAdmin           = "admin"
	SystemRoleCodeCustomerService = "customer_service"
	SystemRoleCodeStaff           = "staff"
)

// SystemRole 系统角色定义（只读展示用）
type SystemRole struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	TagType     string `json:"tag_type"`
	Description string `json:"description"`
	IsSystem    bool   `json:"is_system"`
}

// SystemRoles 系统角色列表（按 v3.1 收口为 3 档）
//
// 顺序即前端卡片展示顺序：
//  1. admin —— 最高权限，唯一性保护（至少保留 1 个）
//  2. customer_service —— 客服，负责客户沟通、订单处理、智能体协同
//  3. staff —— 员工，负责内容编辑、数据分析、运营等日常工作
//
// 注意：本文件使用 SystemRoleList 命名以避免与 model/team_user.go 同包内重复声明。
var SystemRoleList = []SystemRole{
	{
		Code:        SystemRoleCodeAdmin,
		Name:        "超管",
		TagType:     "danger",
		Description: "拥有全部权限，可管理账号/角色/授权，至少保留 1 个启用账号",
		IsSystem:    true,
	},
	{
		Code:        SystemRoleCodeCustomerService,
		Name:        "客服",
		TagType:     "warning",
		Description: "负责客户沟通、订单处理、智能体协同",
		IsSystem:    true,
	},
	{
		Code:        SystemRoleCodeStaff,
		Name:        "员工",
		TagType:     "info",
		Description: "负责内容编辑、数据分析、运营等日常工作",
		IsSystem:    true,
	},
}

// IsValidRole 校验 role code 是否为合法系统角色
//
// 与 model.IsValidSystemUserRole 的差异：
//   - IsValidSystemUserRole 仅校验 "admin" / "user"（用于登录态校验）
//   - IsValidRole 校验完整 3 档（用于业务层校验）
//
// 历史兼容：v1 的 "user" 不在新 3 档内，service 层在创建账号时
// 会将 "user" 归一化为 "staff"。
func IsValidRole(code string) bool {
	for _, r := range SystemRoleList {
		if r.Code == code {
			return true
		}
	}
	return false
}

// GetRoleByCode 按 code 取角色定义；不存在时返回 nil
func GetRoleByCode(code string) *SystemRole {
	for i, r := range SystemRoleList {
		if r.Code == code {
			return &SystemRoleList[i]
		}
	}
	return nil
}

// NormalizeRole 将历史 role 值归一化为 v3.1 三档之一
//
//   - "user" → "staff"（v1 的 user 等价于新 staff）
//   - 其余非法值 → ""（调用方应返回 ErrInvalidInput）
func NormalizeRole(code string) string {
	switch code {
	case SystemRoleCodeAdmin, SystemRoleCodeCustomerService, SystemRoleCodeStaff:
		return code
	case "user":
		return SystemRoleCodeStaff
	default:
		return ""
	}
}
