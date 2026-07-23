package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	"marketing/internal/pkg/utils"
)

// ============================================================================
// 敏感字段加密器（SensitiveFieldEncryption）
// 商业产品级安全合规：
//   1. 平台账号 Cookie / Token / 手机号 / 邮箱 等敏感字段必须落库前加密
//   2. AES-256-GCM 模式（认证加密 + 防篡改）
//   3. 密钥从环境变量强制加载，缺失则 panic
//   4. nonce 每次随机生成（防重放）
// ============================================================================

// SensitiveFieldEncryption 敏感字段加密器
type SensitiveFieldEncryption struct {
	gcm cipher.AEAD
}

// NewSensitiveFieldEncryption 创建敏感字段加密器
// 密钥从环境变量 SENSITIVE_ENCRYPTION_KEY 加载
// 长度必须 >= 32 字符（256 bit），否则 panic
func NewSensitiveFieldEncryption() *SensitiveFieldEncryption {
	key := utils.GetCookieEncryptionKey()
	if len(key) < 32 {
		panic("[SECURITY] SENSITIVE_ENCRYPTION_KEY 长度不足 32 字符")
	}
	// 取前 32 字节作为 AES-256 密钥
	keyBytes := []byte(key[:32])

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		panic("[SECURITY] AES cipher 初始化失败: " + err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panic("[SECURITY] GCM 初始化失败: " + err.Error())
	}
	return &SensitiveFieldEncryption{gcm: gcm}
}

// Encrypt 加密敏感字段
// 输出格式：base64(nonce + ciphertext + tag)
func (e *SensitiveFieldEncryption) Encrypt(ctx context.Context, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	nonce := make([]byte, e.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	cipherText := e.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

// Decrypt 解密敏感字段
func (e *SensitiveFieldEncryption) Decrypt(ctx context.Context, encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	nonceSize := e.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("密文长度不足")
	}
	nonce, cipherText := data[:nonceSize], data[nonceSize:]
	plain, err := e.gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// EncryptAccountCookie 加密平台账号 Cookie
func (e *SensitiveFieldEncryption) EncryptAccountCookie(ctx context.Context, platform, cookie string) (string, error) {
	if cookie == "" {
		return "", nil
	}
	// 加平台前缀，避免跨平台解密混淆
	prefixed := platform + "::" + cookie
	return e.Encrypt(context.Background(), prefixed)
}

// DecryptAccountCookie 解密平台账号 Cookie
func (e *SensitiveFieldEncryption) DecryptAccountCookie(ctx context.Context, encrypted string) (platform, cookie string, err error) {
	plain, err := e.Decrypt(context.Background(), encrypted)
	if err != nil {
		return "", "", err
	}
	for i := 0; i < len(plain)-1; i++ {
		if plain[i] == ':' && plain[i+1] == ':' {
			return plain[:i], plain[i+2:], nil
		}
	}
	return "", plain, nil
}

// EncryptPhone 加密手机号（用于存储客户敏感信息）
func (e *SensitiveFieldEncryption) EncryptPhone(ctx context.Context, phone string) (string, error) {
	return e.Encrypt(context.Background(), phone)
}

// DecryptPhone 解密手机号
func (e *SensitiveFieldEncryption) DecryptPhone(ctx context.Context, encrypted string) (string, error) {
	return e.Decrypt(context.Background(), encrypted)
}
