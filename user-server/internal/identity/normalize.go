// Package identity 提供 OneID 客户身份识别所需的规范化与哈希工具。
//
// 商业产品级 OneID 规范化：
//   - 不做模糊匹配：手机号/邮箱/ID 必须是精确归一，不引入概率风险；
//   - 业务可解释：归一规则公开、可在管理后台查看；
//   - 可逆：normalizePhone 输出可直接落库作为展示值。
package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// 渠道身份标识（不引入 service 包，避免循环依赖）
type Identifiers struct {
	Phone         string `json:"phone"`
	Email         string `json:"email"`
	WechatOpenID  string `json:"wechat_open_id"`
	DouyinOpenID  string `json:"douyin_open_id"`
	XiaohongshuID string `json:"xiaohongshu_id"`
}

var (
	phoneDigits = regexp.MustCompile(`\D+`)
	emailValid = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9](?:[a-z0-9.\-]*[a-z0-9])?\.[a-z]{2,}$`)
)

// NormalizePhone 将各种格式的手机号统一为 11 位数字（国内）。
// 输入：+86 138 0013 8000 / 138-0013-8000 / 86 13800138000
// 输出：13800138000
// 没有任何数字时返回原 trim 字符串。
func NormalizePhone(raw string) string {
	if raw == "" {
		return ""
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	digits := phoneDigits.ReplaceAllString(trimmed, "")
	if digits == "" {
		return trimmed
	}
	if strings.HasPrefix(digits, "0086") && len(digits) >= 15 {
		digits = digits[4:]
	} else if strings.HasPrefix(digits, "86") && len(digits) >= 12 {
		digits = digits[2:]
	}
	return digits
}

// NormalizeEmail 邮箱小写、去空格。
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

// Normalize 一次性规范化所有身份标识。
func Normalize(in Identifiers) Identifiers {
	return Identifiers{
		Phone:         NormalizePhone(in.Phone),
		Email:         NormalizeEmail(in.Email),
		WechatOpenID:  NormalizeOpenID(in.WechatOpenID),
		DouyinOpenID:  NormalizeOpenID(in.DouyinOpenID),
		XiaohongshuID: NormalizeOpenID(in.XiaohongshuID),
	}
}

// PhoneHash 返回手机号 SHA-256 哈希（用于脱敏匹配、日志审计）。
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

// HasAny 是否有任一有效身份标识。
// 注意：纯空格不算，必须至少有一个 trim 后非空字段。
func HasAny(in Identifiers) bool {
	return strings.TrimSpace(in.Phone) != "" ||
		strings.TrimSpace(in.Email) != "" ||
		strings.TrimSpace(in.WechatOpenID) != "" ||
		strings.TrimSpace(in.DouyinOpenID) != "" ||
		strings.TrimSpace(in.XiaohongshuID) != ""
}

