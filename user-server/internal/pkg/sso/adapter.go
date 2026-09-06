package sso

import "fmt"

// Adapter 统一 SSO IdP 适配器接口
type Adapter interface {
	Name() string
	DisplayName() string
	OIDC() *OIDCProvider
	MapClaims(c *IDTokenClaims) *NormalizedUser
}

// NormalizedUser 归一化后的 SSO 用户信息
type NormalizedUser struct {
	Provider string
	Subject  string
	Username string
	Email    string
	RealName string
	Avatar   string
	Role     string
	Groups   []string
}

type baseAdapter struct {
	name        string
	displayName string
	provider    *OIDCProvider
}

func (a *baseAdapter) Name() string        { return a.name }
func (a *baseAdapter) DisplayName() string { return a.displayName }
func (a *baseAdapter) OIDC() *OIDCProvider { return a.provider }

func (a *baseAdapter) normalize(c *IDTokenClaims) *NormalizedUser {
	prov := a.provider
	return &NormalizedUser{
		Provider: a.name,
		Subject:  c.Subject,
		Username: prov.MapUsername(c),
		Email:    prov.MapEmail(c),
		RealName: c.Name,
		Avatar:   c.Picture,
		Role:     prov.MapRole(c),
		Groups:   c.Groups,
	}
}

// NewAdapter 按 provider 名称构建统一适配器
//
// 支持：
//   - feishu / dingtalk / wecom：内置 IdP 专属 claim 映射
//   - 其他名称：通用 OIDC（标准 claims 映射）
func NewAdapter(name string, cfg OIDCConfig) (Adapter, error) {
	base := &baseAdapter{
		name:     name,
		provider: NewOIDCProvider(cfg),
	}
	switch name {
	case "feishu":
		base.displayName = "飞书"
		return &FeishuAdapter{base}, nil
	case "dingtalk":
		base.displayName = "钉钉"
		return &DingTalkAdapter{base}, nil
	case "wecom":
		base.displayName = "企业微信"
		return &WeComAdapter{base}, nil
	case "":
		return nil, fmt.Errorf("sso: provider name is empty")
	default:
		base.displayName = name
		return &GenericAdapter{base}, nil
	}
}
