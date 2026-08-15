package dto

// DomainPoolCreateRequest 创建域名池请求
type DomainPoolCreateRequest struct {
	Domain  string `json:"domain" binding:"required"` 
	Port    int    `json:"port"`                      
	Purpose string `json:"purpose"`                   
}

// DomainPoolUpdateRequest 更新域名池请求
type DomainPoolUpdateRequest struct {
	ID                int    `json:"id" binding:"required"`         
	Domain            string `json:"domain" binding:"required"`     
	Port              int    `json:"port"`                          
	Purpose           string `json:"purpose"`                       
	Status            int    `json:"status"`                        
	AutoSwitchEnabled *bool  `json:"auto_switch_enabled,omitempty"` 
}

// DomainPoolListRequest 域名池列表请求
type DomainPoolListRequest struct {
	Page     int    `form:"page,default=1"`       
	PageSize int    `form:"page_size,default=10"` 
	Domain   string `form:"domain"`               
	Status   int    `form:"status"`               
}

// DomainPoolResponse 域名池响应
// G 域 ：扩展健康度评分、黑名单、活跃标记
type DomainPoolResponse struct {
	ID                  int    `json:"id"`                   
	Domain              string `json:"domain"`               
	Port                int    `json:"port"`                 
	Purpose             string `json:"purpose"`              
	Status              int    `json:"status"`               
	StatusStr           string `json:"status_str"`           
	LastCheck           string `json:"last_check"`           
	CreatedAt           string `json:"created_at"`           
	UpdatedAt           string `json:"updated_at"`           
	HealthScore         int    `json:"health_score"`         
	ConsecutiveFailures int    `json:"consecutive_failures"` 
	DNSResolved         bool   `json:"dns_resolved"`         
	DNSError            string `json:"dns_error"`            
	LastHTTPStatus      int    `json:"last_http_status"`     
	LastLatencyMs       int    `json:"last_latency_ms"`      
	OnBlacklist         bool   `json:"on_blacklist"`         
	AutoSwitchEnabled   bool   `json:"auto_switch_enabled"`  
	IsActive            bool   `json:"is_active"`            
}

// DomainPoolListResponse 域名池列表响应
type DomainPoolListResponse struct {
	List  []DomainPoolResponse `json:"list"`  
	Total int64                `json:"total"` 
}

// DomainPoolCheckResponse 域名检查响应
type DomainPoolCheckResponse struct {
	ID     int    `json:"id"`     
	Status int    `json:"status"` 
	Msg    string `json:"msg"`    
}

