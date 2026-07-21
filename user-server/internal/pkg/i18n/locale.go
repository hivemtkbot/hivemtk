// Package i18n 提供后端业务层（Go 返回给前端的提示/消息）的多语言能力。
//
// 设计边界（关键）：
//   - 本包只负责「业务层返回给前端的文案」本地化（API 响应 message、错误提示、通知文案等）。
//   - LLM 调用（system prompt / 大模型生成内容）不在此处理 —— 大模型自带多语言能力，
//     由前端把用户语言透传给对话接口即可，后端无需翻译模型提示词。
package i18n

import (
	"strings"
)

// Locale 语言标识（内部统一用小写短码）
type Locale string

const (
	ZH Locale = "zh" // 简体中文（默认）
	EN Locale = "en" // English
	JA Locale = "ja" // 日本語
	AR Locale = "ar" // العربية
)

// String 返回短码
func (l Locale) String() string { return string(l) }

// IsRTL 阿拉伯语为从右向左排版
func (l Locale) IsRTL() bool { return l == AR }

// AllLocales 返回全部支持的语言
func AllLocales() []Locale {
	return []Locale{ZH, EN, JA, AR}
}

// Parse 将任意输入（header / 参数 / 名称）规范化为支持的语言，无法识别时回退中文。
func Parse(s string) Locale {
	s = strings.TrimSpace(strings.ToLower(s))
	switch {
	case s == "":
		return ZH
	case strings.HasPrefix(s, "zh"):
		return ZH
	case strings.HasPrefix(s, "en"):
		return EN
	case s == "ja" || s == "jp" || strings.HasPrefix(s, "ja") || strings.Contains(s, "japanese"):
		return JA
	case s == "ar" || strings.Contains(s, "arabic"):
		return AR
	default:
		return ZH
	}
}

// ParseAcceptLanguage 解析 HTTP Accept-Language 头（如 "en-US,en;q=0.9,zh;q=0.8"），
// 返回第一个受支持的语言。
func ParseAcceptLanguage(header string) Locale {
	header = strings.TrimSpace(header)
	if header == "" {
		return ZH
	}
	for _, part := range strings.Split(header, ",") {
		// 去掉权重 ";q=0.8"
		tag := part
		if idx := strings.Index(tag, ";"); idx >= 0 {
			tag = tag[:idx]
		}
		tag = strings.TrimSpace(tag)
		// 取主语言子标签（en-US -> en）
		if idx := strings.Index(tag, "-"); idx >= 0 {
			tag = tag[:idx]
		}
		if loc := Parse(tag); loc != ZH || tag == "zh" {
			return loc
		}
	}
	return ZH
}

// DetectText 根据文本内容（字符脚本）推断语言，用于让业务提示跟随用户消息语言。
// 顺序：阿拉伯语 -> 日语（含假名）-> 中文（CJK）-> 默认英文。
func DetectText(text string) Locale {
	if text == "" {
		return ZH
	}
	for _, r := range text {
		if (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) || (r >= 0x08A0 && r <= 0x08FF) {
			return AR // 阿拉伯文
		}
	}
	for _, r := range text {
		if (r >= 0x3040 && r <= 0x30FF) || (r >= 0x31F0 && r <= 0x31FF) || (r >= 0xFF65 && r <= 0xFF9F) {
			return JA // 平假名/片假名/半角片假名
		}
	}
	for _, r := range text {
		if r >= 0x4E00 && r <= 0x9FFF {
			return ZH // 中日韩统一表意文字（本项目主语言为中文）
		}
	}
	return EN
}
