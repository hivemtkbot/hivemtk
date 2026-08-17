package service

import (
	"hivemtk-user/internal/identity"
)

// 本文件保留为空壳，所有归一/哈希函数已迁移至 internal/identity 包，
// 委托方式以保持向后兼容（service.NormalizePhone = identity.NormalizePhone）。
//
// 历史原因：v3 审计 P0-05 修复前，service 与 identity 两份 NormalizePhone
// 边界条件不一致（"86"/"+86"/"0086" 长度判断有差），同一手机号会得到不同归一结果，
// 引发 OneID 跨表 join miss。已统一收敛到 identity 包。

// NormalizePhone 委托到 identity.NormalizePhone（保留向后兼容）
func NormalizePhone(raw string) string { return identity.NormalizePhone(raw) }

// NormalizeEmail 委托到 identity.NormalizeEmail
func NormalizeEmail(raw string) string { return identity.NormalizeEmail(raw) }

// NormalizeOpenID 委托到 identity.NormalizeOpenID
func NormalizeOpenID(raw string) string { return identity.NormalizeOpenID(raw) }

// NormalizeIdentifiers 委托到 identity.Normalize
func NormalizeIdentifiers(in identity.Identifiers) identity.Identifiers {
	return identity.Normalize(in)
}

// PhoneHash 委托到 identity.PhoneHash
func PhoneHash(phone string) string { return identity.PhoneHash(phone) }

// EmailHash 委托到 identity.EmailHash
func EmailHash(email string) string { return identity.EmailHash(email) }

// HasAnyIdentifier 委托到 identity.HasAny
func HasAnyIdentifier(in identity.Identifiers) bool { return identity.HasAny(in) }
