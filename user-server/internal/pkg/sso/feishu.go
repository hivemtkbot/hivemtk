// 飞书 SSO 适配器（2026-08-15 M3-P1-E3）
//
// 飞书（Lark）OIDC：
//   - 授权端点：https://accounts.feishu.cn/open-apis/authen/v1/authorize
//   - token 端点：https://accounts.feishu.cn/open-apis/authen/v1/oidc/access_token
//   - userinfo 端点：https://accounts.feishu.cn/open-apis/authen/v1/user_info
//
// ID Token 常用 claims：
//   - sub：open_id（唯一标识）
//   - name：姓名
//   - email：邮箱（需申请 contact 权限）
//   - avatar_url：头像（Extra）
//
// 由于飞书不提供标准 OIDC Discovery，部署时必须显式配置端点
// （authorization_endpoint / token_endpoint / userinfo_endpoint），
// 或经由网关标准 OIDC 代理。
package sso

// FeishuAdapter 飞书 SSO 适配器
type FeishuAdapter struct {
	*baseAdapter
}

// MapClaims 归一化飞书 ID Token claims
func (a *FeishuAdapter) MapClaims(c *IDTokenClaims) *NormalizedUser {
	nu := a.normalize(c)
	if c.PreferredUsername == "" && c.Name != "" {
		nu.Username = c.Name
	}
	if v, ok := stringExtra(c, "avatar_url"); ok && nu.Avatar == "" {
		nu.Avatar = v
	}
	if v, ok := stringExtra(c, "union_id"); ok && nu.Subject == "" {
		nu.Subject = v
	}
	return nu
}
