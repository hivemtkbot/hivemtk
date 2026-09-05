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
