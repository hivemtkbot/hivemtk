package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"sync"

	"marketing/internal/pkg/utils/logger"
)

// GetCookieEncryptionKey 统一读取 Cookie 加密密钥
// 安全策略（私域独立部署）：
//  1. 强制从环境变量 COOKIE_ENCRYPTION_KEY 加载
//  2. 首次启动时若环境变量未配置且本地密钥文件不存在，则生成 64 字节随机密钥并保存到 <data_dir>/.cookie_secret
//  3. 后续启动优先从密钥文件读取（避免每次重启密钥变化导致已加密数据无法解密）
//  4. 严禁使用公开硬编码字符串作为密钥回退
//     用于 platform 适配器、AutoReplyAccount 等模块的 Cookie 加解密。
var (
	cookieSecretOnce sync.Once
	cookieSecret     string
	cookieSecretPath string
)

// getCookieSecretDataDir 解析密钥持久化目录
// 优先级：COOKIE_SECRET_DIR > DATA_DIR > ./.data
func getCookieSecretDataDir() string {
	if dir := os.Getenv("COOKIE_SECRET_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}
	return "./.data"
}

// loadOrGenerateCookieSecret 加载或生成持久化 Cookie 加密密钥
func loadOrGenerateCookieSecret() string {
	dir := getCookieSecretDataDir()
	secretPath := dir + "/.cookie_secret"
	cookieSecretPath = secretPath

	// 1. 优先环境变量
	if envKey := os.Getenv("COOKIE_ENCRYPTION_KEY"); envKey != "" {
		if len(envKey) < 32 {
			logger.Warnf("[security] COOKIE_ENCRYPTION_KEY 长度不足 32 字符，使用 SHA-256 派生为 64 字符")
			h := sha256.Sum256([]byte(envKey))
			return hexEncode(h[:])
		}
		return envKey
	}

	// 2. 尝试从持久化文件读取
	if data, err := os.ReadFile(secretPath); err == nil {
		key := string(data)
		if len(key) >= 32 {
			return key
		}
	}

	// 3. 生成新密钥（64 字节随机）并持久化
	buf := make([]byte, 64)
	if _, err := io.ReadFull(rand.Reader, buf); err != nil {
		// 极端情况下 rand 失败，使用降级方案：基于 hostname + 启动时间生成
		host, _ := os.Hostname()
		seed := host + "|" + os.Getenv("PATH") + "|" + randomFallbackSeed()
		h := sha256.Sum256([]byte(seed))
		buf = h[:]
	}
	key := base64.StdEncoding.EncodeToString(buf)

	// 持久化（0600 权限）
	if err := os.MkdirAll(dir, 0700); err == nil {
		_ = os.WriteFile(secretPath, []byte(key), 0600)
	}

	return key
}

// randomFallbackSeed 极端情况下的随机种子
func randomFallbackSeed() string {
	b := make([]byte, 16)
	_, _ = io.ReadFull(rand.Reader, b)
	return base64.StdEncoding.EncodeToString(b)
}

// hexEncode 字节转十六进制
func hexEncode(b []byte) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, len(b)*2)
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0F]
	}
	return string(out)
}

// GetCookieEncryptionKey 线程安全获取 Cookie 加密密钥
func GetCookieEncryptionKey() string {
	cookieSecretOnce.Do(func() {
		cookieSecret = loadOrGenerateCookieSecret()
	})
	return cookieSecret
}

// GetCookieSecretPath 返回当前密钥文件路径（用于运维诊断）
func GetCookieSecretPath() string {
	if cookieSecretPath == "" {
		GetCookieEncryptionKey()
	}
	return cookieSecretPath
}

// Encrypt 加密数据
func Encrypt(data, key string) (string, error) {
	block, err := aes.NewCipher([]byte(createKey(key)))
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(data), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密数据
func Decrypt(encryptedData, key string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher([]byte(createKey(key)))
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

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// createKey 创建固定长度的密钥
func createKey(key string) string {
	const keyLen = 32 // AES-256 requires 32-byte key
	if len(key) >= keyLen {
		return key[:keyLen]
	}

	// 如果密钥长度不足，填充零字节
	result := make([]byte, keyLen)
	copy(result, key)
	return string(result)
}
