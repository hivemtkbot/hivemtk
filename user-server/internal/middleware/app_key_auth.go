package middleware

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"marketing/internal/model"
	"marketing/internal/service"
)

// AppKeyResolve AppKey 软解析中间件（不再强制鉴权）
//
// 设计原则（2026-07-17 私域部署优化）：
//  1. 不再强制要求 X-Chat-App-Key Header
//  2. 如果请求带 X-Chat-App-Key -> 解析出 channel 注入上下文（保留溯源能力）
//  3. 如果请求带 X-Chat-Channel-Id Header 或 body.channel_id -> 用 channel_id 反查 channel
//  4. 都未提供 -> 注入 DEFAULT channel（"default"）并放行
//  5. 渠道禁用返回 403（防误用）
//  6. 不再做 Origin 白名单阻断（仅记录，不影响访问）
//
// 使用方式：r.Group("/api/chat/public").Use(AppKeyResolve(channelSvc))
//
// 适用场景：用户自己部署本系统后，作为通道嵌入到自有网站，无需 AppKey 鉴权。
// AppKey 仍保留为渠道的软标识（用于日志追踪 + 未来多渠道管理），但不再作为强制凭证。
func AppKeyResolve(channelSvc *service.ChatChannelService) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 测试模式直接放行
		if IsTestMode && testing.Testing() {
			c.Next()
			return
		}

		// 解析顺序：X-Chat-App-Key > X-Chat-Channel-Id > body.channel_id
		appKey := strings.TrimSpace(c.GetHeader("X-Chat-App-Key"))
		channelIDHeader := strings.TrimSpace(c.GetHeader("X-Chat-Channel-Id"))
		channelIDQuery := strings.TrimSpace(c.Query("channel_id"))

		// 先尝试 AppKey 解析
		if appKey != "" {
			channel, err := channelSvc.GetByAppKey(c.Request.Context(), appKey)
			if err == nil {
				if !service.ChatChannelIsActive(channel) {
					c.JSON(http.StatusForbidden, gin.H{
						"code":    403,
						"message": "渠道已被禁用",
					})
					c.Abort()
					return
				}
				c.Set("chat_channel", channel)
				c.Set("chat_channel_id", channel.ChannelID)
				c.Set("chat_app_key", appKey)
				c.Next()
				return
			}
			// AppKey 解析失败 -> 降级：不阻断，继续走 channel_id 流程
		}

		// 兜底：取 channel_id（header > query > 默认）
		channelID := channelIDHeader
		if channelID == "" {
			channelID = channelIDQuery
		}
		if channelID == "" {
			channelID = DefaultChannelID
		}

		// 解析 channel
		channel, err := channelSvc.GetByChannelID(c.Request.Context(), channelID)
		if err != nil {
			// 没有这个 channel，使用 placeholder 放行
			// 让业务层 controller 内部决定如何处理
			placeholder := &model.ChatChannel{
				ChannelID:   channelID,
				ChannelName: channelID,
				Status:      model.ChatChannelStatusActive,
			}
			c.Set("chat_channel", placeholder)
			c.Set("chat_channel_id", channelID)
			c.Next()
			return
		}

		if !service.ChatChannelIsActive(channel) {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "渠道已被禁用",
			})
			c.Abort()
			return
		}

		// Origin 软记录：仅记录到 header，不阻断
		origin := c.GetHeader("Origin")
		if origin != "" {
			origins := service.ChatChannelAllowedOriginsList(channel)
			allowed := false
			for _, o := range origins {
				if o == "*" || strings.EqualFold(o, origin) {
					allowed = true
					break
				}
			}
			if !allowed && len(origins) > 0 {
				c.Header("X-Origin-Not-Allowed", origin)
			}
		}

		c.Set("chat_channel", channel)
		c.Set("chat_channel_id", channel.ChannelID)
		c.Next()
	}
}

// GetChatChannel 从上下文获取 chat channel（由 AppKeyResolve 注入）
// 即使没有真实 channel，也会返回一个 placeholder（含 ChannelID）
func GetChatChannel(c *gin.Context) (*model.ChatChannel, bool) {
	v, ok := c.Get("chat_channel")
	if !ok {
		return nil, false
	}
	ch, ok := v.(*model.ChatChannel)
	return ch, ok
}

// GetChatChannelID 软获取 channel_id（一定返回非空）
// 如果上下文中没有 channel_id，会从 X-Chat-Channel-Id Header / query 中提取，最后 fallback 到 DefaultChannelID
func GetChatChannelID(c *gin.Context) string {
	if v, ok := c.Get("chat_channel_id"); ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	if h := strings.TrimSpace(c.GetHeader("X-Chat-Channel-Id")); h != "" {
		return h
	}
	if q := strings.TrimSpace(c.Query("channel_id")); q != "" {
		return q
	}
	return DefaultChannelID
}

// DefaultChannelID 默认渠道 ID（无渠道时使用）
// 私域部署：单租户，所有未指定 channel 的访客都归入此 channel
const DefaultChannelID = "default"
