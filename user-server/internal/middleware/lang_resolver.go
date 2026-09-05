package middleware

import (
	"context"
	"strconv"

	"github.com/gin-gonic/gin"

	"hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/service/translation"
)

// LangResolverMiddleware 语言解析中间件（gin 版本）。
//
// 用法：
//
//	r.Group("/api/chat/public").Use(middleware.LangResolverMiddleware(resolver))
//
// resolver 为 nil 时直接注入默认值（zh），保证主流程不中断。
func LangResolverMiddleware(resolver *translation.LangConfigResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := GetChatChannelID(c)

		agentID := c.GetUint("agent_id")
		if agentID == 0 {
			if aid := c.Query("agent_id"); aid != "" {
				if n, err := strconv.ParseUint(aid, 10, 64); err == nil {
					agentID = uint(n)
				}
			}
		}

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

func injectDefaultLang(ctx context.Context) context.Context {
	ctx = i18n.WithInternalLang(ctx, "zh")
	ctx = i18n.WithTargetLang(ctx, "zh")
	ctx = i18n.WithCrossLingual(ctx, false)
	return ctx
}
