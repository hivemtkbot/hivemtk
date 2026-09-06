package text

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
	tokens := (ascii + 3) / 4
	tokens += (wide + 1) / 2
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
