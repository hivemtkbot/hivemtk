package service

import (
	"context"
	"math/rand"
	"regexp"
	"strings"
)


// HumanizePolisher 拟人润色器
type HumanizePolisher struct {
	enableParticles  bool 
	enableTruncation bool 
	maxLength        int  
	platformStyle    PlatformStyle
	particlePool     []string            
	emojiPool        map[string][]string 
}

// PlatformStyle 平台风格
type PlatformStyle struct {
	AllowEmoji     bool   
	AllowFormality bool   
	StyleName      string 
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
		maxLength:        200,
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

	polished = p.removeAITraces(ctx, polished)

	polished = p.removeExtraSymbols(ctx, polished)

	polished = filterExtremeClaims(polished)

	polished = p.applyPlatformStyle(ctx, polished, pctx)

	if p.enableTruncation {
		polished = p.truncateByLength(ctx, polished, p.maxLength)
	}

	if pctx != nil && pctx.CustomerName != "" {
		polished = p.personalize(ctx, polished, pctx.CustomerName, pctx.Platform)
	}

	if pctx == nil || !p.shouldPreserveLeadingAck(pctx) {
		polished = p.removeLeadingFlattery(polished)
	}

	if p.enableParticles && pctx != nil && p.shouldAddParticle(ctx, pctx) {
		polished = p.addNaturalParticle(ctx, polished, pctx)
	}

	polished = maybeInjectTypo(polished, pctx)

	return strings.TrimSpace(polished), nil
}

// shouldPreserveLeadingAck 是否保留句首"好的"等客套过渡(用于投诉/挽留/售后场景的安抚话术)。
func (p *HumanizePolisher) shouldPreserveLeadingAck(pctx *PolishContext) bool {
	if pctx == nil {
		return false
	}
	switch pctx.Intent {
	case IntentComplaint, IntentChurn, IntentAfterSale:
		return true
	}
	return false
}

// removeLeadingFlattery 清理句首纯客套过渡词（"好的"/"可以的"等 + 分隔符）。
// P2-5 修复:由 Polish 调用,complaint 场景由 shouldPreserveLeadingAck 跳过此步。
//
// 严格定义"句首客套"为"客套词 + 分隔符 + 非空后续内容":
//   - "好的，xxx"  →  "xxx"      ✅ 删除
//   - "好的😊 xxx"  →  " xxx"     ✅ 删除(emoji 视为分隔符)
//   - "好的"        →  "好的"     ❌ 保留(整段作有效回答,如"有问题吗"→"好的")
//   - "好的 xxx"   →  "好的 xxx"  ❌ 保留(中间是空格+普通内容,不是分隔符 → 视为"好的"作实词)
func (p *HumanizePolisher) removeLeadingFlattery(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return text
	}
	trimPrefixes := []string{"好的", "可以的", "没问题", "当然可以"}
	for _, prefix := range trimPrefixes {
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		after := strings.TrimPrefix(trimmed, prefix)
		after = strings.TrimLeft(after, " 　\t")
		if after == "" {
			return text
		}
		first := []rune(after)[0]
		isSeparator := first == ',' || first == '，' || first == '.' || first == '。' ||
			first == '!' || first == '！' || first == '?' || first == '？' ||
			first == ':' || first == '：' || first == ';' || first == '；'
		isEmoji := first >= 0x1F000
		if isSeparator || isEmoji {
			return strings.TrimLeft(after, " 　\t")
		}
		return text
	}
	return text
}

// removeAITraces 去除 AI 痕迹
func (p *HumanizePolisher) removeAITraces(ctx context.Context, text string) string {
	aiTraces := []string{
		"作为 AI 助手",
		"作为人工智能",
		"我是 AI",
		"我是人工智能",
		"我是一个 AI",
		"作为一个",
		"我的能力有限",
		"根据您提供的信息",
		"我理解您的",
		"作为您的销售顾问",
		"非常感谢您的咨询",
		"非常感谢您的提问",
		"感谢您的咨询",
		"感谢您的提问",
		"感谢您的关注",
		"很高兴为您服务",
		"很高兴能帮到您",
		"很高兴为您解答",
		"很荣幸为您服务",
		"非常荣幸为您介绍",
		"感谢您的信任",
		"非常感谢您的信任",
		"很乐意为您解答",
		"随时为您服务",
		"期待您的回复",
		"期待您的反馈",
		"可以的！",
		"可以的!",
		"好的！",
		"好的!",
		"没问题！",
		"没问题!",
		"当然可以！",
		"当然可以!",
		"很抱歉，我无法",
		"很抱歉，无法",
		"非常抱歉，给您带来不便",
		"造成不便深表歉意",
		"请您谅解",
		"请谅解",
	}
	for _, trace := range aiTraces {
		text = strings.ReplaceAll(text, trace, "")
	}
	return text
}

