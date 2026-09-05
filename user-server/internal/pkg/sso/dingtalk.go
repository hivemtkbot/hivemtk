// 钉钉 SSO 适配器（2026-08-15 M3-P1-E3）
//
// 钉钉（DingTalk）OIDC：
//   - 授权端点：https://login.dingtalk.com/oauth2/auth
//   - token 端点：https://api.dingtalk.com/v1.0/oauth2/access_token
//   - userinfo 端点：https://api.dingtalk.com/v1.0/contact/users/me
//
// ID Token / userinfo 常用 claims：
//   - sub：userid（唯一标识）
//   - nick：昵称（Extra）
//   - name：姓名
//   - email：邮箱（可能为空）
//   - mobile：手机号（Extra）
//
// 钉钉不提供标准 OIDC Discovery，部署时必须显式配置端点。
package sso

// DingTalkAdapter 钉钉 SSO 适配器
type DingTalkAdapter struct {
	*baseAdapter
}

// MapClaims 归一化钉钉 ID Token claims
func (a *DingTalkAdapter) MapClaims(c *IDTokenClaims) *NormalizedUser {
	nu := a.normalize(c)
	if v, ok := stringExtra(c, "nick"); ok && v != "" {
		nu.Username = v
	} else if nu.Username == c.Subject && c.Name != "" {
		nu.Username = c.Name
	}
	if v, ok := stringExtra(c, "avatar"); ok && nu.Avatar == "" {
		nu.Avatar = v
	}
	return nu
}
