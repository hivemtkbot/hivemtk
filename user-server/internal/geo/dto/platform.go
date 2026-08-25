package dto

import "time"

// PlatformInfo 支持的发布平台信息
type PlatformInfo struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
	AuthType    string `json:"auth_type"`
	Enabled     bool   `json:"enabled"`
}

// PlatformAccountResponse 平台账号响应（脱敏：绝不回显明文凭据）
type PlatformAccountResponse struct {
	ID             string    `json:"id"`
	Platform       string    `json:"platform"`
	AccountID      string    `json:"account_id"`
	AccountName    string    `json:"account_name"`
	Status         string    `json:"status"`
	HasCredentials bool      `json:"has_credentials"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SavePlatformAccountRequest 保存平台账号请求
type SavePlatformAccountRequest struct {
	Platform    string            `json:"platform" binding:"required"`
	AccountID   string            `json:"account_id"`
	AccountName string            `json:"account_name" binding:"required"`
	Status      string            `json:"status"`
	Credentials map[string]string `json:"credentials"`
}

// PublishRequest 平台发布请求
type PublishRequest struct {
	ArticleID string `json:"article_id" binding:"required"`
	Platform  string `json:"platform" binding:"required"`
	Repo      string `json:"repo"`
	Path      string `json:"path"`
	Branch    string `json:"branch"`
	// AuthToken 兼容回退字段：服务端优先取用已保存账号凭据，其次环境变量，
	// 仅两者皆缺时才使用本字段（避免 token 明文传输与日志残留）
	AuthToken string `json:"auth_token"`
	CommitMsg string `json:"commit_message"`
	Filename  string `json:"filename"`
}

// PublishResponse 平台发布响应
type PublishResponse struct {
	ID           string `json:"id"`
	Status       string `json:"status"`
	PublishedURL string `json:"published_url"`
	Message      string `json:"message,omitempty"`
}
