// 企业微信 SSO 适配器（2026-08-15 M3-P1-E3）
//
// 企业微信（WeCom / WeChat Work）OIDC：
//   - 授权端点：https://login.work.weixin.qq.com/wwlogin/sso/login
//   - token 端点：https://qyapi.weixin.qq.com/cgi-bin/auth/getuserinfo
//   - userinfo：随 token 响应返回 userid/name/avatar
//
// ID Token 常用 claims：
//   - sub / userid：userid（唯一标识，Extra 中通常带 userid）
//   - name：姓名
//   - email：邮箱（可能为空）
//   - avatar：头像（Extra）
//
// 企业微信不提供标准 OIDC Discovery，部署时必须显式配置端点。
package sso

// WeComAdapter 企业微信 SSO 适配器
type WeComAdapter struct {
	*baseAdapter
}

// MapClaims 归一化企业微信 ID Token claims
func (a *WeComAdapter) MapClaims(c *IDTokenClaims) *NormalizedUser {
	nu := a.normalize(c)
	if v, ok := stringExtra(c, "userid"); ok && v != "" {
		if nu.Subject == "" {
			nu.Subject = v
		}
		if nu.Username == "" || nu.Username == c.Subject {
			nu.Username = v
		}
	}
	if v, ok := stringExtra(c, "avatar"); ok && nu.Avatar == "" {
		nu.Avatar = v
	}
	return nu
}

