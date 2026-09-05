package sso

// GenericAdapter 通用 OIDC SSO 适配器
type GenericAdapter struct {
	*baseAdapter
}

// MapClaims 归一化标准 OIDC ID Token claims
func (a *GenericAdapter) MapClaims(c *IDTokenClaims) *NormalizedUser {
	return a.normalize(c)
}

func stringExtra(c *IDTokenClaims, key string) (string, bool) {
	if c == nil || c.Extra == nil {
		return "", false
	}
	v, ok := c.Extra[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}
