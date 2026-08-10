package service

import (
	"crypto/sha256"
	"encoding/hex"
	"hivemtk-user/internal/identity"
	"regexp"
	"strings"
)

// 商业产品级 OneID 规范化工具
// 目标：让"138 0013 8000"、"+86 13800138000"、"138-0013-8000"等不同写法都能归一为同一个客户。
//
// 设计原则：
//  1. 不做模糊匹配：手机号/邮箱/ID 必须是精确归一，不引入概率风险；
//  2. 业务可解释：归一规则公开、可在管理后台查看；
//  3. 可逆：normalizePhone 输出可直接落库作为展示值。

var (
	// 仅保留数字
	phoneDigits = regexp.MustCompile(`\D+`)
	// 邮箱白名单字符
	emailValid = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}$`)
)

// NormalizePhone 将各种格式的手机号统一为 11 位数字（国内）。
// 输入：+86 138 0013 8000 / 138-0013-8000 / 86 13800138000 / 0086 13800138000
// 输出：13800138000
// 非 11 位数字时返回原始 trim 字符串（保留观察）。
func NormalizePhone(raw string) string {
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	// 仅保留数字
	digits := phoneDigits.ReplaceAllString(trimmed, "")
	// 去除 86 前缀（兼容 +86 / 86 / 0086 / 86 等多种国际区号写法）
	switch {
	case len(digits) == 13 && strings.HasPrefix(digits, "86"):
		// 8613800138000 (13 位) → 13800138000
		digits = digits[2:]
	case len(digits) == 15 && strings.HasPrefix(digits, "0086"):
		// 008613800138000 (15 位) → 13800138000
		digits = digits[4:]
	case len(digits) == 15 && strings.HasPrefix(digits, "+86"):
		// 保险：若 phoneDigits 没把 + 去掉导致 15 位
		digits = digits[3:]
	case len(digits) == 14 && strings.HasPrefix(digits, "0086"):
		// 00861380013800 (14 位) 防御性保留
		digits = digits[4:]
	}
	// 归一后非 11 位视为非国内手机号，原样返回 trim 字符串（保留观察）
	if len(digits) != 11 {
		// 若原输入中完全不包含数字（仅含分隔符/字母/标点），返回空字符串
		// 字母场景如 "abcdefghijk" 仍视为非有效手机号，但保留观察用 trimmed
		if digits == "" {
			// 仅当原输入完全是分隔符/标点（无任何字母数字）时返回空
			if hasAnyAlnum(trimmed) {
				return trimmed
			}
			return ""
		}
		return trimmed
	}
	return digits
}

// hasAnyAlnum 字符串中是否包含任何字母或数字
func hasAnyAlnum(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

// NormalizeEmail 邮箱小写、去空格。
// 输入：  Foo@Example.COM
// 输出：foo@example.com
// 不符合邮箱格式时返回原始 trim 字符串。
func NormalizeEmail(raw string) string {
	if raw == "" {
		return ""
	}
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if !emailValid.MatchString(trimmed) {
		return strings.TrimSpace(raw)
	}
	return trimmed
}

// NormalizeOpenID 通用渠道 ID 规范化（去除首尾空格）。
// 注意：不修改大小写，OpenID 在不同平台是大小写敏感的。
func NormalizeOpenID(raw string) string {
	return strings.TrimSpace(raw)
}

// NormalizeIdentifiers 一次性规范化所有身份标识。
func NormalizeIdentifiers(in identity.Identifiers) identity.Identifiers {
	return identity.Identifiers{
		Phone:         NormalizePhone(in.Phone),
		Email:         NormalizeEmail(in.Email),
		WechatOpenID:  NormalizeOpenID(in.WechatOpenID),
		DouyinOpenID:  NormalizeOpenID(in.DouyinOpenID),
		XiaohongshuID: NormalizeOpenID(in.XiaohongshuID),
	}
}

// PhoneHash 返回手机号 SHA-256 哈希（用于脱敏匹配、日志审计）。
// 注意：本项目是单租户私有部署，hash 仅用于日志审计，不用于跨表 join。
func PhoneHash(phone string) string {
	normalized := NormalizePhone(phone)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("oneid_phone_salt::" + normalized))
	return hex.EncodeToString(sum[:])
}

// EmailHash 返回邮箱 SHA-256 哈希。
func EmailHash(email string) string {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("oneid_email_salt::" + normalized))
	return hex.EncodeToString(sum[:])
}

// HasAnyIdentifier 是否有任一有效身份标识。
// trim 后非空才算（避免 "  " 这种纯空白被误判为有效标识）。
func HasAnyIdentifier(in identity.Identifiers) bool {
	return strings.TrimSpace(in.Phone) != "" ||
		strings.TrimSpace(in.Email) != "" ||
		strings.TrimSpace(in.WechatOpenID) != "" ||
		strings.TrimSpace(in.DouyinOpenID) != "" ||
		strings.TrimSpace(in.XiaohongshuID) != ""
}
