package dto

// DomainPoolCreateRequest 创建域名池请求
type DomainPoolCreateRequest struct {
	Domain  string `json:"domain" binding:"required"` // 域名
	Port    int    `json:"port"`                      // 端口
	Purpose string `json:"purpose"`                   // 用途备注
}

// DomainPoolUpdateRequest 更新域名池请求
type DomainPoolUpdateRequest struct {
	ID                int    `json:"id" binding:"required"`         // ID
	Domain            string `json:"domain" binding:"required"`     // 域名
	Port              int    `json:"port"`                          // 端口
	Purpose           string `json:"purpose"`                       // 用途备注
	Status            int    `json:"status"`                        // 状态
	AutoSwitchEnabled *bool  `json:"auto_switch_enabled,omitempty"` // 是否启用自动切换
}

// DomainPoolListRequest 域名池列表请求
type DomainPoolListRequest struct {
	Page     int    `form:"page,default=1"`       // 页码
	PageSize int    `form:"page_size,default=10"` // 每页数量
	Domain   string `form:"domain"`               // 域名搜索
	Status   int    `form:"status"`               // 状态筛选
}

// DomainPoolResponse 域名池响应
// G 域 P1：扩展健康度评分、黑名单、活跃标记
type DomainPoolResponse struct {
	ID                  int    `json:"id"`                   // ID
	Domain              string `json:"domain"`               // 域名
	Port                int    `json:"port"`                 // 端口
	Purpose             string `json:"purpose"`              // 用途备注
	Status              int    `json:"status"`               // 状态
	StatusStr           string `json:"status_str"`           // 状态字符串
	LastCheck           string `json:"last_check"`           // 最后检查时间
	CreatedAt           string `json:"created_at"`           // 创建时间
	UpdatedAt           string `json:"updated_at"`           // 更新时间
	HealthScore         int    `json:"health_score"`         // 健康度评分 0-100
	ConsecutiveFailures int    `json:"consecutive_failures"` // 连续失败次数
	DNSResolved         bool   `json:"dns_resolved"`         // DNS 是否可解析
	DNSError            string `json:"dns_error"`            // DNS 错误信息
	LastHTTPStatus      int    `json:"last_http_status"`     // 最近 HTTP 状态码
	LastLatencyMs       int    `json:"last_latency_ms"`      // 最近 HTTP 响应耗时
	OnBlacklist         bool   `json:"on_blacklist"`         // 是否在黑名单
	AutoSwitchEnabled   bool   `json:"auto_switch_enabled"`  // 是否启用自动切换
	IsActive            bool   `json:"is_active"`            // 是否当前活跃
}

// DomainPoolListResponse 域名池列表响应
type DomainPoolListResponse struct {
	List  []DomainPoolResponse `json:"list"`  // 列表
	Total int64                `json:"total"` // 总数
}

// DomainPoolCheckResponse 域名检查响应
type DomainPoolCheckResponse struct {
	ID     int    `json:"id"`     // ID
	Status int    `json:"status"` // 状态
	Msg    string `json:"msg"`    // 消息
}
