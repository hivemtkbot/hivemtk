package middleware

import (
	"log"
	"net/http"
	"strings"
	"testing"

	"marketing/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// IsTestMode 测试模式标志，仅用于测试时绕过认证
var IsTestMode bool

// JWTAuthMiddleware JWT认证中间件
func JWTAuthMiddleware() gin.HandlerFunc {
	// 创建JWT工具实例
	jwtUtils := utils.NewJWTUtils(utils.DefaultJWTConfig)

	return func(c *gin.Context) {
		if IsTestMode && testing.Testing() {
			c.Set("user_id", uint(1))
			c.Set("license_id", "system_admin")
			c.Set("role", "admin")
			c.Set("data_scope", "all")
			c.Next()
			return
		}
		// 从请求头获取Authorization；WebSocket 无法设置自定义 header，
		// 故对 Upgrade=websocket 的连接额外兼容 ?token= 查询参数（不扩大 REST 鉴权面）
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" && c.GetHeader("Upgrade") == "websocket" {
			if q := c.Query("token"); q != "" {
				authHeader = "Bearer " + q
			}
		}
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "未提供认证令牌",
			})
			c.Abort()
			return
		}

		// 检查Bearer前缀
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		// 解析JWT令牌
		claims, err := jwtUtils.ParseToken(parts[1])
		if err != nil {
			// 不把 JWT 库内部错误原文返回给客户端，避免泄露算法/校验细节辅助绕过
			log.Printf("[WARN] JWT 校验失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证令牌",
			})
			c.Abort()
			return
		}

		// 修复：令牌若已被拉黑（登出/改密后），直接拒绝，使失效立即生效。
		// fail-open：缓存查询失败时 IsJWTBlacklisted 返回 false，不阻断正常请求。
		if utils.IsJWTBlacklisted(parts[1]) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌已失效，请重新登录",
			})
			c.Abort()
			return
		}

		// 将用户信息存储到上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		// 行级权限：解析 data_scope（若 JWT 中携带）
		// admin 角色 → 强制 all
		if claims.Role == "admin" {
			c.Set("license_id", "system_admin")
			c.Set("data_scope", "all")
		} else {
			if claims.DataScope != "" {
				c.Set("data_scope", claims.DataScope)
			} else {
				c.Set("data_scope", "self") // 默认仅自己
			}
		}
		c.Set("department_id", claims.DepartmentID)
		c.Set("team_id", claims.TeamID)

		c.Next()
	}
}

// AdminAuthMiddleware 管理员权限中间件
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsTestMode && testing.Testing() {
			c.Set("role", "admin")
			c.Next()
			return
		}
		// 从上下文获取用户角色
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "未找到用户角色信息",
			})
			c.Abort()
			return
		}

		// 检查是否为管理员
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"code": 403,
				"msg":  "权限不足，需要管理员权限",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// OptionalAuthMiddleware 可选认证中间件（不强制要求登录）
func OptionalAuthMiddleware() gin.HandlerFunc {
	// 创建JWT工具实例
	jwtUtils := utils.NewJWTUtils(utils.DefaultJWTConfig)

	return func(c *gin.Context) {
		if IsTestMode && testing.Testing() {
			c.Set("user_id", uint(1))
			c.Set("license_id", "system_admin")
			c.Next()
			return
		}
		// 从请求头获取Authorization
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// 没有提供令牌，继续处理请求
			c.Next()
			return
		}

		// 检查Bearer前缀
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			// 令牌格式错误，继续处理请求
			c.Next()
			return
		}

		// 解析JWT令牌
		claims, err := jwtUtils.ParseToken(parts[1])
		if err != nil {
			// 令牌无效，继续处理请求
			c.Next()
			return
		}

		// 将用户信息存储到上下文
		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}
