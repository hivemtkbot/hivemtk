package middleware

import (
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// MFAVerifyContextKey 上下文键：表示请求已通过 MFA 验证
const MFAVerifyContextKey = "mfa_verified"

// mfaRecentVerify 记录某用户最近一次 MFA 验证时间
// 用于敏感操作（如修改密码、修改邮箱）要求近期 MFA 验证
// 单进程内存，5 分钟有效期；多实例需迁移 Redis
var (
	mfaRecentVerify      = make(map[uint]time.Time)
	mfaRecentVerifyMutex sync.RWMutex
)

const mfaRecentVerifyTTL = 5 * time.Minute

// MarkMFAVerified 标记用户在当前时刻通过 MFA 验证
// 在 MFA 验证成功、敏感操作前置验证成功时调用
func MarkMFAVerified(userID uint) {
	mfaRecentVerifyMutex.Lock()
	defer mfaRecentVerifyMutex.Unlock()
	mfaRecentVerify[userID] = time.Now()

	// 异步清理过期项
	go cleanupExpiredMFAVerified()
}

// IsMFAVerifiedRecently 检查用户最近 5 分钟内是否通过 MFA 验证
func IsMFAVerifiedRecently(userID uint) bool {
	mfaRecentVerifyMutex.RLock()
	t, ok := mfaRecentVerify[userID]
	mfaRecentVerifyMutex.RUnlock()
	if !ok {
		return false
	}
	return time.Since(t) < mfaRecentVerifyTTL
}

// cleanupExpiredMFAVerified 清理过期记录
func cleanupExpiredMFAVerified() {
	mfaRecentVerifyMutex.Lock()
	defer mfaRecentVerifyMutex.Unlock()
	now := time.Now()
	for k, v := range mfaRecentVerify {
		if now.Sub(v) >= mfaRecentVerifyTTL {
			delete(mfaRecentVerify, k)
		}
	}
}

// RequireMFARecent 中间件：要求用户在最近 5 分钟内通过 MFA 验证
// 用于敏感操作（修改密码、修改邮箱、修改权限等）
//
// 用法：
//
//	r.PUT("/api/users/:id/password",
//	    middleware.JWTAuthMiddleware(),
//	    middleware.RequireMFARecent(),
//	    controller.ResetPassword)
func RequireMFARecent() gin.HandlerFunc {
	return func(c *gin.Context) {
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

		if !IsMFAVerifiedRecently(userID) {
			response.Error(c, http.StatusForbidden, "需要二次验证（MFA）才能执行此操作")
			c.Abort()
			return
		}

		c.Set(MFAVerifyContextKey, true)
		c.Next()
	}
}

// RequireMFAEnabled 中间件：要求用户已启用 MFA 才能访问
// 用于：高敏感接口（如导出全部客户数据、批量删除）
func RequireMFAEnabled(mfaEnabledFunc func(userID uint) (bool, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		enabled, err := mfaEnabledFunc(userID)
		if err != nil {
			response.Error(c, http.StatusInternalServerError, "MFA 状态查询失败")
			c.Abort()
			return
		}
		if !enabled {
			response.Error(c, http.StatusForbidden, "此操作要求启用 MFA（多因素认证）")
			c.Abort()
			return
		}

		c.Next()
	}
}
