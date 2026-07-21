package middleware

import (
	"errors"
	"net/http"

	"marketing/internal/model"
	"marketing/internal/pkg/utils/db"
	"marketing/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DataScopeMiddleware 数据范围中间件
//
// 设计要点：
//   - 在 JWTAuthMiddleware 之后执行
//   - 从 gin.Context 读取 user_id + role + data_scope
//   - 若 data_scope 缺失，从数据库查询 user.data_scope（兼容旧 JWT）
//   - admin 角色强制 data_scope = all
//   - 将最终 data_scope 写回 gin.Context，供 controller / service 使用
func DataScopeMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)

		// admin → all
		if roleStr == model.SystemUserRoleAdmin {
			c.Set("data_scope", model.DataScopeAll)
			c.Next()
			return
		}

		// 已有 data_scope（JWT 解析注入）
		if ds, exists := c.Get("data_scope"); exists {
			if dsStr, ok := ds.(string); ok && model.IsValidDataScope(dsStr) {
				c.Next()
				return
			}
		}

		// 从 DB 查询补充
		userIDRaw, exists := c.Get("user_id")
		if !exists {
			response.Error(c, http.StatusUnauthorized, "未找到用户信息")
			c.Abort()
			return
		}
		userID, ok := userIDRaw.(uint)
		if !ok {
			response.Error(c, http.StatusInternalServerError, "用户 ID 类型错误")
			c.Abort()
			return
		}

		database := db.GetDB()
		if database == nil {
			// db 未初始化（测试场景），默认 self
			c.Set("data_scope", model.DataScopeSelf)
			c.Next()
			return
		}

		var user model.SystemUser
		if err := database.Select("data_scope, department_id, team_id").
			First(&user, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				response.Error(c, http.StatusUnauthorized, "用户不存在")
				c.Abort()
				return
			}
			// 查询失败降级为 self（保守策略）
			c.Set("data_scope", model.DataScopeSelf)
			c.Next()
			return
		}

		if user.DataScope == "" {
			user.DataScope = model.DefaultDataScopeForRole(roleStr)
		}
		c.Set("data_scope", user.DataScope)
		if user.DepartmentID > 0 {
			c.Set("department_id", user.DepartmentID)
		}
		if user.TeamID > 0 {
			c.Set("team_id", user.TeamID)
		}

		c.Next()
	}
}

// ApplyDataScope 在 GORM 查询上追加数据范围过滤
//
// 参数：
//   - database: GORM DB 实例
//   - ctx: gin 上下文（需先经过 JWTAuthMiddleware + DataScopeMiddleware）
//   - ownerField: 数据所有者字段名（如 "owner_id" / "user_id" / "creator_id"）
//     空字符串默认 "user_id"
//   - departmentField: 部门字段名（空表示不支持部门维度过滤）
//   - teamField: 团队字段名（空表示不支持团队维度过滤）
//
// 返回：附加了 WHERE 条件的 *gorm.DB（链式调用安全）
//
// 用法：
//
//	database := db.GetDB()
//	database = middleware.ApplyDataScope(database, ctx, "owner_id", "dept_id", "team_id")
//	database.Find(&customers)
func ApplyDataScope(database *gorm.DB, ctx *gin.Context, ownerField string, departmentField string, teamField string) *gorm.DB {
	if database == nil {
		return database
	}
	if ownerField == "" {
		ownerField = "user_id"
	}

	// admin / data_scope=all → 不过滤
	role, _ := ctx.Get("role")
	roleStr, _ := role.(string)
	if roleStr == model.SystemUserRoleAdmin {
		return database
	}

	ds, _ := ctx.Get("data_scope")
	dsStr, _ := ds.(string)
	if dsStr == model.DataScopeAll {
		return database
	}

	userIDRaw, _ := ctx.Get("user_id")
	userID, _ := userIDRaw.(uint)

	switch dsStr {
	case model.DataScopeSelf:
		// 仅自己创建的
		if userID > 0 {
			database = database.Where(ownerField+" = ?", userID)
		}
	case model.DataScopeDepartment:
		// 本部门：先按 owner 限制为本部门所有成员
		deptIDRaw, _ := ctx.Get("department_id")
		deptID, _ := deptIDRaw.(uint)
		if deptID > 0 && departmentField != "" {
			database = database.Where(departmentField+" = ?", deptID)
		} else if userID > 0 {
			// 没有部门字段或部门 ID → 降级为 self
			database = database.Where(ownerField+" = ?", userID)
		}
	case model.DataScopeTeam:
		teamIDRaw, _ := ctx.Get("team_id")
		teamID, _ := teamIDRaw.(uint)
		if teamID > 0 && teamField != "" {
			database = database.Where(teamField+" = ?", teamID)
		} else if userID > 0 {
			// 没有团队字段或团队 ID → 降级为 self
			database = database.Where(ownerField+" = ?", userID)
		}
	default:
		// 默认 self
		if userID > 0 {
			database = database.Where(ownerField+" = ?", userID)
		}
	}

	return database
}

// GetDataScope 从 gin.Context 读取当前数据范围
// 若未设置，返回 self
func GetDataScope(ctx *gin.Context) string {
	if ds, exists := ctx.Get("data_scope"); exists {
		if dsStr, ok := ds.(string); ok && model.IsValidDataScope(dsStr) {
			return dsStr
		}
	}
	return model.DataScopeSelf
}

// GetUserID 从 gin.Context 读取当前用户 ID
func GetUserID(ctx *gin.Context) uint {
	if v, exists := ctx.Get("user_id"); exists {
		switch u := v.(type) {
		case uint:
			return u
		case int:
			return uint(u)
		case int64:
			return uint(u)
		case float64:
			return uint(u)
		}
	}
	return 0
}

// IsAdmin 检查当前用户是否为 admin
func IsAdmin(ctx *gin.Context) bool {
	role, _ := ctx.Get("role")
	roleStr, _ := role.(string)
	return roleStr == model.SystemUserRoleAdmin
}
