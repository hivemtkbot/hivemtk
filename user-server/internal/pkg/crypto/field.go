package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sync"
)

var (
	gcm       cipher.AEAD
	once      sync.Once
	initErr   error
	keySource = "FIELD_ENCRYPTION_KEY"
)

// Init 初始化全局 AEAD（从环境变量读密钥）
func Init() error {
	once.Do(func() {
		key := os.Getenv(keySource)
		if len(key) < 32 {
			initErr = errors.New("FIELD_ENCRYPTION_KEY must be >= 32 bytes")
			return
		}

		block, err := aes.NewCipher([]byte(key[:32]))
		if err != nil {
			initErr = err
			return
		}
		gcm, err = cipher.NewGCM(block)
		if err != nil {
			initErr = err
			return
		}
	})
	return initErr
}

// Encrypt 加密明文为 base64 字符串（nonce || ciphertext）
func Encrypt(plaintext string) (string, error) {
	if gcm == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 base64 字符串为明文
func Decrypt(ciphertextBase64 string) (string, error) {
	if gcm == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}
	if ciphertextBase64 == "" {
		return "", nil
	}

	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextBase64)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func MaskPhone(phone string) string {
	if len(phone) < 7 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

// MaskEmail 脱敏邮箱
// 例：test@example.com → t**t@example.com
func MaskEmail(email string) string {
	if email == "" {
		return ""
	}
	at := -1
	for i, c := range email {
		if c == '@' {
			at = i
			break
		}
	}
	if at < 1 {
		return "****"
	}
	if at <= 2 {
		return "**" + email[at:]
	}
	return email[:1] + "**" + email[at:]
}
