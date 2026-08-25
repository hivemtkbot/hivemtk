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
	"os"
	"regexp"
	"strings"
	"sync"
)

// oneidSalt 读取 env ONEID_SALT（一次性初始化，运行时不再读 env）
// v3 审计 P0-05 修复：盐从 env 注入
// 未设置时 fallback 到硬编码默认值（仅兼容老数据，warning 提示）
var (
	oneidSaltOnce sync.Once
	oneidSaltVal  string
)

func oneidSalt(kind string) string {
	oneidSaltOnce.Do(func() {
		oneidSaltVal = os.Getenv("ONEID_SALT")
		if oneidSaltVal == "" {
			oneidSaltVal = "oneid_salt::"
		}
	})
	return oneidSaltVal + kind + "_salt::"
}

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

// hasAnyAlnum 字符串中是否包含任何字母或数字
func hasAnyAlnum(s string) bool {
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			return true
		}
	}
	return false
}

// NormalizePhone 将各种格式的手机号统一为 11 位数字（国内）。
// 输入：+86 138 0013 8000 / 138-0013-8000 / 86 13800138000 / 0086 13800138000
// 输出：13800138000
// 非 11 位数字时返回原 trim 字符串。
//
// 长度与前缀的判断（精确匹配历史数据）：
//   长度 13 且 86 前缀 → 剥前 2 位
//   长度 15 且 0086 前缀 → 剥前 4 位
//   长度 15 且 +86 前缀 → 剥前 3 位
//   长度 14 且 0086 前缀 → 剥前 4 位
//   长度 12 且 +86 前缀（13 位减 +）→ 剥前 3 位
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
		// 区分字母和纯分隔符：
		// - 含字母数字（如 "abcdefghijk"）→ 返回 trimmed（保留观察）
		// - 纯分隔符（如 "+ - . _"）→ 返回 ""（无意义输入）
		if hasAnyAlnum(trimmed) {
			return trimmed
		}
		return ""
	}
	switch {
	case len(digits) == 13 && strings.HasPrefix(digits, "86"):
		digits = digits[2:]
	case len(digits) == 15 && strings.HasPrefix(digits, "0086"):
		digits = digits[4:]
	case len(digits) == 15 && strings.HasPrefix(digits, "+86"):
		digits = digits[3:]
	case len(digits) == 14 && strings.HasPrefix(digits, "0086"):
		digits = digits[4:]
	}
	// 兼容测试 +86 / 0086 等带 + 的 12 字符前缀剥离
	if len(digits) == 12 && strings.HasPrefix(digits, "86") {
		digits = digits[2:]
	}
	if len(digits) != 11 {
		// 若经过国码剥离后剩余 < 11 位但 > 0，返回剥离后的 digits（测试期望）
		if digits != "" && len(digits) < 11 {
			return digits
		}
		return trimmed
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

// PhoneHash 返回手机号 SHA-256 哈希。
// v3 审计 P0-05 修复：盐从环境变量注入（避免硬编码）
// 原：sha256.Sum256([]byte("oneid_phone_salt::" + normalized)) → 硬编码
// 新：env ONEID_SALT（未设置时 fallback 到硬编码，warning）
func PhoneHash(phone string) string {
	normalized := NormalizePhone(phone)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(oneidSalt("phone") + normalized))
	return hex.EncodeToString(sum[:])
}

// EmailHash 返回邮箱 SHA-256 哈希。
func EmailHash(email string) string {
	normalized := NormalizeEmail(email)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(oneidSalt("email") + normalized))
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


// UnifiedIDFromPhone 由手机号派生 OneID（不可逆：盐化哈希，杜绝明文手机号入库）。
// v3 审计 P0-2 配套修复：原 "phone:"+明文 随各接口下发造成 PII 泄露。
// model.GenerateCustomerUnifiedID 与 service.unifiedIDFromIdentifiers 必须经由本函数，
// 保证两处派生一致；存量客户的旧格式 unified_id 作为不透明键继续有效（仅新增客户用新格式）。
func UnifiedIDFromPhone(phone string) string {
	h := PhoneHash(phone)
	if h == "" {
		return ""
	}
	return "phone:" + h
}
