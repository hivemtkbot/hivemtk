package middleware

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
	"hivemtk-user/internal/pkg/utils/response"

	"github.com/gin-gonic/gin"
)

// MFAVerifyContextKey 上下文键：表示请求已通过 MFA 验证
const MFAVerifyContextKey = "mfa_verified"

const (
	mfaRecentVerifyTTL    = 5 * time.Minute
	mfaRecentVerifyKeyFmt = "mfa:verified:%d" // key: mfa:verified:<user_id>
)

// mfaRecentVerify 内存缓存（OPT-ARC-12 二期：与 Redis 双写）
// 多副本部署时，优先读 Redis，Redis 不可达时回退内存
var (
	mfaRecentVerify      = make(map[uint]time.Time)
	mfaRecentVerifyMutex sync.RWMutex
)

// MarkMFAVerified 标记用户在当前时刻通过 MFA 验证
// OPT-ARC-12 + OPT-SEC-02：多副本兼容
// 1) 写 Redis（多实例共享，TTL 5min）
// 2) 写内存（Redis 不可达时回退）
// 3) 异步清理过期内存项
func MarkMFAVerified(userID uint) {
	now := time.Now()

	// 1) Redis 主写
	key := fmt.Sprintf(mfaRecentVerifyKeyFmt, userID)
	if err := cache.GetGlobalCache().Set(context.Background(), key, now.Unix(), mfaRecentVerifyTTL); err != nil {
		logger.Warnf("MarkMFAVerified Redis Set failed, fallback to memory: %v", err)
		// 2) Redis 失败时内存回退
		mfaRecentVerifyMutex.Lock()
		mfaRecentVerify[userID] = now
		mfaRecentVerifyMutex.Unlock()
	}

	// v3 审计 P1-23 修复：cleanup 改为启动期单次 + 周期触发
	// 原：每次 MarkMFAVerified 都 go func()，高频下产生大量 goroutine 阻塞持锁清理
	// 新：sync.Once + 周期 ticker，所有实例共享同一个 cleanup goroutine
	mfaCleanupTrigger.Do(func() {
		go mfaCleanupLoop()
	})
}

// mfaCleanupTrigger 防止重复启动 cleanup goroutine
var mfaCleanupTrigger sync.Once

// mfaCleanupLoop 周期清理过期 MFA 记录（启动期一次性启动）
func mfaCleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cleanupExpiredMFAVerified()
	}
}

// IsMFAVerifiedRecently 检查用户最近 5 分钟内是否通过 MFA 验证
// OPT-ARC-12：优先读 Redis（多副本共享），失败回退内存
func IsMFAVerifiedRecently(userID uint) bool {
	// 1) Redis 优先
	key := fmt.Sprintf(mfaRecentVerifyKeyFmt, userID)
	val, err := cache.GetGlobalCache().Get(context.Background(), key)
	if err == nil && val != "" {
		// 解析时间戳
		var ts int64
		if _, scanErr := fmt.Sscanf(val, "%d", &ts); scanErr == nil {
			verifiedAt := time.Unix(ts, 0)
			if time.Since(verifiedAt) < mfaRecentVerifyTTL {
				return true
			}
		}
	}

	// 2) Redis 失败/未命中 → 内存回退
	mfaRecentVerifyMutex.RLock()
	t, ok := mfaRecentVerify[userID]
	mfaRecentVerifyMutex.RUnlock()
	if !ok {
		return false
	}
	return time.Since(t) < mfaRecentVerifyTTL
}

// cleanupExpiredMFAVerified 清理过期内存记录（仅在 Redis 失败的实例上累积）
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
			response.Error(c, http.StatusInternalServerError, "查询 MFA 状态失败")
			c.Abort()
			return
		}
		if !enabled {
			response.Error(c, http.StatusForbidden, "此操作要求启用 MFA")
			c.Abort()
			return
		}

		c.Next()
	}
}
