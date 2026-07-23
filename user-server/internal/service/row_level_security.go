package service

// row_level_security.go A 域 P1-4 数据行级权限服务
//
// 五层架构归属: L3 业务服务层
// 设计依据: docs/standards/MASTER_RULES.md「私域独立部署，无 merchant_id 字段」
//          A 域 P1 缺口修复 (2026-07-21)
//
// 数据范围说明（team_user.data_scope）：
//   1 = DataScopeAll        全部数据（仅 admin/超管）
//   2 = DataScopeDepartment 本部门数据
//   3 = DataScopeSelf       本人数据
//   4 = DataScopeCustom     自定义（按 custom_dept_ids 白名单）
//
// 本服务职责：
//  1. 提供 DataScope 枚举与默认值
//  2. 提供 ApplyDataScopeForTeam：基于 gin.Context + data_scope 在 GORM 查询上注入 WHERE
//  3. 提供 GetDataScope / GetUserID 等 ctx 读取助手（与 middleware/data_scope.go 互补，
//     service 层无需引入 middleware 依赖）
//  4. 提供 BuildScopeDescription：将 data_scope 转中文描述
//
// 与 middleware/data_scope.go 的关系：
//   - middleware 负责从 JWT/DB 解析 data_scope 注入 ctx
//   - 本 service 负责将 ctx 中的 data_scope 翻译成 WHERE 条件
//   - service 层调用 ApplyDataScopeForTeam 即可获得带过滤的 *gorm.DB

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"context"
)

// TeamDataScope 团队用户数据范围（与 team_user.data_scope 列对齐）
//
// 设计依据：A 域 P1-4 需求文档「team_user.data_scope (1=全部 2=本部门 3=本人 4=自定义)」
type TeamDataScope int

const (
	TeamDataScopeAll	TeamDataScope	= 1	// 全部
	TeamDataScopeDepartment	TeamDataScope	= 2	// 本部门
	TeamDataScopeSelf	TeamDataScope	= 3	// 本人
	TeamDataScopeCustom	TeamDataScope	= 4	// 自定义（custom_dept_ids 白名单）
)

// IsValidTeamDataScope 校验 team 数据范围
func IsValidTeamDataScope(scope int) bool {
	return scope == int(TeamDataScopeAll) ||
		scope == int(TeamDataScopeDepartment) ||
		scope == int(TeamDataScopeSelf) ||
		scope == int(TeamDataScopeCustom)
}

// DefaultTeamDataScope 默认数据范围（新建用户未指定时）
const DefaultTeamDataScope = int(TeamDataScopeSelf)

// TeamDataScopeName 范围名（中文）
func TeamDataScopeName(scope int) string {
	switch scope {
	case int(TeamDataScopeAll):
		return "全部"
	case int(TeamDataScopeDepartment):
		return "本部门"
	case int(TeamDataScopeSelf):
		return "本人"
	case int(TeamDataScopeCustom):
		return "自定义"
	default:
		return "未知"
	}
}

// TeamDataScopeContext 团队数据范围上下文（从 gin.Context 解析后的强类型结构）
type TeamDataScopeContext struct {
	UserID		uint
	Role		string
	DepartmentID	uint
	TeamID		uint
	DataScope	int
	CustomDeptIDs	[]uint	// data_scope=4 时的部门白名单
	IsAdmin		bool
}

// ReadTeamDataScopeContext 从 gin.Context 解析 TeamDataScopeContext
//
// 优先使用 ctx 中已注入的强类型值，回退到 data_scope / department_id / team_id 字符串/数字
func ReadTeamDataScopeContext(c *gin.Context) (*TeamDataScopeContext, error) {
	if c == nil {
		return nil, errors.New("gin context is nil")
	}

	out := &TeamDataScopeContext{}

	// user_id
	if v, ok := c.Get("user_id"); ok {
		switch u := v.(type) {
		case uint:
			out.UserID = u
		case int:
			out.UserID = uint(u)
		case int64:
			out.UserID = uint(u)
		case float64:
			out.UserID = uint(u)
		}
	}

	// role
	if v, ok := c.Get("role"); ok {
		if rs, ok := v.(string); ok {
			out.Role = rs
		}
	}
	out.IsAdmin = out.Role == "admin"

	// data_scope：service 层调用方可能注入 int，也可能注入 string
	if v, ok := c.Get("data_scope"); ok {
		switch s := v.(type) {
		case int:
			out.DataScope = s
		case int64:
			out.DataScope = int(s)
		case float64:
			out.DataScope = int(s)
		case string:
			out.DataScope = mapStringDataScopeToInt(s)
		}
	}

	// department_id
	if v, ok := c.Get("department_id"); ok {
		switch u := v.(type) {
		case uint:
			out.DepartmentID = u
		case int:
			out.DepartmentID = uint(u)
		case int64:
			out.DepartmentID = uint(u)
		case float64:
			out.DepartmentID = uint(u)
		}
	}

	// team_id
	if v, ok := c.Get("team_id"); ok {
		switch u := v.(type) {
		case uint:
			out.TeamID = u
		case int:
			out.TeamID = uint(u)
		case int64:
			out.TeamID = uint(u)
		case float64:
			out.TeamID = uint(u)
		}
	}

	return out, nil
}

