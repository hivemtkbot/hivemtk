// Package secrets 提供基于 AES-256-GCM 的对称加密原语，落地 TL-1 决策：
//
//	启动时若未配置 HIVEMTK_MASTER_KEY（>=32 字节），则 fail-fast，
//	禁止任何明文落库路径继续运行。
//
// 设计要点：
//   - 使用 AEAD (GCM) 而非裸 AES，提供完整性校验。
//   - nonce 12 字节随机，前缀拼接到密文，方便持久化。
//   - 与 DB 中"明文 + 旧列"双轨期间，提供 MigrateLegacy 方法把旧明文重写为密文。
//   - 任意路径泄露 MASTER_KEY 即整盘作废；建议通过 secret manager 注入。
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

const (
	envMasterKey   = "HIVEMTK_MASTER_KEY"
	keyLen         = 32 // AES-256
	nonceLen       = 12 // GCM standard
	masterKeyReady = "secrets.master_key_ready"
)

// ErrMasterKeyMissing 在启动 fail-fast 时返回；任何调用 Encrypt/Decrypt 的代码
// 在缺密钥时也会拿到该错误。
var ErrMasterKeyMissing = errors.New("secrets: HIVEMTK_MASTER_KEY is missing or shorter than 32 bytes")

var (
	gcmOnce sync.Once
	gcmAEAD cipher.AEAD
)

// InitFromEnv 从环境变量读取主密钥并初始化全局 AEAD。空/过短则 fail-fast 返回 error。
// 进程内多次调用幂等。
func InitFromEnv() error {
	var initErr error
	gcmOnce.Do(func() {
		raw := os.Getenv(envMasterKey)
		if len(raw) < keyLen {
			initErr = fmt.Errorf("%w (got len=%d)", ErrMasterKeyMissing, len(raw))
			return
		}
		// 仅使用前 32 字节；超出部分忽略，便于运维粘贴完整 base64/hex 时不致失败。
		key := []byte(raw)[:keyLen]
		block, err := aes.NewCipher(key)
		if err != nil {
			initErr = fmt.Errorf("secrets: aes.NewCipher: %w", err)
			return
		}
		aead, err := cipher.NewGCM(block)
		if err != nil {
			initErr = fmt.Errorf("secrets: cipher.NewGCM: %w", err)
			return
		}
		gcmAEAD = aead
	})
	return initErr
}

// Ready 报告全局 AEAD 是否已就绪。
func Ready() bool { return gcmAEAD != nil }

// Encrypt 加密任意明文。失败可能由未初始化或随机源失败导致。
func Encrypt(plaintext []byte) ([]byte, error) {
	if gcmAEAD == nil {
		return nil, ErrMasterKeyMissing
	}
	nonce := make([]byte, nonceLen)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("secrets: read nonce: %w", err)
	}
	out := gcmAEAD.Seal(nil, nonce, plaintext, nil)
	// 输出格式：nonce || ciphertext（含 GCM tag）。
	full := make([]byte, 0, nonceLen+len(out))
	full = append(full, nonce...)
	full = append(full, out...)
	return full, nil
}

// Decrypt 解密；密文损坏或密钥不匹配会返回 error。
func Decrypt(ciphertext []byte) ([]byte, error) {
	if gcmAEAD == nil {
		return nil, ErrMasterKeyMissing
	}
	if len(ciphertext) < nonceLen {
		return nil, errors.New("secrets: ciphertext too short")
	}
	nonce := ciphertext[:nonceLen]
	body := ciphertext[nonceLen:]
	pt, err := gcmAEAD.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: gcm.Open: %w", err)
	}
	return pt, nil
}

// EncryptString/DecryptString 是字符串版本的便捷封装。
func EncryptString(s string) (string, error) {
	out, err := Encrypt([]byte(s))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func DecryptString(s string) (string, error) {
	out, err := Decrypt([]byte(s))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// IsCiphertextFormat 判定字符串是否是本模块产生的密文（粗略长度 + nonce 长度启发式）。
// 用于兼容历史明文双轨：DB 列里既可能是明文，也可能是本模块写的密文。
func IsCiphertextFormat(s string) bool {
	return len(s) >= nonceLen+16 // 16 字节是 GCM tag 的最小长度
}
