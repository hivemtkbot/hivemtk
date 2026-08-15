// SSO 适配器抽象（2026-08-15 M3-P1-E3）
//
// 在通用 OIDCProvider 之上，为各 IdP（飞书 / 钉钉 / 企微 / 通用 OIDC）
// 提供统一接口：把 IdP 特有的 ID Token claims 归一化为标准用户信息，
// 供 SSO 服务做本地用户关联 / 自动 provisioning。
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

// baseAdapter 通用适配器骨架（各 IdP 复用 OIDCProvider 通用映射逻辑）
type baseAdapter struct {
	name        string
	displayName string
	provider    *OIDCProvider
}

func (a *baseAdapter) Name() string        { return a.name }
func (a *baseAdapter) DisplayName() string { return a.displayName }
func (a *baseAdapter) OIDC() *OIDCProvider { return a.provider }

// normalize 用 OIDCProvider 的通用映射规则生成归一化用户
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

