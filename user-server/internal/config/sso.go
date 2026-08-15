package config

// SSOConfig 企业 SSO 登录配置（2026-08-15 M3-P1-E3）
//
// 由 config.yaml 顶层 sso 段加载，未配置时 Enabled=false（企业登录关闭，不影响既有本地登录）。
//
// 示例 (config.yaml)：
//
//	sso:
//	  enabled: true
//	  providers:
//	    feishu:
//	      issuer: "https://open.feishu.cn"
//	      client_id: "cli_xxx"
//	      client_secret: "xxx"
//	      redirect_url: "https://your-domain/api/sso/callback/feishu"
//	      auto_provision: true
//	      default_role: "user"
//	      scopes: ["openid", "profile", "email"]
type SSOConfig struct {
	Enabled   bool                        `yaml:"enabled" json:"enabled"`
	Providers map[string]SSOProviderConfig `yaml:"providers" json:"providers"`
}

// SSOProviderConfig 单个 IdP 提供方配置
type SSOProviderConfig struct {
	Issuer                string   `yaml:"issuer" json:"issuer"`
	ClientID              string   `yaml:"client_id" json:"client_id"`
	ClientSecret          string   `yaml:"client_secret" json:"client_secret"`
	RedirectURL           string   `yaml:"redirect_url" json:"redirect_url"`
	AutoProvision         bool     `yaml:"auto_provision" json:"auto_provision"`
	DefaultRole           string   `yaml:"default_role" json:"default_role"`
	Scopes                []string `yaml:"scopes" json:"scopes"`
	AuthorizationEndpoint string   `yaml:"authorization_endpoint" json:"authorization_endpoint"`
	TokenEndpoint         string   `yaml:"token_endpoint" json:"token_endpoint"`
	UserInfoEndpoint      string   `yaml:"userinfo_endpoint" json:"userinfo_endpoint"`
	JWKSURI               string   `yaml:"jwks_uri" json:"jwks_uri"`
}
