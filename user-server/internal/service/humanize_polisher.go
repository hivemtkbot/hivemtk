package service

import (
	"context"
	"regexp"
	"strings"
)

// ============================================================================
// 商业产品级 智能体：拟人度润色器（Humanize Polisher）
// ----------------------------------------------------------------------------
// 商业需求：客户最怕"机器人味儿"。本组件把 LLM 的机械化输出转为真人对话风格。
// 设计目标：
//   1. 减少 AI 痕迹（删"作为 AI 助手..."等套话）
//   2. 增加自然停顿（"嗯"、"那个"、"对了"等语气词）
//   3. 个性化称呼（按客户名/阶段）
//   4. 长度控制（≤80 字，超长截断）
//   5. 平台适配（小红书/微信/抖音不同风格）
// ============================================================================

// HumanizePolisher 拟人润色器
type HumanizePolisher struct {
	// 拟人化策略（可热更新）
	enableParticles  bool // 是否启用语气词
	enableTruncation bool // 是否启用截断
	maxLength        int  // 最长字符
	platformStyle    PlatformStyle
	particlePool     []string            // 语气词池
	emojiPool        map[string][]string // 平台 → emoji 池
}

// PlatformStyle 平台风格
type PlatformStyle struct {
	AllowEmoji     bool   // 是否允许表情
	AllowFormality bool   // 是否允许正式语气
	StyleName      string // 风格标识
}

// PolishContext 润色上下文
type PolishContext struct {
	Persona      string
	Intent       string
	Platform     string
	Stage        string
	CustomerName string
}

// NewHumanizePolisher 创建拟人润色器
func NewHumanizePolisher() *HumanizePolisher {
	return &HumanizePolisher{
		enableParticles:  true,
		enableTruncation: true,
		maxLength:        80,
		platformStyle: PlatformStyle{
			AllowEmoji:     true,
			AllowFormality: false,
			StyleName:      "default",
		},
		particlePool: []string{"嗯", "那个", "对了", "哈哈", "呢", "哦", "啦", "呀"},
		emojiPool: map[string][]string{
			"wechat":      {"😊", "👍", "💪", "✨"},
			"douyin":      {"🎉", "🔥", "💯", "👍"},
			"xiaohongshu": {"🌟", "💕", "✨", "😊"},
			"xianyu":      {"😊", "👍", "🉑"},
			"tiktok":      {"🔥", "💯", "🎉"},
		},
	}
}

// Polish 拟人润色
func (p *HumanizePolisher) Polish(ctx context.Context, raw string, pctx *PolishContext) (string, error) {
	if raw == "" {
		return raw, nil
	}
	polished := raw

	// 1. 去除 AI 痕迹
	polished = p.removeAITraces(ctx, polished)

	// 2. 去除多余符号
	polished = p.removeExtraSymbols(ctx, polished)

	// 3. 平台风格适配
	polished = p.applyPlatformStyle(ctx, polished, pctx)

	// 4. 长度控制
	if p.enableTruncation {
		polished = p.truncateByLength(ctx, polished, p.maxLength)
	}

	// 5. 个性化（客户名）
	if pctx != nil && pctx.CustomerName != "" {
		polished = p.personalize(ctx, polished, pctx.CustomerName, pctx.Platform)
	}

	// 6. 自然语气（轻量添加，不强制）
	if p.enableParticles && pctx != nil && p.shouldAddParticle(ctx, pctx) {
		polished = p.addNaturalParticle(ctx, polished, pctx)
	}

	return strings.TrimSpace(polished), nil
}

// removeAITraces 去除 AI 痕迹
func (p *HumanizePolisher) removeAITraces(ctx context.Context, text string)  string {
	aiTraces := []string{
		"作为 AI 助手",
		"作为人工智能",
		"我是 AI",
		"我是人工智能",
		"很抱歉，我无法",
		"作为一个",
		"我是一个 AI",
		"我的能力有限",
		"根据您提供的信息",
		"我理解您的",
		"作为您的销售顾问",
	}
	for _, trace := range aiTraces {
		text = strings.ReplaceAll(text, trace, "")
	}
	return text
}

