package websocket

import (
	"context"

	"hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/service/translation"
)

// injectLangToCtx 在 WebSocket handler 内部使用的多语言 ctx 注入 helper。
//
// 与 middleware.InjectLangToCtx 行为一致，但放在 websocket 包内以避免
// middleware → service → websocket → middleware 的 import cycle。
//
// 多层兜底：
//   - resolver 为 nil → 注入默认 zh
//   - resolver.Resolve 报错或返回 nil → 注入默认 zh
//   - 正常解析 → 注入 result 中的语言
func injectLangToCtx(ctx context.Context, resolver *translation.LangConfigResolver, channelID string, agentID uint) context.Context {
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
func injectDefaultLang(ctx context.Context) context.Context {
	ctx = i18n.WithInternalLang(ctx, "zh")
	ctx = i18n.WithTargetLang(ctx, "zh")
	ctx = i18n.WithCrossLingual(ctx, false)
	return ctx
}
