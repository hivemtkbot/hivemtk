// Package text 提供文本处理工具
package text

import "strings"

// StripHTML 简单 HTML 标签剥离(含移除 script/style 块)
func StripHTML(html string) string {
	html = StripBetween(html, "<script", "</script>")
	html = StripBetween(html, "<style", "</style>")

	var result strings.Builder
	inTag := false
	for _, r := range html {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			result.WriteRune(' ')
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// StripBetween 移除 start..end(含)之间所有成对出现的段落
func StripBetween(s, start, end string) string {
	for {
		si := strings.Index(s, start)
		if si < 0 {
			return s
		}
		ei := strings.Index(s[si:], end)
		if ei < 0 {
			return s
		}
		s = s[:si] + s[si+ei+len(end):]
	}
}

// Truncate 截断字符串到 max 字符,超出部分用 "…" 补足
func Truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
