package dto

import "time"

// CreateShortLinkRequest 创建短链请求
type CreateShortLinkRequest struct {
	ShortCode   string     `json:"short_code" binding:"required"`       
	OriginalURL string     `json:"original_url" binding:"required,url"` 
	Title       string     `json:"title"`                               
	Description string     `json:"description"`
	UtmSource   string     `json:"utm_source"`
	UtmMedium   string     `json:"utm_medium"`
	UtmCampaign string     `json:"utm_campaign"`
	DomainID    uint       `json:"domain_id"`                           
	Password    string     `json:"password"`                            
	ExpireTime  *time.Time `json:"expire_time"`                         
}

// UpdateShortLinkRequest 更新短链请求
type UpdateShortLinkRequest struct {
	ID          uint       `json:"id" binding:"required"`
	ShortCode   string     `json:"short_code"`                          
	OriginalURL string     `json:"original_url" binding:"required,url"` 
	Title       string     `json:"title"`                               
	Description string     `json:"description"`
	UtmSource   string     `json:"utm_source"`
	UtmMedium   string     `json:"utm_medium"`
	UtmCampaign string     `json:"utm_campaign"`
	DomainID    uint       `json:"domain_id"`                           
	Password    string     `json:"password"`                            
	ExpireTime  *time.Time `json:"expire_time"`                         
	Status      int        `json:"status"`                              
}

// GetShortLinkRequest 获取短链请求
type GetShortLinkRequest struct {
	ID uint `json:"id" binding:"required"`
}

// DeleteShortLinkRequest 删除短链请求
type DeleteShortLinkRequest struct {
	ID uint `json:"id" binding:"required"`
}

// ListShortLinkRequest 短链列表请求
type ListShortLinkRequest struct {
	Page        int    `json:"page" form:"page"`                 
	PageSize    int    `json:"page_size" form:"page_size"`       
	ShortCode   string `json:"short_code" form:"short_code"`     
	OriginalURL string `json:"original_url" form:"original_url"` 
	Status      int    `json:"status" form:"status"`             
}

// ShortLinkResponse 短链响应
type ShortLinkResponse struct {
	ID          uint       `json:"id"`
	ShortCode   string     `json:"short_code"`
	OriginalURL string     `json:"original_url"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DomainID    uint       `json:"domain_id"`
	Password    string     `json:"password"`
	ExpireTime  *time.Time `json:"expire_time"`
	ClickCount  int        `json:"click_count"`
	Status      int        `json:"status"`
	StatusStr   string     `json:"status_str"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ShortLinkListResponse 短链列表响应
type ShortLinkListResponse struct {
	List  []ShortLinkResponse `json:"list"`
	Total int64               `json:"total"`
}

// AccessShortLinkRequest 访问短链请求
type AccessShortLinkRequest struct {
	ShortCode string `json:"short_code" binding:"required"` 
	Password  string `json:"password"`                      
	UserAgent string `json:"user_agent"`                    
	IP        string `json:"ip"`                            
	Referer   string `json:"referer"`                       
}

// AccessShortLinkResponse 访问短链响应
type AccessShortLinkResponse struct {
	OriginalURL string `json:"original_url"` 
	Title       string `json:"title"`        
}

// GenerateShortCodeRequest 生成短码请求
type GenerateShortCodeRequest struct {
	Length int `json:"length" binding:"min=4,max=10"` 
}

// GenerateShortCodeResponse 生成短码响应
type GenerateShortCodeResponse struct {
	ShortCode string `json:"short_code"` 
}

