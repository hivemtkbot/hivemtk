// Package i18n 提供 v1.2 出海多语言方案的服务层组件。
//
// 本包与 internal/pkg/i18n（业务文案本地化）解耦：
//   - pkg/i18n：Go 后端返回给前端的 API 提示本地化（Locale / Messages）
//   - service/translation：LLM 调用 / 内容生成链路的双语言解析、术语表、后置校准
//
// 组件：
//   - LangConfigResolver：双语言配置读取（多层兜底，零状态）
//   - GlossaryService   ：术语表加载 + system prompt 渲染（带 Redis 缓存）
//   - PostValidator     ：LLM 输出后置校准（正则保护 + 术语校准）
package translation

import (
	"context"

	"hivemtk-user/internal/model"
	i18npkg "hivemtk-user/internal/pkg/i18n"
)

// ChannelReader 渠道读取接口（避免 service 层直接依赖 repository 包）。
// 由 repository.ChatChannelRepository 实现。
type ChannelReader interface {
	GetByChannelID(ctx context.Context, channelID string) (*model.ChatChannel, error)
}

// AgentReader 智能体读取接口。
// 由 repository.AIAgentRepository 实现。
type AgentReader interface {
	GetByID(ctx context.Context, id uint) (*model.AIAgent, error)
}

// LangResolveResult 语言解析结果。
//
// 字段说明：
//   - InternalLang / TargetLang：始终为受支持的语言短码（已 NormalizeLang）
//   - InternalSrc / TargetSrc  ：配置来源标记，便于排查与日志
//   - CrossLingual              ：是否需要跨语言生成
type LangResolveResult struct {
	InternalLang string
	TargetLang   string
	CrossLingual bool
	InternalSrc  string
	TargetSrc    string
	ChannelID    string
	AgentID      uint
}

// LangConfigResolver 双语言配置读取器。
type LangConfigResolver struct {
	channelRepo ChannelReader
	agentRepo   AgentReader
}

// NewLangConfigResolver 构造 LangConfigResolver。
// channelRepo / agentRepo 可为 nil（仅在不涉及对应实体时安全）。
func NewLangConfigResolver(channelRepo ChannelReader, agentRepo AgentReader) *LangConfigResolver {
	return &LangConfigResolver{channelRepo: channelRepo, agentRepo: agentRepo}
}

// Resolve 双语言解析（多层兜底：渠道 → 智能体 → 默认 "zh"）。
//
// 即使所有配置缺失或仓储报错，也保证返回有效语言（默认 "zh"），
// 第二个返回值永远为 nil（保留 error 槽位仅为未来扩展兼容）。
func (r *LangConfigResolver) Resolve(ctx context.Context, channelID string, agentID uint) (*LangResolveResult, error) {
	result := &LangResolveResult{
		InternalLang: "zh",
		TargetLang:   "zh",
		InternalSrc:  "default",
		TargetSrc:    "default",
		ChannelID:    channelID,
		AgentID:      agentID,
	}

	if agentID > 0 && r.agentRepo != nil {
		if ag, err := r.agentRepo.GetByID(ctx, agentID); err == nil && ag != nil {
			if lang := i18npkg.NormalizeLang(ag.InternalLanguage); lang != "" {
				result.InternalLang = lang
				result.InternalSrc = "agent"
			}
		}
	}

	targetLang := result.InternalLang
	targetSrc := "internal"

	if channelID != "" && r.channelRepo != nil {
		if ch, err := r.channelRepo.GetByChannelID(ctx, channelID); err == nil && ch != nil {
			if lang := i18npkg.NormalizeLang(ch.TargetLanguage); lang != "" && ch.TargetLanguage != "" {
				targetLang = lang
				targetSrc = "channel"
			}
		}
	}

	if targetSrc == "internal" && agentID > 0 && r.agentRepo != nil {
		if ag, err := r.agentRepo.GetByID(ctx, agentID); err == nil && ag != nil {
			if ag.TargetLanguage != "" {
				if lang := i18npkg.NormalizeLang(ag.TargetLanguage); lang != "" {
					targetLang = lang
					targetSrc = "agent"
				}
			}
		}
	}

	result.TargetLang = targetLang
	result.TargetSrc = targetSrc
	result.CrossLingual = result.InternalLang != result.TargetLang

	return result, nil
}

// InjectToCtx 把解析结果注入 ctx（供后续 LLM 调用使用）。
//
// 链路上的下游（如 prompt 装配 / RAG 检索 / 后置校准）通过
// i18n.GetInternalLang / i18n.GetTargetLang / i18n.GetCrossLingual 读取。
func (r *LangConfigResolver) InjectToCtx(ctx context.Context, result *LangResolveResult) context.Context {
	if result == nil {
		return ctx
	}
	ctx = i18npkg.WithInternalLang(ctx, result.InternalLang)
	ctx = i18npkg.WithTargetLang(ctx, result.TargetLang)
	ctx = i18npkg.WithCrossLingual(ctx, result.CrossLingual)
	return ctx
}

// ResolveAndInject 是 Resolve + InjectToCtx 的便捷组合，返回新的 ctx 与结果。
//
// 调用方典型用法：
//
//	ctx, lr := resolver.ResolveAndInject(ctx, channelID, agentID)
//	// 后续基于 ctx 调用 LLM / RAG / GlossaryService
func (r *LangConfigResolver) ResolveAndInject(ctx context.Context, channelID string, agentID uint) (context.Context, *LangResolveResult) {
	result, _ := r.Resolve(ctx, channelID, agentID)
	return r.InjectToCtx(ctx, result), result
}
