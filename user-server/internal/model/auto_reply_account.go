package model

import (
	"encoding/json"
	"time"

	"marketing/internal/pkg/utils"
)

type AutoReplyAccount struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	UserID            uint       `gorm:"index" json:"user_id"`
	Platform          string     `gorm:"size:20;index" json:"platform"`
	Username          string     `gorm:"size:100" json:"username"`
	Cookie            string     `gorm:"type:text" json:"-"`
	IsActive          bool       `gorm:"default:false" json:"is_active"`
	Headless          bool       `gorm:"default:true" json:"headless"`
	WsMode            bool       `gorm:"default:false" json:"ws_mode"`
	LastWSConnectedAt *time.Time `json:"last_ws_connected_at"`
	LoginAt           *time.Time `json:"login_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// GetCookie 获取解密后的Cookie
func (a *AutoReplyAccount) GetCookie() (string, error) {
	if a.Cookie == "" {
		return "", nil
	}

	encryptionKey := getEncryptionKey()
	return utils.Decrypt(a.Cookie, encryptionKey)
}

// SetCookie 设置加密后的Cookie
func (a *AutoReplyAccount) SetCookie(cookie string) error {
	if cookie == "" {
		a.Cookie = ""
		return nil
	}

	encryptionKey := getEncryptionKey()
	encryptedCookie, err := utils.Encrypt(cookie, encryptionKey)
	if err != nil {
		return err
	}

	a.Cookie = encryptedCookie
	return nil
}

// getEncryptionKey 获取加密密钥
// 私域部署：统一从 utils.GetCookieEncryptionKey 获取（已实现持久化安全密钥）
func getEncryptionKey() string {
	return utils.GetCookieEncryptionKey()
}

// MarshalJSON 自定义JSON序列化，用于安全地处理Cookie
func (a AutoReplyAccount) MarshalJSON() ([]byte, error) {
	type Alias AutoReplyAccount
	return json.Marshal(&struct {
		*Alias
		Cookie string `json:"cookie,omitempty"` // 在序列化时返回解密后的Cookie
	}{
		Alias:  (*Alias)(&a),
		Cookie: a.getDecryptedCookieForSerialization(),
	})
}

// UnmarshalJSON 自定义JSON反序列化，用于加密存储Cookie
func (a *AutoReplyAccount) UnmarshalJSON(data []byte) error {
	type Alias AutoReplyAccount
	aux := &struct {
		*Alias
		Cookie string `json:"cookie"`
	}{
		Alias: (*Alias)(a),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// 加密存储Cookie
	if aux.Cookie != "" {
		if err := a.SetCookie(aux.Cookie); err != nil {
			return err
		}
	}

	return nil
}

// getDecryptedCookieForSerialization 用于序列化的解密Cookie方法
func (a *AutoReplyAccount) getDecryptedCookieForSerialization() string {
	decrypted, err := a.GetCookie()
	if err != nil {
		// 如果解密失败，返回空字符串
		return ""
	}
	return decrypted
}
