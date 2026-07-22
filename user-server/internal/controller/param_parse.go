package controller

import "strconv"

// parsePositiveInt 把字符串解析为正整数；如果解析失败或 <= 0 则返回 def；上限 max。
// 注意：仅本控制器包内复用，避免污染其他包。
func parsePositiveInt(s string, def, max int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	if v > max {
		return max
	}
	return v
}