// mapStringDataScopeToInt 兼容 SystemUser.data_scope 字符串值，转 TeamDataScope
func mapStringDataScopeToInt(s string) int {
	switch s {
	case "all", "All", "ALL":
		return int(TeamDataScopeAll)
	case "department", "Department", "DEPARTMENT":
		return int(TeamDataScopeDepartment)
	case "team", "Team", "TEAM":
		return int(TeamDataScopeSelf)	// team 在 team_user 中映射到 self（与 system_user 区分）
	case "self", "Self", "SELF":
		return int(TeamDataScopeSelf)
	case "custom", "Custom", "CUSTOM":
		return int(TeamDataScopeCustom)
	default:
		return DefaultTeamDataScope
	}
}

// RowLevelSecurityService 行级权限服务
type RowLevelSecurityService struct{}

// NewRowLevelSecurityService 创建行级权限服务
func NewRowLevelSecurityService() *RowLevelSecurityService {
	return &RowLevelSecurityService{}
}

// ApplyDataScopeForTeam 基于 TeamDataScopeContext 在 GORM 查询上追加 WHERE 条件
//
// 参数：
//   - database: GORM DB 实例
//   - ctx: gin 上下文（service 层只能依赖 ctx 注入的元数据，不直接查 DB）
//   - ownerField: 资源 owner 字段名（如 "owner_id" / "user_id" / "created_by"）
//     空字符串默认 "user_id"
//   - departmentField: 资源部门字段名（空表示本资源不支持部门维度过滤）
//   - teamField: 资源团队字段名（空表示本资源不支持团队维度过滤）
//
// 返回：附加 WHERE 后的 *gorm.DB
//
// 行为：
//   - admin 角色 / data_scope=1：不过滤
//   - data_scope=3：owner_field = user_id
//   - data_scope=2：department_field = department_id
//   - data_scope=4：department_field IN custom_dept_ids
//   - department_field 为空时降级为 self
func (s *RowLevelSecurityService) ApplyDataScopeForTeam(ctx context.Context,
	database *gorm.DB,
	c *gin.Context,
	ownerField string,
	departmentField string,
	teamField string,
) *gorm.DB {
	if database == nil {
		// 五层架构合规：service 层不允许直接调全局 DB 入口回退
		// 由调用方显式注入 *gorm.DB；nil 直接返回，由调用方决定降级策略
		return nil
	}
	if ownerField == "" {
		ownerField = "user_id"
	}
	if c == nil {
		return database
	}

	scope, err := ReadTeamDataScopeContext(c)
	if err != nil {
		return database
	}

	// admin → 不过滤
	if scope.IsAdmin {
		return database
	}

	switch TeamDataScope(scope.DataScope) {
	case TeamDataScopeAll:
		return database

	case TeamDataScopeSelf:
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database

	case TeamDataScopeDepartment:
		if scope.DepartmentID > 0 && departmentField != "" {
			return database.Where(departmentField+" = ?", scope.DepartmentID)
		}
		// 降级为 self
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database

	case TeamDataScopeCustom:
		if len(scope.CustomDeptIDs) > 0 && departmentField != "" {
			return database.Where(departmentField+" IN ?", scope.CustomDeptIDs)
		}
		// custom 但无白名单 → 降级为 self
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database

	default:
		// 未知 scope → 保守 self
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database
	}
}

// ApplyDataScopeForTeamByScope 显式传入 scope（无需 gin.Context，service 内单测/批处理场景）
func (s *RowLevelSecurityService) ApplyDataScopeForTeamByScope(ctx context.Context,
	database *gorm.DB,
	scope *TeamDataScopeContext,
	ownerField string,
	departmentField string,
) *gorm.DB {
	if database == nil || scope == nil {
		return database
	}
	if ownerField == "" {
		ownerField = "user_id"
	}
	if scope.IsAdmin {
		return database
	}

	switch TeamDataScope(scope.DataScope) {
	case TeamDataScopeAll:
		return database
	case TeamDataScopeSelf:
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database
	case TeamDataScopeDepartment:
		if scope.DepartmentID > 0 && departmentField != "" {
			return database.Where(departmentField+" = ?", scope.DepartmentID)
		}
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database
	case TeamDataScopeCustom:
		if len(scope.CustomDeptIDs) > 0 && departmentField != "" {
			return database.Where(departmentField+" IN ?", scope.CustomDeptIDs)
		}
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database
	default:
		if scope.UserID > 0 {
			return database.Where(ownerField+" = ?", scope.UserID)
		}
		return database
	}
}

// BuildScopeDescription 将 data_scope 整型值转中文描述
func (s *RowLevelSecurityService) BuildScopeDescription(ctx context.Context, scope int) string {
	return TeamDataScopeName(scope)
}
