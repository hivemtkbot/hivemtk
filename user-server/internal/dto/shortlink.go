package dto

import "time"

// CreateShortLinkRequest 创建短链请求
type CreateShortLinkRequest struct {
	ShortCode   string     `json:"short_code" binding:"required"`       // 短码
	OriginalURL string     `json:"original_url" binding:"required,url"` // 原始URL
	Title       string     `json:"title"`                               // 标题
	Description string     `json:"description"`                         // 描述
	DomainID    uint       `json:"domain_id"`                           // 域名ID
	Password    string     `json:"password"`                            // 访问密码
	ExpireTime  *time.Time `json:"expire_time"`                         // 过期时间
}

// UpdateShortLinkRequest 更新短链请求
type UpdateShortLinkRequest struct {
	ID          uint       `json:"id" binding:"required"`
	ShortCode   string     `json:"short_code"`                          // 短码
	OriginalURL string     `json:"original_url" binding:"required,url"` // 原始URL
	Title       string     `json:"title"`                               // 标题
	Description string     `json:"description"`                         // 描述
	DomainID    uint       `json:"domain_id"`                           // 域名ID
	Password    string     `json:"password"`                            // 访问密码
	ExpireTime  *time.Time `json:"expire_time"`                         // 过期时间
	Status      int        `json:"status"`                              // 状态: 1-正常, 2-禁用
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
	Page        int    `json:"page" form:"page"`                 // 页码
	PageSize    int    `json:"page_size" form:"page_size"`       // 每页数量
	ShortCode   string `json:"short_code" form:"short_code"`     // 短码
	OriginalURL string `json:"original_url" form:"original_url"` // 原始URL
	Status      int    `json:"status" form:"status"`             // 状态
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
	ShortCode string `json:"short_code" binding:"required"` // 短码
	Password  string `json:"password"`                      // 访问密码
	UserAgent string `json:"user_agent"`                    // 用户代理
	IP        string `json:"ip"`                            // IP地址
	Referer   string `json:"referer"`                       // 来源页面
}

// AccessShortLinkResponse 访问短链响应
type AccessShortLinkResponse struct {
	OriginalURL string `json:"original_url"` // 原始URL
	Title       string `json:"title"`        // 标题
}

// GenerateShortCodeRequest 生成短码请求
type GenerateShortCodeRequest struct {
	Length int `json:"length" binding:"min=4,max=10"` // 短码长度
}

// GenerateShortCodeResponse 生成短码响应
type GenerateShortCodeResponse struct {
	ShortCode string `json:"short_code"` // 生成的短码
}
