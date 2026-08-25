package controller

import (
	"os"
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"hivemtk-user/internal/pkg/utils/response"
	"hivemtk-user/internal/repository"

	"github.com/gin-gonic/gin"
)

// BridgeTokenController 桥接通道凭证管理（v3 BRIDGE_TOKEN_PROTOCOL 阶段一落地）
//
// GET  /api/bridge/token/status  —— 查询配置状态（不回显明文）
// POST /api/bridge/token/reset   —— 轮换：旧值转 PREV，生成新值返回（仅此一次明文）
//
// 存储优先级：DB system_config_kv（运行时可变）> 环境变量 BRIDGE_INGEST_TOKEN。
type BridgeTokenController struct {
	kv repository.SystemConfigKVRepository
}

func NewBridgeTokenController() *BridgeTokenController {
	return &BridgeTokenController{kv: repository.NewSystemConfigKVRepository()}
}

const (
	bridgeTokenKVKey     = "bridge_ingest_token"
	bridgeTokenPrevKVKey = "bridge_ingest_token_prev"
)

// generateBridgeToken 32 字节随机 → base64url（43 字符，无填充）
func generateBridgeToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GetBridgeTokenStatus godoc
// @Summary      桥接凭证状态
// @Description  返回主/灰度 token 是否已配置（不回显明文）
// @Tags         Bridge
// @Security     BearerAuth
// @Success      200 {object} response.Response
// @Router       /api/bridge/token/status [get]
func (c *BridgeTokenController) GetStatus(ctx *gin.Context) {
	status := gin.H{
		"main_configured": false,
		"prev_configured": false,
		"source":          "unset",
	}
	if v, err := c.kv.Get(ctx.Request.Context(), bridgeTokenKVKey); err == nil && v != "" {
		status["main_configured"] = true
		status["source"] = "db"
	} else if os.Getenv("BRIDGE_INGEST_TOKEN") != "" {
		status["main_configured"] = true
		status["source"] = "env"
	}
	if v, err := c.kv.Get(ctx.Request.Context(), bridgeTokenPrevKVKey); err == nil && v != "" {
		status["prev_configured"] = true
	}
	response.Success(ctx, status, "ok")
}

// ResetBridgeToken godoc
// @Summary      轮换桥接凭证
// @Description  旧 token 转入灰度位（PREV），生成并返回新 token 明文（仅此一次）
// @Tags         Bridge
// @Security     BearerAuth
// @Success      200 {object} response.Response
// @Router       /api/bridge/token/reset [post]
func (c *BridgeTokenController) ResetBridgeToken(ctx *gin.Context) {
	newTok, err := generateBridgeToken()
	if err != nil {
		response.Error(ctx, http.StatusInternalServerError, "生成凭证失败")
		return
	}
	cctx := ctx.Request.Context()
	// 当前值 → PREV（双值灰度窗口）
	if cur, gerr := c.kv.Get(cctx, bridgeTokenKVKey); gerr == nil && cur != "" {
		_, _ = c.kv.Upsert(cctx, bridgeTokenPrevKVKey, cur)
	}
	if _, uerr := c.kv.Upsert(cctx, bridgeTokenKVKey, newTok); uerr != nil {
		response.Error(ctx, http.StatusInternalServerError, "持久化凭证失败")
		return
	}
	response.Success(ctx, gin.H{"token": newTok}, "轮换成功；旧凭据进入灰度窗口，请更新浏览器扩展后移除 PREV")
}
