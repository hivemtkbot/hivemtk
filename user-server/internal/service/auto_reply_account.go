package service

import (
	"marketing/internal/model"
)

// auto_reply_account_service.go
//
// 从 model.AutoReplyAccount 迁移而来的 Cookie 存取函数（架构清理）。
// model 不应包含业务方法（五层架构约束），Cookie 的 Get/Set 由 service 包级函数承载。
// 敏感字段加密已彻底移除，本私有化单用户部署凭据以明文存储。

// GetAutoReplyAccountCookie 返回 AutoReplyAccount 的 Cookie（明文存储）
//
// 该函数从 model.AutoReplyAccount 迁移而来（架构清理），逻辑等价于原
// (*AutoReplyAccount).GetCookie() 方法。空 Cookie 返回 ("", nil)。
func GetAutoReplyAccountCookie(a *model.AutoReplyAccount) (string, error) {
	if a == nil || a.Cookie == "" {
		return "", nil
	}
	return a.Cookie, nil
}

// SetAutoReplyAccountCookie 写入 Cookie 到 AutoReplyAccount（明文存储）
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
	a.Cookie = cookie
	return nil
}
