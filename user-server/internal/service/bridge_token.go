package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"os"

	"hivemtk-user/internal/repository"
)

// bridgeTokenKVKey / bridgeTokenPrevKVKey 桥接凭证 KV 键（controller 与 guard 中间件共用语义）
const (
	bridgeTokenKVKey     = "bridge_ingest_token"
	bridgeTokenPrevKVKey = "bridge_ingest_token_prev"
)

// BridgeTokenService 桥接通道凭证服务
//
// GET  /api/bridge/token/status  —— 查询配置状态（不回显明文）
// POST /api/bridge/token/reset   —— 轮换：旧值转 PREV，生成新值返回（仅此一次明文）
//
// 存储优先级：DB system_config_kv（运行时可变）> 环境变量 BRIDGE_INGEST_TOKEN。
type BridgeTokenService struct {
	kv repository.SystemConfigKVRepository
}

// NewBridgeTokenService 创建服务（全局 DB 装配）
func NewBridgeTokenService() *BridgeTokenService {
	return &BridgeTokenService{kv: repository.NewSystemConfigKVRepository()}
}

// generateBridgeTokenValue 32 字节随机 → base64url（43 字符，无填充）
func generateBridgeTokenValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Status 返回主/灰度 token 是否已配置（不回显明文）
func (s *BridgeTokenService) Status(ctx context.Context) map[string]any {
	status := map[string]any{
		"main_configured": false,
		"prev_configured": false,
		"source":          "unset",
	}
	if v, err := s.kv.Get(ctx, bridgeTokenKVKey); err == nil && v != "" {
		status["main_configured"] = true
		status["source"] = "db"
	} else if os.Getenv("BRIDGE_INGEST_TOKEN") != "" {
		status["main_configured"] = true
		status["source"] = "env"
	}
	if v, err := s.kv.Get(ctx, bridgeTokenPrevKVKey); err == nil && v != "" {
		status["prev_configured"] = true
	}
	return status
}

// Reset 轮换凭证：旧 token 转入灰度位（PREV），生成并返回新 token 明文（仅此一次）
func (s *BridgeTokenService) Reset(ctx context.Context) (string, error) {
	newTok, err := generateBridgeTokenValue()
	if err != nil {
		return "", err
	}
	// 当前值 → PREV（双值灰度窗口）
	if cur, gerr := s.kv.Get(ctx, bridgeTokenKVKey); gerr == nil && cur != "" {
		_, _ = s.kv.Upsert(ctx, bridgeTokenPrevKVKey, cur)
	}
	if _, uerr := s.kv.Upsert(ctx, bridgeTokenKVKey, newTok); uerr != nil {
		return "", uerr
	}
	return newTok, nil
}
