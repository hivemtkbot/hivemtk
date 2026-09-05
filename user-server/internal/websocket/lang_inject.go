package websocket

import (
	"context"

	"hivemtk-user/internal/pkg/i18n"
	"hivemtk-user/internal/service/translation"
)

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

func injectDefaultLang(ctx context.Context) context.Context {
	ctx = i18n.WithInternalLang(ctx, "zh")
	ctx = i18n.WithTargetLang(ctx, "zh")
	ctx = i18n.WithCrossLingual(ctx, false)
	return ctx
}
