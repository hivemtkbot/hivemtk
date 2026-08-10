package middleware

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/service/translation"
)

// ============================================================================
// LangResolverMiddleware 多语言解析中间件（v1.2 出海方案 - 消息入口集成）
// ----------------------------------------------------------------------------
// 设计目标：
//   - 在 HTTP 消息入口（chat / webhook 等）调用 LangConfigResolver 解析双语言
//   - 多层兜底：resolver 未注入 / 解析失败 / channel_id 与 agent_id 缺失，
//     任何情况都向 ctx 注入默认值（internal=zh, target=zh, cross_lingual=false），
//     永不中断主流程
//
// 数据来源：
//   - channel_id：优先复用 AppKeyResolve 注入的 chat_channel_id，
//     兜底走 GetChatChannelID（header / query / 默认 "default"）
//   - agent_id：优先 c.GetUint("agent_id")，兜底 c.Query("agent_id")
//     （注意：HTTP 入口通常无 AI Agent ID，留 0 时 resolver 走 channel 兜底）
// ============================================================================

// LangResolverMiddleware 语言解析中间件（gin 版本）。
//
// 用法：
//
//	r.Group("/api/chat/public").Use(middleware.LangResolverMiddleware(resolver))
//
// resolver 为 nil 时直接注入默认值（zh），保证主流程不中断。
func LangResolverMiddleware(resolver *translation.LangConfigResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. 提取 channel_id（复用 AppKeyResolve 的结果，兜底走软解析）
		channelID := GetChatChannelID(c)

		// 2. 提取 agent_id（HTTP 入口通常缺失，0 时 resolver 走 channel 兜底）
		agentID := c.GetUint("agent_id")
		if agentID == 0 {
			if aid := c.Query("agent_id"); aid != "" {
				if n, err := strconv.ParseUint(aid, 10, 64); err == nil {
					agentID = uint(n)
				}
			}
		}

		// 3. 单次解析 + 多层兜底注入（永不报错）
		ctx := c.Request.Context()
		var internalLang, targetLang, langSource string
		var crossLingual bool

		if resolver != nil {
			result, err := resolver.Resolve(ctx, channelID, agentID)
			if err == nil && result != nil {
				ctx = i18n.WithInternalLang(ctx, result.InternalLang)
				ctx = i18n.WithTargetLang(ctx, result.TargetLang)
				ctx = i18n.WithCrossLingual(ctx, result.CrossLingual)
				internalLang = result.InternalLang
				targetLang = result.TargetLang
				crossLingual = result.CrossLingual
				langSource = result.TargetSrc
			} else {
				ctx = injectDefaultLang(ctx)
				internalLang = "zh"
				targetLang = "zh"
				langSource = "default"
			}
		} else {
			ctx = injectDefaultLang(ctx)
			internalLang = "zh"
			targetLang = "zh"
			langSource = "default"
		}

		// 4. 同步到 gin context 便于日志 / 调试读取（与 ctx 保持一致）
		c.Set("internal_lang", internalLang)
		c.Set("target_lang", targetLang)
		c.Set("cross_lingual", crossLingual)
		c.Set("lang_source", langSource)

		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// InjectLangToCtx 纯 context 版本（用于非 gin 场景，如 WebSocket handler）。
//
// 多层兜底：
//   - resolver 为 nil → 注入默认 zh
//   - resolver.Resolve 报错或返回 nil → 注入默认 zh
//   - 正常解析 → 注入 result 中的语言
//
// 调用方典型用法：
//
//	ctx = middleware.InjectLangToCtx(ctx, langResolver, channelID, agentID)
func InjectLangToCtx(ctx context.Context, resolver *translation.LangConfigResolver, channelID string, agentID uint) context.Context {
	if resolver == nil {
		return injectDefaultLang(ctx)
	}
	result, err := resolver.Resolve(ctx, channelID, agentID)
	if err != nil || result == nil {
		return injectDefaultLang(ctx)
	}
	ctx = i18n.WithInternalLang(ctx, result.InternalLang)
	ctx = i18n.WithTargetLang(ctx, result.TargetLang)
	ctx = i18n.WithCrossLingual(ctx, result.CrossLingual)
	return ctx
}

// injectDefaultLang 注入默认语言（zh + 非跨语言）。
// 用于 resolver 缺失 / 解析失败的兜底场景。
func injectDefaultLang(ctx context.Context) context.Context {
	ctx = i18n.WithInternalLang(ctx, "zh")
	ctx = i18n.WithTargetLang(ctx, "zh")
	ctx = i18n.WithCrossLingual(ctx, false)
	return ctx
}
