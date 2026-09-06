// 通用 OIDC SSO 适配器（2026-08-15 M3-P1-E3）
//
// 用于标准 OIDC 提供方（Okta / Azure AD / Keycloak / Auth0 等）：
// 直接使用标准 claims（sub / preferred_username / email / name / roles），
// 支持标准 OIDC Discovery。
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
