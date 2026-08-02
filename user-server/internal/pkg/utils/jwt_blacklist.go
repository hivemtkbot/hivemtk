package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"marketing/internal/cache"
)

// jwt_blacklist.go JWT 黑名单工具
//
// F-: RefreshToken / JWT 主动失效支持
//
// 设计：
//   - 后端为 Redis 时跨实例生效；未配置 Redis 时退化为进程内内存（单实例部署）
//   - 仅存储 token 的 sha256 摘要，避免完整 token 落缓存（即使缓存泄露也无法直接复用）
//   - TTL 略大于 JWT 最大过期时间（24h），保证登出令牌在有效期内始终被拦截
//
// 五层架构归属：L3 工具层（无业务语义，纯函数式工具）
// 调用方：middleware.JWTAuthMiddleware（校验）、service.AuthService.Logout/RefreshToken（写入）

const (
	jwtBlacklistTTL = 25 * time.Hour
	jwtBlacklistKey = "jwt:blacklist:"
)

// hashJWT 对 JWT 做哈希后存储，避免完整 token 落 Redis
func hashJWT(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// BlacklistJWT 将指定 JWT 加入黑名单（登出 / 刷新时调用）
func BlacklistJWT(token string) {
	if token == "" {
		return
	}
	key := jwtBlacklistKey + hashJWT(token)
	_ = cache.GetGlobalCache().Set(context.Background(), key, "1", jwtBlacklistTTL)
}

// IsJWTBlacklisted 检查 JWT 是否在黑名单中
// 缓存查询失败时返回 false（fail-open，避免缓存故障导致全部请求被拒）
func IsJWTBlacklisted(token string) bool {
	if token == "" {
		return false
	}
	key := jwtBlacklistKey + hashJWT(token)
	exists, err := cache.GetGlobalCache().Exists(context.Background(), key)
	if err != nil {
		return false
	}
	return exists
}
