package text

// X-4 统一 token 估算工具（全项目唯一实现，A-4/K-7 共用）。
//
// 轻量估算口径（MASTER_COMPETITIVE_DECISIONS.md A-4）：
//   - 中文等宽字符（rune > 0x7F）≈ 字符数/2 个 token
//   - ASCII ≈ 4 字符/token
// 结果向上取整，宁可高估提前截断，不低估撑爆上下文。

// EstimateTokens 估算文本的 token 数
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	ascii, wide := 0, 0
	for _, r := range s {
		if r > 0x7F {
			wide++
		} else {
			ascii++
		}
	}
	tokens := (ascii + 3) / 4 // ceil(ascii/4)
	tokens += (wide + 1) / 2 // ceil(wide/2)
	return tokens
}

// EstimateTokensOf 批量估算多段文本合计 token 数
func EstimateTokensOf(parts ...string) int {
	total := 0
	for _, p := range parts {
		total += EstimateTokens(p)
	}
	return total
}
