package i18n

// DetectLangCode 基于 Unicode 脚本做轻量语言检测，用于"按客户语言回复"的自动路由。
//
// 设计原则：
//   - 零依赖、零 IO、纯函数，可直接在生成链路热路径调用。
//   - 仅对"脚本可明显区分"的语言返回确定结果（中文/日文/韩文/阿拉伯文/泰文/俄文/
//     印地文/越南文）；对纯拉丁文本（英文/德文/法文等无法仅靠脚本区分）返回 ""，
//     由调用方回退到内部语种，避免中文坐席场景下用户随手敲的英文单词被误判为切换语言。
//   - 返回值为 NormalizeLang 认可的合法语言码；无法判定时返回 ""。
func DetectLangCode(text string) string {
	if text == "" {
		return ""
	}
	counts := map[string]int{}
	for _, r := range text {
		switch {
		case isHiragana(r) || isKatakana(r):
			counts["ja"]++
		case isHangul(r):
			counts["ko"]++
		case isCyrillic(r):
			counts["ru"]++
		case isArabic(r):
			counts["ar"]++
		case isThai(r):
			counts["th"]++
		case isDevanagari(r):
			counts["hi"]++
		case isCJK(r):
			counts["zh"]++
		case isVietnamese(r):
			counts["vi"]++
		default:
		}
	}
	best := ""
	bestN := 0
	for lang, n := range counts {
		if n > bestN {
			bestN = n
			best = lang
		}
	}
	if best == "" {
		return ""
	}
	return NormalizeLang(best)
}

func isHiragana(r rune) bool {
	return r >= 0x3040 && r <= 0x309F
}

func isKatakana(r rune) bool {
	return (r >= 0x30A0 && r <= 0x30FF) || (r >= 0xFF65 && r <= 0xFF9F) 
}

func isHangul(r rune) bool {
	return (r >= 0x1100 && r <= 0x11FF) || (r >= 0xAC00 && r <= 0xD7A3) || (r >= 0x3130 && r <= 0x318F)
}

func isCyrillic(r rune) bool {
	return (r >= 0x0400 && r <= 0x04FF) || (r >= 0x0500 && r <= 0x052F)
}

func isArabic(r rune) bool {
	return (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F) ||
		(r >= 0x08A0 && r <= 0x08FF) || (r >= 0xFB50 && r <= 0xFDFF) || (r >= 0xFE70 && r <= 0xFEFF)
}

func isThai(r rune) bool {
	return r >= 0x0E00 && r <= 0x0E7F
}

func isDevanagari(r rune) bool {
	return r >= 0x0900 && r <= 0x097F
}

func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0xF900 && r <= 0xFAFF)
}

// isVietnamese 仅识别越南语独有的字母（đ/Đ/ă/Ă/ơ/Ơ/ư/Ư），
// 避免与同样带变音符号的法文/西班牙文等混淆。
func isVietnamese(r rune) bool {
	switch r {
	case 0x0111, 0x0110, 
		0x0103, 0x0102, 
		0x01A1, 0x01A0, 
		0x01B0, 0x01AF: 
		return true
	}
	return false
}

