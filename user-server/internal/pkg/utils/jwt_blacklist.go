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
