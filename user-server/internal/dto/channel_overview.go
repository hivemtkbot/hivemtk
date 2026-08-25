package dto

// ChannelStatus 单渠道状态
type ChannelStatus struct {
	Channel          string   `json:"channel"`
	ChannelName      string   `json:"channel_name"`
	Category         string   `json:"category"`           // official_api / bridge
	AccountCount     int      `json:"account_count"`
	ActiveCount      int      `json:"active_count"`
	OnlineCount      int      `json:"online_count"`
	IntegrationReady bool     `json:"integration_ready"`
	RequiredFields   []string `json:"required_fields"`    // 配置时需要填写
	ConfigURLs       []string `json:"config_urls"`        // 配置入口 URL
	HealthURL        string   `json:"health_url,omitempty"`
}

// ChannelOverview 渠道总览
type ChannelOverview struct {
	Channels         []ChannelStatus `json:"channels"`
	TotalChannels    int             `json:"total_channels"`
	RealChannels     int             `json:"real_channels"`     // 真实可用数
	BridgeChannels   int             `json:"bridge_channels"`   // Bridge 类
	OfficialChannels int             `json:"official_channels"` // 官方 API 类
}

// CustomerChannelBinding 客户渠道绑定请求（POST /api/channels/bind）
type CustomerChannelBinding struct {
	CustomerID    string `json:"customer_id" binding:"required"`
	OneID         string `json:"one_id,omitempty"`
	Channel       string `json:"channel" binding:"required"`
	ChannelUserID string `json:"channel_user_id" binding:"required"`
	ChannelName   string `json:"channel_name,omitempty"`
	AccountID     string `json:"account_id,omitempty"`
	IsPrimary     bool   `json:"is_primary"`
	GroupID       string `json:"group_id,omitempty"`
}

// CustomerChannelIdentity 客户基础身份（ListCustomerChannels 用）
type CustomerChannelIdentity struct {
	UnifiedID string
	Phone     string
	Email     string
	Name      string
}
