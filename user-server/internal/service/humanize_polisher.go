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

	// 6. 句首客套前缀清理（仅在非 complaint/after_sale/churn 场景下启用,避免误伤投诉安抚话术）
	//   P2-5 修复:把"好的，xxx"这类句首纯客套过渡词在文本层面去掉,统一收件箱展示更紧凑。
	//   白名单:complaint/churn/after_sale → 不清理,保留"好的,让我帮您处理..."这类安抚过渡。
	if pctx == nil || !p.shouldPreserveLeadingAck(pctx) {
		polished = p.removeLeadingFlattery(polished)
	}

	// 7. 自然语气（轻量添加，不强制）
	if p.enableParticles && pctx != nil && p.shouldAddParticle(ctx, pctx) {
		polished = p.addNaturalParticle(ctx, polished, pctx)
	}

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
			// 文本只有"好的"本身 → 整段是有效回答(单字回复),保留
			return text
		}
		first := []rune(after)[0]
		isSeparator := first == ',' || first == '，' || first == '.' || first == '。' ||
			first == '!' || first == '！' || first == '?' || first == '？' ||
			first == ':' || first == '：' || first == ';' || first == '；'
		// 启发式判断 emoji: 首字节高 4 位 >= 0xF0 (UTF-8 4 字节) 或 surrogate 高代理
		// 注意: 中文字符 (U+4E00..U+9FFF) 是 3 字节,首字节 0xE0..0xEF,不算 emoji
		isEmoji := first >= 0x1F000
		if isSeparator || isEmoji {
			return strings.TrimLeft(after, " 　\t")
		}
		// 不是分隔符(如空格) → "好的"是实词,保留
		return text
	}
	return text
}

// removeAITraces 去除 AI 痕迹
func (p *HumanizePolisher) removeAITraces(ctx context.Context, text string) string {
	// 2026-08-15 P2-5 修复：扩充客套话/模板化清单
	//   原清单只匹配 "作为 AI 助手" 等 11 个关键词,实际 LLM 输出了大量"可以的！😊" +
	//   空泛寒暄 + 模板列表的"客套话",原清单 100% 漏判,导致统一收件箱展示出来一眼假。
	//   新增 4 类识别:
	//     1) 自报家门(我是助手/我是 AI/作为您的销售...)— 已有
	//     2) 空泛寒暄(非常感谢您的咨询/很高兴为您服务/感谢您的提问)→ 整句去除
	//     3) 单一应答客套(可以的!/好的!/没问题!+😊)→ 整句去除(后续是具体内容时去除客套前缀即可)
	//     4) 模板化道歉(很抱歉/请谅解/造成不便深表歉意)→ 整句去除
	//   私域部署原则:每条都做兜底字符串匹配,宁多杀不放过,避免重复套话污染下游。
	aiTraces := []string{
		// 1) 自报家门
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
		// 2) 空泛寒暄(P2-5 新增)
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
		// 3) 单一应答客套(P2-5 新增,带/不带空格/标点)
		//   关键约束:只删除纯文本+标点的客套话,**不**带 emoji 的形式(emoji 由 applyPlatformStyle 决定是否保留)
		//   否则 wechat/xhs 等允许 emoji 的平台会把"好的😊"整段删掉,违反 platformStyle。
		"可以的！",
		"可以的!",
		"好的！",
		"好的!",
		"没问题！",
		"没问题!",
		"当然可以！",
		"当然可以!",
		// 4) 模板化道歉(P2-5 新增)
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
	// 句首客套过渡词（"好的"/"可以的"等 + 分隔符）由 Polish.removeLeadingFlattery 处理：
	//   - 该步骤在 PolishContext 已知时执行,complaint/churn/after_sale 场景跳过
	//   - 避免 removeAITraces 在无 PolishContext 时也盲目删除"好的,我帮您..."这种安抚过渡
	return text
}

// removeExtraSymbols 去除多余符号
func (p *HumanizePolisher) removeExtraSymbols(ctx context.Context, text string) string {
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
func (p *HumanizePolisher) applyPlatformStyle(ctx context.Context, text string, pctx *PolishContext) string {
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
func (p *HumanizePolisher) personalize(ctx context.Context, text, name, platform string) string {
	if name == "" {
		return text
	}
	// 如果文本已经包含称呼，跳过
	if strings.Contains(text, name) || strings.Contains(text, "亲") || strings.Contains(text, "您") {
		return text
	}
	// 在开头加称呼（轻量化）
	return name + "，" + text
}

// shouldAddParticle 是否应添加语气词
func (p *HumanizePolisher) shouldAddParticle(ctx context.Context, pctx *PolishContext) bool {
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
func (p *HumanizePolisher) addNaturalParticle(ctx context.Context, text string, pctx *PolishContext) string {
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
