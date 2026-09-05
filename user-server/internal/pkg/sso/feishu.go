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
