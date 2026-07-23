package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"

	"gorm.io/gorm"
)

// SensitiveEncryptor 敏感字段加密器
// 用于对明文敏感字段（手机/邮箱）进行加密存储
// 私域部署：使用统一的 Cookie 加密密钥（同源加密便于密钥管理）
type SensitiveEncryptor struct {
	key []byte
}

// NewSensitiveEncryptor 创建加密器
// 密钥派生：使用 SHA-256(cookie_secret + "sensitive_v1")
func NewSensitiveEncryptor(cookieSecret string) *SensitiveEncryptor {
	h := sha256.New()
	h.Write([]byte(cookieSecret))
	h.Write([]byte("sensitive_v1"))
	return &SensitiveEncryptor{key: h.Sum(nil)}
}

// Encrypt 加密敏感数据
func (e *SensitiveEncryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密敏感数据
func (e *SensitiveEncryptor) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}
	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// SensitiveBeforeSave GORM BeforeSave 钩子辅助函数
func SensitiveBeforeSave(encryptor *SensitiveEncryptor, plain string) (string, error) {
	return encryptor.Encrypt(plain)
}

// SensitiveAfterFind GORM AfterFind 钩子辅助函数
func SensitiveAfterFind(encryptor *SensitiveEncryptor, encrypted string) (string, error) {
	if encrypted == "" {
		return "", nil
	}
	return encryptor.Decrypt(encrypted)
}

// BeforeSaveHook 默认敏感字段加密钩子（用于 BeforeSave 回调）
func BeforeSaveHook(encryptor *SensitiveEncryptor) func(db *gorm.DB) error {
	return func(db *gorm.DB) error {
		if v, ok := db.Statement.Dest.(interface {
			GetSensitiveFields() map[string]string
		}); ok {
			for field, value := range v.GetSensitiveFields() {
				encrypted, err := encryptor.Encrypt(value)
				if err != nil {
					return err
				}
				db.Statement.SetColumn(field+"_encrypted", encrypted)
			}
		}
		return nil
	}
}
