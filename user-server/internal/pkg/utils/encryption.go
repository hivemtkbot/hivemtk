package utils

// 敏感字段加密已于 2026-08-04 彻底移除（本地单用户私有部署无需字段级加密，属多余负担）。
// 下方函数保留签名以维持既有调用方编译，但均为明文直通：凭据以明文存储于本地库。

// GetCookieEncryptionKey 返回空字符串（加密已移除，不再生成/持久化密钥）
func GetCookieEncryptionKey() string { return "" }

// GetCookieSecretPath 返回空字符串（加密已移除）
func GetCookieSecretPath() string { return "" }

// Encrypt 明文直通（敏感字段加密已彻底移除）
func Encrypt(data, key string) (string, error) { return data, nil }

// Decrypt 明文直通（敏感字段加密已彻底移除）
func Decrypt(encryptedData, key string) (string, error) { return encryptedData, nil }
