package utils


// GetCookieEncryptionKey 返回空字符串（加密已移除，不再生成/持久化密钥）
func GetCookieEncryptionKey() string { return "" }

// GetCookieSecretPath 返回空字符串（加密已移除）
func GetCookieSecretPath() string { return "" }

// Encrypt 明文直通（敏感字段加密已彻底移除）
func Encrypt(data, key string) (string, error) { return data, nil }

// Decrypt 明文直通（敏感字段加密已彻底移除）
func Decrypt(encryptedData, key string) (string, error) { return encryptedData, nil }

