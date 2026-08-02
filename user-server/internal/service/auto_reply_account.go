package service

import (
	"marketing/internal/model"
	"marketing/internal/pkg/utils"
)

// auto_reply_account_service.go
//
// 从 model.AutoReplyAccount 迁移而来的 Cookie 加解密函数（架构清理）。
// model 不应包含业务方法（五层架构约束），Cookie 的 Get/Set 由 service 包级函数承载。
//
// MarshalJSON / UnmarshalJSON / getDecryptedCookieForSerialization 等自定义
// Cookie 字段已有 `json:"-"`，默认序列化不会暴露；
// 反序列化时若需要从 JSON 写入加密 Cookie，由调用方在 service 层先
// Unmarshal 再调用 SetAutoReplyAccountCookie 完成加密。

// getAutoReplyAccountEncryptionKey 获取 Cookie 加密密钥
//
// 私域部署：统一从 utils.GetCookieEncryptionKey 获取（已实现持久化安全密钥）。
// 从 model 包迁移而来（原本是 model 包级函数 getEncryptionKey，因 model 不应
// import utils 而下沉到 service）。
func getAutoReplyAccountEncryptionKey() string {
	return utils.GetCookieEncryptionKey()
}

// GetAutoReplyAccountCookie 解密并返回 AutoReplyAccount 的 Cookie
//
// 该函数从 model.AutoReplyAccount 迁移而来（架构清理），逻辑等价于原
// (*AutoReplyAccount).GetCookie() 方法。空 Cookie 返回 ("", nil)。
func GetAutoReplyAccountCookie(a *model.AutoReplyAccount) (string, error) {
	if a == nil || a.Cookie == "" {
		return "", nil
	}
	return utils.Decrypt(a.Cookie, getAutoReplyAccountEncryptionKey())
}

// SetAutoReplyAccountCookie 加密并写入 Cookie 到 AutoReplyAccount
//
// 该函数从 model.AutoReplyAccount 迁移而来（架构清理），逻辑等价于原
// (*AutoReplyAccount).SetCookie(cookie) 方法。空 cookie 会清空 Cookie 字段。
func SetAutoReplyAccountCookie(a *model.AutoReplyAccount, cookie string) error {
	if a == nil {
		return nil
	}
	if cookie == "" {
		a.Cookie = ""
		return nil
	}
	encrypted, err := utils.Encrypt(cookie, getAutoReplyAccountEncryptionKey())
	if err != nil {
		return err
	}
	a.Cookie = encrypted
	return nil
}
