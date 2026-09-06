package utils

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"hivemtk-user/internal/cache"
	"hivemtk-user/internal/pkg/utils/logger"
)

const (
	jwtBlacklistTTL = 25 * time.Hour
	jwtBlacklistKey = "jwt:blacklist:"
)

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
	if err := cache.GetGlobalCache().Set(context.Background(), key, "1", jwtBlacklistTTL); err != nil {

		logger.Warnf("BlacklistJWT cache.Set failed (token may be replay-able for %v): %v", jwtBlacklistTTL, err)
	}
}

// IsJWTBlacklisted 检查 JWT 是否在黑名单中
// OPT-ARC-13 + OPT-SEC-01：fail-closed 策略
//
// 旧实现：缓存查询失败时返回 false（fail-open），已被吊销的 token 可能被短暂放行
// 新实现：缓存查询失败时返回 true（fail-closed），宁可误拒也不放过
//
// 理由：登出 token 写入失败 = 系统已不可信，强行放行会扩大攻击面
// 副作用：Redis 不可达时全部 JWT 请求被拒，需要 Redis 高可用保障
func IsJWTBlacklisted(token string) bool {
	if token == "" {
		return false
	}
	key := jwtBlacklistKey + hashJWT(token)
	exists, err := cache.GetGlobalCache().Exists(context.Background(), key)
	if err != nil {
		logger.Errorf("IsJWTBlacklisted cache.Exists failed (fail-closed, rejecting token): %v", err)
		return true
	}
	return exists
}