// removeExtraSymbols 去除多余符号
func (p *HumanizePolisher) removeExtraSymbols(ctx context.Context, text string)  string {
	// 把连续的"！"压缩成1个（半角 + 全角）
	multiBang := regexp.MustCompile(`[!！]{2,}`)
	text = multiBang.ReplaceAllString(text, "！")

	// 把连续"？"压缩（半角 + 全角）
	multiQuestion := regexp.MustCompile(`[?？]{2,}`)
	text = multiQuestion.ReplaceAllString(text, "？")

	// 把连续 3+ 句号压缩
	multiDot := regexp.MustCompile(`\.{3,}`)
	text = multiDot.ReplaceAllString(text, "……")

	return text
}

// applyPlatformStyle 应用平台风格
func (p *HumanizePolisher) applyPlatformStyle(ctx context.Context, text string, pctx *PolishContext)  string {
	if pctx == nil || pctx.Platform == "" {
		return text
	}
	style := p.getStyleForPlatform(ctx, pctx.Platform)

	// 不允许表情的渠道 → 去除 emoji
	if !style.AllowEmoji {
		emojiRegex := regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)
		text = emojiRegex.ReplaceAllString(text, "")
	}

	// 允许正式语气 → 把"嗯"替换为更正式的"是的"
	if style.AllowFormality {
		text = strings.ReplaceAll(text, "嗯", "是的")
		text = strings.ReplaceAll(text, "哈哈", "呵呵")
	}

	return text
}

// getStyleForPlatform 获取平台风格
func (p *HumanizePolisher) getStyleForPlatform(ctx context.Context, platform string)  PlatformStyle {
	switch strings.ToLower(platform) {
	case "wechat", "weixin":
		return PlatformStyle{AllowEmoji: true, AllowFormality: false, StyleName: "wechat"}
	case "douyin":
		return PlatformStyle{AllowEmoji: true, AllowFormality: false, StyleName: "douyin"}
	case "xiaohongshu", "xhs":
		return PlatformStyle{AllowEmoji: true, AllowFormality: false, StyleName: "xhs"}
	case "xianyu":
		return PlatformStyle{AllowEmoji: true, AllowFormality: false, StyleName: "xianyu"}
	case "tiktok":
		return PlatformStyle{AllowEmoji: true, AllowFormality: false, StyleName: "tiktok"}
	case "whatsapp", "telegram":
		return PlatformStyle{AllowEmoji: true, AllowFormality: true, StyleName: "im"}
	case "email", "mail":
		return PlatformStyle{AllowEmoji: false, AllowFormality: true, StyleName: "email"}
	default:
		return p.platformStyle
	}
}

// truncateByLength 按长度截断
func (p *HumanizePolisher) truncateByLength(ctx context.Context, text string, maxLen int)  string {
	// 按 rune 计数（中文友好）
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	// 截到 maxLen - 1，留 1 个字给 "…"
	if maxLen > 0 {
		return string(runes[:maxLen-1]) + "…"
	}
	return text
}

// personalize 个性化称呼
func (p *HumanizePolisher) personalize(ctx context.Context, text, name, platform string)  string {
	if name == "" {
		return text
	}
	// 如果文本已经包含称呼，跳过
	if strings.Contains(text, name) || strings.Contains(text, "亲") || strings.Contains(text, "您") {
		return text
	}
	// 在开头加称呼（轻量化）
	// 注意：商业产品对称呼很敏感，我们只对"未识别"类回复加称呼
	noPersonalizeIntents := map[string]bool{
		IntentPriceInquiry: true,
		IntentAskProduct:   true,
	}
	// 实际可以根据意图决定，这里保守起见不强制添加
	_ = noPersonalizeIntents
	return text
}

// shouldAddParticle 是否应添加语气词
func (p *HumanizePolisher) shouldAddParticle(ctx context.Context, pctx *PolishContext)  bool {
	// 投诉、流失倾向不添加（避免火上浇油）
	noParticleIntents := map[string]bool{
		IntentComplaint: true,
		IntentChurn:     true,
		IntentAfterSale: true,
	}
	if pctx != nil && noParticleIntents[pctx.Intent] {
		return false
	}
	return true
}

// addNaturalParticle 添加自然语气词
func (p *HumanizePolisher) addNaturalParticle(ctx context.Context, text string, pctx *PolishContext)  string {
	// 极轻量化：仅在首句添加，最多 1 个
	if text == "" || len([]rune(text)) > 60 {
		return text
	}
	// 不在句首添加（避免 LLM 困惑），只对过短的"是/好的"类回复
	shortReply := map[string]bool{
		"好的": true, "是的": true, "可以": true, "没问题": true, "OK": true,
	}
	if shortReply[text] {
		return "嗯，" + text
	}
	return text
}
