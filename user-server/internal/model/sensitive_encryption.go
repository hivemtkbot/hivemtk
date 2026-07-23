package model

// sensitive_encryption.go 敏感字段类型定义
//
// 加密器 SensitiveEncryptor 及加解密逻辑已迁移到 internal/pkg/utils/sensitive_encryption.go，
// model 仅保留 SensitiveString 类型（实现 driver.Valuer，被白名单允许）与脱敏工具函数。

// MaskPhone 脱敏手机号（保留前3位+后4位）
func MaskPhone(phone string) string {
	if len(phone) < 7 {
		return "***"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

// MaskEmail 脱敏邮箱（保留首字符+@+域名）
func MaskEmail(email string) string {
	at := -1
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at <= 0 || at >= len(email)-1 {
		return "***"
	}
	if at == 1 {
		return email[:1] + "***" + email[at:]
	}
	return email[:1] + "***" + email[at:]
}

// SensitiveField GORM 钩子辅助
// 用法：实现 BeforeSave/AfterFind 钩子调用 Encrypt/Decrypt
// 简化：通过 SensitiveString 类型自动处理
type SensitiveString struct {
	Plain     string // 明文（不持久化）
	Encrypted string `gorm:"column:phone_encrypted"` // 密文（持久化）
}

// Value 实现 driver.Valuer
func (s SensitiveString) Value() any {
	return s.Encrypted
}
