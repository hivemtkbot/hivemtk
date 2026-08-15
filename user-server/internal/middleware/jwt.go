package middleware

import (
	"log"
	"net/http"
	"strings"
	"testing"

	"hivemtk-user/internal/pkg/utils"

	"github.com/gin-gonic/gin"
)

// IsTestMode 测试模式标志，仅用于测试时绕过认证
var IsTestMode bool

// JWTAuthMiddleware JWT认证中间件
func JWTAuthMiddleware() gin.HandlerFunc {
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

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌格式错误",
			})
			c.Abort()
			return
		}

		claims, err := jwtUtils.ParseToken(parts[1])
		if err != nil {
			log.Printf("[WARN] JWT 校验失败: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "无效的认证令牌",
			})
			c.Abort()
			return
		}

		if utils.IsJWTBlacklisted(parts[1]) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "认证令牌已失效，请重新登录",
			})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		if claims.Role == "admin" {
			c.Set("license_id", "system_admin")
			c.Set("data_scope", "all")
		} else {
			if claims.DataScope != "" {
				c.Set("data_scope", claims.DataScope)
			} else {
				c.Set("data_scope", "self") 
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
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code": 401,
				"msg":  "未找到用户角色信息",
			})
			c.Abort()
			return
		}

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
	jwtUtils := utils.NewJWTUtils(utils.DefaultJWTConfig)

	return func(c *gin.Context) {
		if IsTestMode && testing.Testing() {
			c.Set("user_id", uint(1))
			c.Set("license_id", "system_admin")
			c.Next()
			return
		}
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			c.Next()
			return
		}

		claims, err := jwtUtils.ParseToken(parts[1])
		if err != nil {
			c.Next()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)

		c.Next()
	}
}

