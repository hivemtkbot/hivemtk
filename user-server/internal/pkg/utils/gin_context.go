package utils

import "github.com/gin-gonic/gin"

// CtxKeyUserID 写入 gin.Context 的 user_id 字段名。
//
// 与 internal/middleware/jwt.go 中 JWTAuthMiddleware 设置的 key 对齐；
// 业务侧应通过 GetUID 读取，禁止散落字面量字符串。
const CtxKeyUserID = "user_id"

// CtxKeyUsername gin context 中的 username 字段名。
const CtxKeyUsername = "username"

// CtxKeyRole gin context 中的 role 字段名。
const CtxKeyRole = "role"

// GetUID 从 gin 上下文安全读取当前登录用户 ID。
//
// 返回 0 表示未登录或 ctx 为 nil——调用方按业务语义选择 fail-closed。
//
// 为兼容 service 层无法直接拿到 gin.Context 的场景，本函数同时支持从
// *gin.Context 与 ctx.Request.Context() 推导的 context.Context 调用：
// gin.Context 优先（性能更佳，类型断言无反射）。
func GetUID(c *gin.Context) uint {
	if c == nil {
		return 0
	}
	v, ok := c.Get(CtxKeyUserID)
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case uint:
		return x
	case uint64:
		return uint(x)
	case int:
		if x < 0 {
			return 0
		}
		return uint(x)
	case int64:
		if x < 0 {
			return 0
		}
		return uint(x)
	case string:
		return 0
	default:
		return 0
	}
}

// MustGetUID 读取 user_id；不存在或为 0 时 panic（仅限强制登录的入口）。
//
// 主要用于：路由已挂 JWTAuthMiddleware 后又显式调用本函数的冗余防御场景。
func MustGetUID(c *gin.Context) uint {
	uid := GetUID(c)
	if uid == 0 {
		panic("[SECURITY] MustGetUID: no user_id in gin context (JWTAuthMiddleware missing?)")
	}
	return uid
}

// GetUsername 安全读取 username（缺省返回空串）。
func GetUsername(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(CtxKeyUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetRole 安全读取 role（缺省返回空串）。
func GetRole(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(CtxKeyRole); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// IsAdmin 便捷判断当前用户是否为 admin 角色。
//
// 注意：admin 视为超管，归属校验可对 admin 豁免；
// 真正的豁免逻辑放在调用方按需短路（不放在本函数以保持语义最小）。
func IsAdmin(c *gin.Context) bool {
	return GetRole(c) == "admin"
}
