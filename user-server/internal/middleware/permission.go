package middleware

import (
	"errors"
	"marketing/internal/pkg/utils"
	"marketing/internal/pkg/utils/response"
	"marketing/internal/service"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// PermissionMiddleware 权限中间件
func PermissionMiddleware(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 获取用户角色
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, utils.ErrorCodeForbidden, "未找到角色信息")
			c.Abort()
			return
		}

		// 创建权限服务
		permissionService := service.NewPermissionService()

		// 检查权限（独立部署：role 直接用于权限判断）
		hasPermission := permissionService.CheckPermission(role.(string), permission)
		if !hasPermission {
			response.Error(c, utils.ErrorCodeForbidden, "无权限执行此操作")
			c.Abort()
			return
		}

		c.Next()
	}
}

// AdminOnlyMiddleware 仅管理员访问中间件
func AdminOnlyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, utils.ErrorCodeForbidden, "未找到角色信息")
			c.Abort()
			return
		}

		if role.(string) != "admin" {
			response.Error(c, utils.ErrorCodeForbidden, "仅管理员可访问")
			c.Abort()
			return
		}

		c.Next()
	}
}

// ManagerOrAdminMiddleware 经理或管理员访问中间件
func ManagerOrAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, utils.ErrorCodeForbidden, "未找到角色信息")
			c.Abort()
			return
		}

		roleStr := role.(string)
		if roleStr != "admin" && roleStr != "manager" {
			response.Error(c, utils.ErrorCodeForbidden, "仅经理或管理员可访问")
			c.Abort()
			return
		}

		c.Next()
	}
}

// TeamJWTAuthMiddleware JWT 认证中间件 - 支持团队用户
func TeamJWTAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if IsTestMode && testing.Testing() {
			c.Set("user_id", uint(1))
			c.Set("license_id", "system_admin")
			c.Next()
			return
		}
		// 从请求头获取 Token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Error(c, utils.ErrorCodeUnauthorized, "未提供认证令牌")
			c.Abort()
			return
		}

		// 提取 Bearer 令牌
		parts := strings.SplitN(authHeader, " ", 2)
		if !(len(parts) == 2 && parts[0] == "Bearer") {
			response.Error(c, utils.ErrorCodeTokenInvalid, "认证令牌格式错误")
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析 Token
		claims, err := ParseJWTToken(tokenString)
		if err != nil {
			response.Error(c, utils.ErrorCodeUnauthorized, "无效的认证令牌")
			c.Abort()
			return
		}

		// 设置用户信息到上下文
		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])

		c.Next()
	}
}

// ParseJWTToken 解析 JWT Token
func ParseJWTToken(tokenString string) (map[string]any, error) {
	// 获取 JWT 密钥
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		return nil, errors.New("JWT_SECRET 未配置")
	}

	// 解析 Token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		// 验证签名算法
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil {
		return nil, err
	}

	// 验证 Token 是否有效
	if !token.Valid {
		return nil, jwt.ErrSignatureInvalid
	}

	// 提取 Claims
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("无效的 Token Claims")
	}

	// 构建返回结果
	result := make(map[string]any)
	if userID, ok := claims["user_id"]; ok {
		result["user_id"] = userID
	}
	if username, ok := claims["username"]; ok {
		result["username"] = username
	}
	if role, ok := claims["role"]; ok {
		result["role"] = role
	}

	return result, nil
}

// RequirePermission 快捷方法 - 检查特定权限
func RequirePermission(permission string) gin.HandlerFunc {
	return PermissionMiddleware(permission)
}

// RequireAnyPermission 快捷方法 - 检查是否有任一权限
func RequireAnyPermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, utils.ErrorCodeForbidden, "未找到角色信息")
			c.Abort()
			return
		}

		permissionService := service.NewPermissionService()

		for _, p := range permissions {
			if permissionService.CheckPermission(role.(string), p) {
				c.Next()
				return
			}
		}

		response.Error(c, utils.ErrorCodeForbidden, "无权限执行此操作")
		c.Abort()
	}
}

// RequireAllPermissions 快捷方法 - 检查是否有所有权限
func RequireAllPermissions(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			response.Error(c, utils.ErrorCodeForbidden, "未找到角色信息")
			c.Abort()
			return
		}

		permissionService := service.NewPermissionService()

		for _, p := range permissions {
			if !permissionService.CheckPermission(role.(string), p) {
				response.Error(c, utils.ErrorCodeForbidden, "无权限执行此操作")
				c.Abort()
				return
			}
		}

		c.Next()
	}
}
