package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"hivemtk-user/internal/cache"
)


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