// adLawReplacements 广告法极限词过滤表（H-5/X-3，集中定义）。
// Polisher 输出前将《广告法》禁用的绝对化用词替换为合规表述。
// 注意：长词必须排在短词前（顺序替换），且避免误伤正常词（如"最近"不处理，
// "第一"仅处理"销量第一/行业第一/第一品牌"等组合，不动"第一次"）。
var adLawReplacements = []struct{ From, To string }{
	{"全网最低价", "价格很有优势"},
	{"全网最低", "价格很有优势"},
	{"史上最低", "很有竞争力的价格"},
	{"国家级", "高标准"},
	{"世界级", "高标准"},
	{"销量第一", "销量领先"},
	{"行业第一", "行业领先"},
	{"排名第一", "排名靠前"},
	{"第一品牌", "领先品牌"},
	{"百分百", "很大程度上"},
	{"100%", "很高比例的"},
	{"最低价", "实惠的价格"},
	{"最先进", "先进"},
	{"最好", "很好"},
	{"最优", "优秀"},
	{"最强", "很强"},
	{"最佳", "优选"},
	{"根治", "有效改善"},
	{"绝对", "确实"},
	{"顶级", "高端"},
	{"极致", "出色"},
}

// filterExtremeClaims 过滤广告法极限词（H-5/X-3）
func filterExtremeClaims(text string) string {
	for _, r := range adLawReplacements {
		text = strings.ReplaceAll(text, r.From, r.To)
	}
	return text
}

// removeExtraSymbols 去除多余符号
func (p *HumanizePolisher) removeExtraSymbols(ctx context.Context, text string) string {
	multiBang := regexp.MustCompile(`[!！]{2,}`)
	text = multiBang.ReplaceAllString(text, "！")

	multiQuestion := regexp.MustCompile(`[?？]{2,}`)
	text = multiQuestion.ReplaceAllString(text, "？")

	multiDot := regexp.MustCompile(`\.{3,}`)
	text = multiDot.ReplaceAllString(text, "……")

	return text
}

// applyPlatformStyle 应用平台风格
func (p *HumanizePolisher) applyPlatformStyle(ctx context.Context, text string, pctx *PolishContext) string {
	if pctx == nil || pctx.Platform == "" {
		return text
	}
	style := p.getStyleForPlatform(ctx, pctx.Platform)

	if !style.AllowEmoji {
		emojiRegex := regexp.MustCompile(`[\x{1F300}-\x{1FAFF}\x{2600}-\x{27BF}]`)
		text = emojiRegex.ReplaceAllString(text, "")
	}

	if style.AllowFormality {
		text = strings.ReplaceAll(text, "嗯", "是的")
		text = strings.ReplaceAll(text, "哈哈", "呵呵")
	}

	return text
}

// getStyleForPlatform 获取平台风格
func (p *HumanizePolisher) getStyleForPlatform(ctx context.Context, platform string) PlatformStyle {
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
func (p *HumanizePolisher) truncateByLength(ctx context.Context, text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text
	}
	if maxLen > 0 {
		return string(runes[:maxLen-1]) + "…"
	}
	return text
}

// personalize 个性化称呼
func (p *HumanizePolisher) personalize(ctx context.Context, text, name, platform string) string {
	if name == "" {
		return text
	}
	if strings.Contains(text, name) || strings.Contains(text, "亲") || strings.Contains(text, "您") {
		return text
	}
	return name + "，" + text
}

// shouldAddParticle 是否应添加语气词
func (p *HumanizePolisher) shouldAddParticle(ctx context.Context, pctx *PolishContext) bool {
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
func (p *HumanizePolisher) addNaturalParticle(ctx context.Context, text string, pctx *PolishContext) string {
	if text == "" || len([]rune(text)) > 60 {
		return text
	}
	shortReply := map[string]bool{
		"好的": true, "是的": true, "可以": true, "没问题": true, "OK": true,
	}
	if shortReply[text] {
		return "嗯，" + text
	}
	return text
}


var typoPool = map[string][]string{
	"好的":   {"好哒", "ok", "好的呀", "好滴"},
	"好":     {"好的~", "ok", "好哒"},
	"是的":   {"对的", "嗯嗯", "是哒"},
	"好的呢": {"好的呀", "好的~"},
}

func maybeInjectTypo(text string, pctx *PolishContext) string {
	if pctx == nil {
		return text
	}
	persona := strings.ToLower(pctx.Persona)
	if strings.Contains(persona, "专业") || strings.Contains(persona, "客服") {
		return text
	}
	if rand.Float64() > 0.1 {
		return text
	}
	for orig, variants := range typoPool {
		if strings.HasSuffix(text, orig) {
			replacement := variants[rand.Intn(len(variants))]
			text = text[:len(text)-len(orig)] + replacement
			break
		}
	}
	return text
}

