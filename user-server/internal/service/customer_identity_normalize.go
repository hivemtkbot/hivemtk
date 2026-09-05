package service

import (
	"hivemtk-user/internal/identity"
)

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
