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
