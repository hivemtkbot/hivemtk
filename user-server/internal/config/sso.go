package config

type SSOConfig struct {
	Enabled   bool                         `yaml:"enabled" json:"enabled"`
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
